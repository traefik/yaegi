// Native boundary of net/http (server side): program listeners,
// connections and handlers given to net/http (docs/BOUNDARY.md).
//
// ListenAndServe and ListenAndServeTLS are net.Listen + Serve/ServeTLS on
// real TCP: the probes give program listeners to Serve and ServeTLS, where
// the checks are.
package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

var discard = log.New(io.Discard, "", 0)

// rlog records events of other goroutines.
type rlog struct {
	mu sync.Mutex
	l  []string
}

func (r *rlog) add(format string, args ...any) {
	r.mu.Lock()
	r.l = append(r.l, fmt.Sprintf(format, args...))
	r.mu.Unlock()
}

func (r *rlog) take() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := strings.Join(r.l, ",")
	r.l = nil
	return s
}

// pipeAddr is a net.Addr of the program.
type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

// tempErr is a temporary error of Accept (value receivers).
type tempErr struct{}

func (tempErr) Error() string   { return "accept busy" }
func (tempErr) Timeout() bool   { return false }
func (tempErr) Temporary() bool { return true }

// stopErr is the last error of Accept (pointer receivers).
type stopErr struct{ s string }

func (e *stopErr) Error() string   { return e.s }
func (e *stopErr) Timeout() bool   { return false }
func (e *stopErr) Temporary() bool { return false }

// plis is an in-memory net.Listener (net.Pipe), no network.
type plis struct {
	ch      chan net.Conn
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	temp    int
	accepts int
}

func newPlis() *plis { return &plis{ch: make(chan net.Conn, 4), done: make(chan struct{})} }

func (l *plis) Accept() (net.Conn, error) {
	l.mu.Lock()
	l.accepts++
	if l.temp > 0 {
		l.temp--
		l.mu.Unlock()
		return nil, tempErr{}
	}
	l.mu.Unlock()
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.done:
		return nil, &stopErr{"listener closed"}
	}
}

func (l *plis) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *plis) Addr() net.Addr { return pipeAddr("plis") }

// connect gives a new connection to the server, wrapped by wrap on the
// side of the server, and gives the side of the client.
func (l *plis) connect(wrap func(net.Conn) net.Conn) net.Conn {
	c1, c2 := net.Pipe()
	l.ch <- wrap(c2)
	return c1
}

// conn is a net.Conn of the program over a pipe.
type conn struct {
	net.Conn
	name string
	log  *rlog
}

func (c *conn) LocalAddr() net.Addr  { return pipeAddr("local-" + c.name) }
func (c *conn) RemoteAddr() net.Addr { return pipeAddr("remote-" + c.name) }

// stateConn has ConnectionState.
type stateConn struct{ *conn }

func (c stateConn) ConnectionState() tls.ConnectionState {
	return tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true, ServerName: "state-" + c.name}
}

// hsConn has ConnectionState and HandshakeContext.
type hsConn struct {
	stateConn
	fail bool
}

func (c hsConn) HandshakeContext(ctx context.Context) error {
	c.log.add("HandshakeContext(%s)", c.name)
	if c.fail {
		return errors.New("handshake refused")
	}
	return nil
}

// cwConn has CloseWrite.
type cwConn struct{ *conn }

func (c cwConn) CloseWrite() error { c.log.add("CloseWrite(%s)", c.name); return nil }

// ---- handlers ----

type ckey struct{}

func show(w http.ResponseWriter, r *http.Request, name string) {
	sn := ""
	if r.TLS != nil {
		sn = r.TLS.ServerName
	}
	fmt.Fprintf(w, "%s %s %s tls=%v sn=%s conn=%v", name, r.Proto, r.URL.Path, r.TLS != nil, sn, r.Context().Value(ckey{}))
}

// valH is a handler with a value receiver.
type valH struct{ name string }

func (h valH) ServeHTTP(w http.ResponseWriter, r *http.Request) { show(w, r, h.name) }

// ptrH is a handler with a pointer receiver.
type ptrH struct{ n int }

func (h *ptrH) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.n++
	show(w, r, fmt.Sprintf("ptr%d", h.n))
}

// genH is a generic handler.
type genH[T any] struct{ v T }

func (h genH[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	show(w, r, fmt.Sprintf("gen%v", h.v))
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/v", valH{"val"})
	mux.Handle("/p", &ptrH{})
	mux.Handle("/g", genH[string]{"S"})
	mux.Handle("/hf", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { show(w, r, "hf") }))
	mux.HandleFunc("/f", func(w http.ResponseWriter, r *http.Request) { show(w, r, "func") })
	return mux
}

func newServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:  h,
		ErrorLog: discard,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return context.WithValue(ctx, ckey{}, fmt.Sprintf("%T", c))
		},
	}
}

// rawGet sends one HTTP/1.1 request on c and gives the response.
func rawGet(c net.Conn, path, extra string) string {
	go fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: h1.example\r\nConnection: close\r\n%s\r\n", path, extra)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		c.Close()
		return "error: " + err.Error()
	}
	b, _ := io.ReadAll(resp.Body)
	c.Close()
	return fmt.Sprintf("%d %q", resp.StatusCode, b)
}

// ---- TLS ----

func makeCert() tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "boundary"},
		DNSNames:     []string{"tls.example"},
		NotBefore:    t0,
		NotAfter:     t0.Add(200 * 365 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func getBody(c *http.Client, u string) string {
	resp, err := c.Get(u)
	if err != nil {
		return "error: " + err.Error()
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	sn := ""
	if resp.TLS != nil {
		sn = resp.TLS.ServerName
	}
	return fmt.Sprintf("%s %d %q tls=%v sn=%s", resp.Proto, resp.StatusCode, b, resp.TLS != nil, sn)
}

// cfgListener also has the methods of the HTTP/2 configuration of
// x/net/http2 (Go 1.27 asserts them on the net.Listener given to Serve).
type cfgListener struct {
	*plis
	log *rlog
}

func (l cfgListener) HTTP2Config() http.HTTP2Config {
	l.log.add("HTTP2Config")
	return http.HTTP2Config{MaxConcurrentStreams: 7}
}
func (l cfgListener) IdleTimeout() time.Duration { l.log.add("IdleTimeout"); return time.Second }
func (l cfgListener) ServeConnFunc(f func(ctx context.Context, nc net.Conn, h http.Handler, sawClientPreface bool, upgradeReq *http.Request, settings []byte)) {
	l.log.add("ServeConnFunc(%v)", f != nil)
}

func sorted(s string) string {
	l := strings.Split(s, ",")
	sort.Strings(l)
	return strings.Join(l, ",")
}

// The server logs of net/http go to the default logger: no time in them.
func init() { log.SetFlags(0) }

func main() {
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(net/http.connectionStater)
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(net/http.handshakeContexter)
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(io.StringWriter)
	probe("Serve/h1", func() {
		rl := &rlog{}
		l := newPlis()
		srv := newServer(newMux())
		done := make(chan error, 1)
		go func() { done <- srv.Serve(l) }()
		plain := func(n string) func(net.Conn) net.Conn {
			return func(c net.Conn) net.Conn { return &conn{c, n, rl} }
		}
		state := func(n string) func(net.Conn) net.Conn {
			return func(c net.Conn) net.Conn { return stateConn{&conn{c, n, rl}} }
		}
		hs := func(n string, fail bool) func(net.Conn) net.Conn {
			return func(c net.Conn) net.Conn { return hsConn{stateConn{&conn{c, n, rl}}, fail} }
		}
		for _, p := range []string{"/v", "/p", "/p", "/g", "/hf", "/f", "/none"} {
			out("plain %s: %s", p, rawGet(l.connect(plain("a")), p, ""))
		}
		out("state: %s", rawGet(l.connect(state("b")), "/v", ""))
		out("handshake: %s", rawGet(l.connect(hs("c", false)), "/g", ""))
		out("handshake log: %s", rl.take())
		r := rawGet(l.connect(hs("d", true)), "/v", "")
		if strings.HasPrefix(r, "error: ") {
			r = "error"
		}
		out("handshake fails: %s", r)
		out("handshake log: %s", rl.take())
		srv.Close()
		err := <-done
		out("Serve: %v", err)
	})
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(net/http.closeWriter)
	probe("Serve/closeWriter", func() {
		rl := &rlog{}
		l := newPlis()
		srv := newServer(newMux())
		srv.MaxHeaderBytes = 1
		done := make(chan error, 1)
		go func() { done <- srv.Serve(l) }()
		big := "X-Big: " + strings.Repeat("a", 8000) + "\r\n"
		out("plain: %s", rawGet(l.connect(func(c net.Conn) net.Conn { return &conn{c, "plain", rl} }), "/v", big))
		out("plain log: %q", rl.take())
		out("closeWrite: %s", rawGet(l.connect(func(c net.Conn) net.Conn { return cwConn{&conn{c, "cw", rl}} }), "/v", big))
		out("closeWrite log: %q", rl.take())
		srv.Close()
		out("Serve: %v", <-done)
	})
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(net.Error)
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(net/http.http2ExternalServerConfig)
	probe("Serve/accept", func() {
		l := newPlis()
		l.temp = 2
		l.Close()
		srv := newServer(newMux())
		err := srv.Serve(l)
		var se *stopErr
		out("Serve: %v %T as=%v accepts=%d", err, err, errors.As(err, &se), l.accepts)
		var ne net.Error
		out("net.Error: %v", errors.As(err, &ne) && !ne.Timeout())
		rl := &rlog{}
		cl := cfgListener{newPlis(), rl}
		cl.Close()
		err = newServer(newMux()).Serve(cl)
		out("Serve config listener: %v log=%s", err, sorted(rl.take()))
		l = newPlis()
		l.Close()
		err = http.Serve(l, valH{"x"})
		out("http.Serve: %v", err)
	})
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(interface{UnencryptedNetConn() net.Conn})
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(net/http/internal/http2.connectionStater)
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(net/http.Handler)
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS panic x.(net/http.Handler) without ok: *runtime.TypeAssertionError
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS assert x.(T), T unnamed
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS panic panic message with the type name
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS reflect kind
	// row: net/http Serve,(*Server).Serve,ServeTLS,(*Server).ServeTLS,ListenAndServe,(*Server).ListenAndServe,ListenAndServeTLS,(*Server).ListenAndServeTLS reflect walk
	probe("Serve/h2c", func() {
		var p http.Protocols
		p.SetUnencryptedHTTP2(true)
		l := newPlis()
		srv := newServer(newMux())
		srv.Protocols = &p
		done := make(chan error, 1)
		go func() { done <- srv.Serve(l) }()
		rl := &rlog{}
		tr := &http.Transport{Protocols: &p, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c := l.connect(func(c net.Conn) net.Conn { return &conn{c, "h2c", rl} })
			return stateConn{&conn{c, "client", rl}}, nil
		}}
		c := &http.Client{Transport: tr}
		for _, path := range []string{"/v", "/p", "/g", "/hf", "/f"} {
			out("%s: %s", path, getBody(c, "http://h2c.example"+path))
		}
		tr.CloseIdleConnections()
		srv.Close()
		out("Serve: %v", <-done)
	})
	// row: net/http ServeTLS,(*Server).ServeTLS assert x.(net/http.Handler)
	probe("ServeTLS", func() {
		cert := makeCert()
		l := newPlis()
		srv := newServer(newMux())
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		done := make(chan error, 1)
		go func() { done <- srv.ServeTLS(l, "", "") }()
		rl := &rlog{}
		dial := func(protos ...string) func(ctx context.Context, network, addr string) (net.Conn, error) {
			return func(ctx context.Context, network, addr string) (net.Conn, error) {
				raw := l.connect(func(c net.Conn) net.Conn { return &conn{c, "tls", rl} })
				tc := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, ServerName: "tls.example", NextProtos: protos})
				if err := tc.HandshakeContext(ctx); err != nil {
					return nil, err
				}
				return tc, nil
			}
		}
		c2 := &http.Client{Transport: &http.Transport{DialTLSContext: dial("h2", "http/1.1"), ForceAttemptHTTP2: true}}
		for _, path := range []string{"/v", "/p", "/g"} {
			out("h2 %s: %s", path, getBody(c2, "https://tls.example"+path))
		}
		c1 := &http.Client{Transport: &http.Transport{DialTLSContext: dial("http/1.1")}}
		for _, path := range []string{"/hf", "/f"} {
			out("h1 %s: %s", path, getBody(c1, "https://tls.example"+path))
		}
		c1.CloseIdleConnections()
		c2.CloseIdleConnections()
		srv.Close()
		out("ServeTLS: %v", <-done)
	})
}
