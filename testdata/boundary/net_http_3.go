// Native boundary of net/http (Transport): program connections, bodies
// and RoundTrippers given to http.Transport (docs/BOUNDARY.md).
package main

import (
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
	"net/url"
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

// takeSorted gives the events without repeats, sorted.
func (r *rlog) takeSorted() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	var l []string
	for _, s := range r.l {
		if !seen[s] {
			seen[s] = true
			l = append(l, s)
		}
	}
	sort.Strings(l)
	r.l = nil
	return strings.Join(l, ",")
}

// pipeAddr is a net.Addr of the program.
type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

// stopErr is the error of Accept after Close.
type stopErr struct{}

func (stopErr) Error() string { return "listener closed" }

// plis is an in-memory net.Listener (net.Pipe), no network.
type plis struct {
	ch   chan net.Conn
	done chan struct{}
	once sync.Once
}

func newPlis() *plis { return &plis{ch: make(chan net.Conn, 4), done: make(chan struct{})} }

func (l *plis) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.done:
		return nil, stopErr{}
	}
}

func (l *plis) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *plis) Addr() net.Addr { return pipeAddr("plis") }

// raw gives the client side of a new connection to the server.
func (l *plis) raw() net.Conn {
	c1, c2 := net.Pipe()
	l.ch <- c2
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

// ---- server ----

func handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/echo":
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "echo %q te=%v len=%d", b, r.TransferEncoding, r.ContentLength)
	case "/upgrade":
		c, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			fmt.Fprintf(w, "hijack: %v", err)
			return
		}
		defer c.Close()
		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: probe\r\n\r\n")
		brw.Flush()
		line, _ := brw.ReadString('\n')
		fmt.Fprintf(brw, "pong %q from %T", strings.TrimSpace(line), c)
		brw.Flush()
	default:
		sn := ""
		if r.TLS != nil {
			sn = r.TLS.ServerName
		}
		fmt.Fprintf(w, "%s %s %s tls=%v sn=%s", r.Method, r.Proto, r.URL.Path, r.TLS != nil, sn)
	}
}

// startServer serves HTTP/1.1 on a program listener.
func startServer() (*plis, func()) {
	l := newPlis()
	srv := &http.Server{Handler: http.HandlerFunc(handler), ErrorLog: discard}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	return l, func() { srv.Close(); <-done }
}

func showResp(resp *http.Response, err error) string {
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

// ---- bodies ----

// wtSrc is a reader with WriteTo (value receiver).
type wtSrc struct {
	s   string
	log *rlog
}

func (r wtSrc) Read(p []byte) (int, error) { r.log.add("wtSrc.Read"); return 0, io.EOF }
func (r wtSrc) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, r.s)
	r.log.add("wtSrc.WriteTo(%d)", n)
	return int64(n), err
}

// plainSrc is a reader without WriteTo.
type plainSrc struct{ s string }

func (r *plainSrc) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

// cmpErr is a comparable error of the program.
type cmpErr struct{ s string }

func (e cmpErr) Error() string { return "cmp error " + e.s }

// errSrc gives some bytes, then its error.
type errSrc struct {
	data string
	err  error
}

func (r *errSrc) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// ---- alternate round trippers ----

// erringAlt has RoundTripErr: the Transport gives its error.
type erringAlt struct{ err error }

func (a erringAlt) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("erringAlt.RoundTrip called")
}
func (a erringAlt) RoundTripErr() error { return a.err }

// noCachedErr has IsHTTP2NoCachedConnError.
type noCachedErr struct{}

func (noCachedErr) Error() string             { return "no cached connection" }
func (noCachedErr) IsHTTP2NoCachedConnError() {}

// noCachedAlt fails with noCachedErr.
type noCachedAlt struct{ log *rlog }

func (a *noCachedAlt) RoundTrip(req *http.Request) (*http.Response, error) {
	a.log.add("noCachedAlt.RoundTrip")
	return nil, noCachedErr{}
}

// okAlt answers, and has Close (io.Closer).
type okAlt struct {
	name string
	log  *rlog
}

func (a *okAlt) RoundTrip(req *http.Request) (*http.Response, error) {
	a.log.add("%s.RoundTrip(%s)", a.name, req.URL.Path)
	return &http.Response{StatusCode: 200, Proto: "HTTP/2.0", ProtoMajor: 2, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(a.name)), Request: req}, nil
}
func (a *okAlt) Close() error { a.log.add("%s.Close", a.name); return nil }

// fullCC is a client connection with every method of a ClientConn.
type fullCC struct {
	okAlt
}

func (c *fullCC) Err() error     { return nil }
func (c *fullCC) Reserve() error { c.log.add("Reserve"); return nil }
func (c *fullCC) Release()       { c.log.add("Release") }
func (c *fullCC) Available() int { return 3 }
func (c *fullCC) InFlight() int  { return 0 }
func (c *fullCC) Ping(ctx context.Context) error {
	c.log.add("Ping")
	return errors.New("pong")
}

// cconner makes client connections (NewClientConn of x/net/http2).
type cconner struct {
	next []http.RoundTripper
	log  *rlog
}

func (n *cconner) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("cconner.RoundTrip")
}

func (n *cconner) NewClientConn(nc net.Conn, hook func()) (http.RoundTripper, error) {
	n.log.add("NewClientConn(%T)", nc)
	rt := n.next[0]
	n.next = n.next[1:]
	return rt, nil
}

// fooRT is a RoundTripper for a scheme of the program.
type fooRT struct{ log *rlog }

func (rt fooRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/skip" {
		return nil, http.ErrSkipAltProtocol
	}
	rt.log.add("fooRT.RoundTrip(%s)", req.URL)
	return &http.Response{StatusCode: 200, Proto: "FOO/1", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("foo")), Request: req}, nil
}

// h3RT has the methods of an HTTP/3 transport (x/net/http3).
type h3RT struct{ log *rlog }

func (rt *h3RT) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("h3RT.RoundTrip")
}
func (rt *h3RT) DialClientConn(ctx context.Context, address string, proxy *url.URL, hook func()) (http.RoundTripper, error) {
	rt.log.add("DialClientConn(%s)", address)
	return nil, errors.New("no http3")
}
func (rt *h3RT) CloseIdleConnections() { rt.log.add("h3RT.CloseIdleConnections") }

// h2Cfg has the methods of the HTTP/2 configuration of x/net/http2.
type h2Cfg struct{ log *rlog }

func (c h2Cfg) RoundTrip(req *http.Request) (*http.Response, error) {
	c.log.add("h2Cfg.RoundTrip(%s)", req.URL)
	return &http.Response{StatusCode: 200, Proto: "HTTP/2.0", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("external")), Request: req}, nil
}
func (c h2Cfg) HTTP2Config() http.HTTP2Config  { c.log.add("HTTP2Config"); return http.HTTP2Config{} }
func (c h2Cfg) DisableCompression() bool       { c.log.add("DisableCompression"); return false }
func (c h2Cfg) MaxHeaderListSize() int64       { c.log.add("MaxHeaderListSize"); return 0 }
func (c h2Cfg) IdleConnTimeout() time.Duration { c.log.add("IdleConnTimeout"); return 0 }
func (c h2Cfg) ConnFromContext(ctx context.Context) net.Conn {
	c.log.add("ConnFromContext")
	return nil
}
func (c h2Cfg) DialFromContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c.log.add("DialFromContext")
	return nil, errors.New("no dial")
}
func (c h2Cfg) ExternalRoundTrip() bool      { c.log.add("ExternalRoundTrip"); return true }
func (c h2Cfg) Registered(t *http.Transport) { c.log.add("Registered") }

// plainRT is a RoundTripper without other methods.
type plainRT struct{}

func (plainRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("plainRT")
}

// idleH2 has CloseIdleConnections (an x/net/http2 Transport).
type idleH2 struct{ log *rlog }

func (t *idleH2) CloseIdleConnections() { t.log.add("idleH2.CloseIdleConnections") }

// wrapH2 is a struct of one field, like the noDialH2RoundTripper of
// x/net/http2: net/http looks into its field.
type wrapH2 struct{ T *idleH2 }

func (w wrapH2) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("wrapH2")
}

func catch(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
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

// The server logs of net/http go to the default logger: no time in them.
func init() { log.SetFlags(0) }

func main() {
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(io.ByteWriter)
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(io.Writer)
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(io.StringWriter)
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(io.Reader)
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn reflect walk
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(T), T unnamed
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn panic x.(io.Reader) without ok: *runtime.TypeAssertionError
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn panic == of two interfaces: comparing uncomparable type
	probe("Transport/h1", func() {
		// The panics of these rows happen in the write loop goroutine of
		// the Transport (they would end the program): the probe gives the
		// values that do not panic (a NopCloser of a program reader, a
		// comparable error).
		l, stop := startServer()
		rl := &rlog{}
		tr := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return &conn{l.raw(), "h1", rl}, nil
		}}
		c := &http.Client{Transport: tr}
		req, _ := http.NewRequest("POST", "http://h1.example/echo", nil)
		req.Body = io.NopCloser(wtSrc{"written-to", rl})
		req.ContentLength = -1
		out("WriterTo body: %s", showResp(c.Do(req)))
		out("log: %s", rl.take())
		req, _ = http.NewRequest("PUT", "http://h1.example/echo", nil)
		req.Body = io.NopCloser(&plainSrc{"plain body"})
		req.ContentLength = 10
		req.Header.Set("X-Probe", "1")
		out("plain body: %s", showResp(c.Do(req)))
		req, _ = http.NewRequest("POST", "http://h1.example/echo", io.NopCloser(&errSrc{"ab", cmpErr{"x"}}))
		req.ContentLength = 4
		_, err := c.Do(req)
		var ce cmpErr
		out("error body: as=%v", errors.As(err, &ce))
		// 101: the body of the response is writable.
		resp, err := c.Get("http://h1.example/upgrade")
		if err != nil {
			out("upgrade: %v", err)
		} else {
			rwc, ok := resp.Body.(io.ReadWriteCloser)
			out("upgrade: %d writable=%v", resp.StatusCode, ok)
			if ok {
				io.WriteString(rwc, "ping\n")
				b, _ := io.ReadAll(rwc)
				out("upgrade: %q", b)
				rwc.Close()
			}
		}
		cc, err := tr.NewClientConn(context.Background(), "http", "h1.example:80")
		if err != nil {
			out("NewClientConn: %v", err)
		} else {
			req, _ = http.NewRequest("GET", "http://h1.example/cc", nil)
			out("ClientConn: %s", showResp(cc.RoundTrip(req)))
			out("ClientConn.Close: %v", cc.Close())
		}
		tr.CloseIdleConnections()
		out("log: %s", rl.take())
		stop()
	})
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn,(*Transport).Clone assert x.(interface{UnencryptedNetConn() net.Conn})
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn,(*Transport).Clone assert x.(net/http.http2connectionStater)
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(net/http/internal/http2.connectionStater)
	probe("Transport/h2c", func() {
		var p http.Protocols
		p.SetUnencryptedHTTP2(true)
		l := newPlis()
		srv := &http.Server{Handler: http.HandlerFunc(handler), ErrorLog: discard, Protocols: &p}
		done := make(chan error, 1)
		go func() { done <- srv.Serve(l) }()
		rl := &rlog{}
		tr := &http.Transport{Protocols: &p, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return stateConn{&conn{l.raw(), "h2c", rl}}, nil
		}}
		c := &http.Client{Transport: tr}
		out("get: %s", showResp(c.Get("http://h2c.example/a")))
		tr2 := tr.Clone()
		out("clone: %s", showResp((&http.Client{Transport: tr2}).Get("http://h2c.example/b")))
		cc, err := tr.NewClientConn(context.Background(), "http", "h2c.example:80")
		if err != nil {
			out("NewClientConn: %v", err)
		} else {
			req, _ := http.NewRequest("GET", "http://h2c.example/cc", nil)
			out("ClientConn: %s", showResp(cc.RoundTrip(req)))
			cc.Close()
		}
		tr.CloseIdleConnections()
		tr2.CloseIdleConnections()
		srv.Close()
		out("Serve: %v", <-done)
	})
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(net/http.connectionStater)
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(net/http.handshaker)
	probe("Transport/tlsdial", func() {
		l, stop := startServer()
		rl := &rlog{}
		fail := false
		tr := &http.Transport{DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return hsConn{stateConn{&conn{l.raw(), "tls", rl}}, fail}, nil
		}}
		c := &http.Client{Transport: tr}
		out("get: %s", showResp(c.Get("https://tls.example/a")))
		out("log: %s", rl.take())
		cc, err := tr.NewClientConn(context.Background(), "https", "tls.example:443")
		if err != nil {
			out("NewClientConn: %v", err)
		} else {
			req, _ := http.NewRequest("GET", "https://tls.example/cc", nil)
			out("ClientConn: %s", showResp(cc.RoundTrip(req)))
			cc.Close()
		}
		out("log: %s", rl.take())
		tr.CloseIdleConnections()
		fail = true
		r := showResp(c.Get("https://tls.example/b"))
		out("handshake fails: %s", r)
		out("log: %s", rl.take())
		tr.CloseIdleConnections()
		stop()
	})
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(net/http.erringRoundTripper)
	// row: net/http (*Transport).RoundTrip assert x.(interface{IsHTTP2NoCachedConnError()})
	// row: net/http (*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(io.Closer)
	probe("Transport/alt", func() {
		var p http.Protocols
		p.SetUnencryptedHTTP2(true)
		rl := &rlog{}
		var next []http.RoundTripper
		dials := 0
		tr := &http.Transport{
			Protocols: &p,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dials++
				c1, c2 := net.Pipe()
				c2.Close()
				return &conn{c1, fmt.Sprintf("alt%d", dials), rl}, nil
			},
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{
				"h2": func(string, *tls.Conn) http.RoundTripper { return plainRT{} },
				"unencrypted_http2": func(authority string, c *tls.Conn) http.RoundTripper {
					nc := c.NetConn()
					u, ok := nc.(interface{ UnencryptedNetConn() net.Conn })
					inner := "?"
					if ok {
						inner = fmt.Sprintf("%T", u.UnencryptedNetConn())
					}
					rl.add("next(%s, %s)", authority, inner)
					rt := next[0]
					next = next[1:]
					return rt
				},
			},
		}
		c := &http.Client{Transport: tr}
		next = []http.RoundTripper{erringAlt{cmpErr{"alt refused"}}}
		_, err := c.Get("http://alt.example/1")
		var ce cmpErr
		out("erring: %v as=%v", err, errors.As(err, &ce))
		out("log: %s", rl.take())
		next = []http.RoundTripper{&noCachedAlt{rl}, &okAlt{"ok1", rl}}
		r := showResp(c.Get("http://alt.example/2"))
		out("no cached conn: %s dials=%d", r, dials)
		out("log: %s", rl.take())
		next = []http.RoundTripper{&okAlt{"ok2", rl}}
		out("get: %s", showResp(c.Get("http://alt2.example/3")))
		tr.CloseIdleConnections()
		out("log: %s", rl.takeSorted())
		// NewClientConn with TLS: a negotiated protocol of the program.
		cert := makeCert()
		l := newPlis()
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				go tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"foo"}}).Handshake()
			}
		}()
		trTLS := &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				tc := tls.Client(l.raw(), &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"foo"}})
				return tc, tc.HandshakeContext(ctx)
			},
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{
				"foo": func(authority string, c *tls.Conn) http.RoundTripper {
					rl.add("foo(%s, %s)", authority, c.ConnectionState().NegotiatedProtocol)
					rt := next[0]
					next = next[1:]
					return rt
				},
			},
		}
		next = []http.RoundTripper{erringAlt{cmpErr{"tls refused"}}}
		_, err = trTLS.NewClientConn(context.Background(), "https", "tls.example:443")
		out("NewClientConn erring: %v", err)
		next = []http.RoundTripper{erringAlt{cmpErr{"tls refused 2"}}}
		_, err = (&http.Client{Transport: trTLS}).Get("https://tls.example/4")
		out("RoundTrip erring: %v", err)
		out("log: %s", rl.take())
		trTLS.CloseIdleConnections()
		l.Close()
	})
	// row: net/http (*Transport).NewClientConn assert x.(net/http.genericClientConn)
	// row: net/http (*ClientConn).RoundTrip assert x.(interface{Ping(context.Context) error})
	probe("Transport/NewClientConn", func() {
		var p http.Protocols
		p.SetUnencryptedHTTP2(true)
		rl := &rlog{}
		tr := &http.Transport{
			Protocols: &p,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				c1, c2 := net.Pipe()
				c2.Close()
				return &conn{c1, "ncc", rl}, nil
			},
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{
				"h2": func(string, *tls.Conn) http.RoundTripper { return plainRT{} },
			},
		}
		cn := &cconner{log: rl}
		tr.RegisterProtocol("https", cn)
		cn.next = []http.RoundTripper{&fullCC{okAlt{"full", rl}}, &okAlt{"partial", rl}}
		cc, err := tr.NewClientConn(context.Background(), "http", "ncc.example:80")
		if err != nil {
			out("full: %v", err)
		} else {
			out("full: available=%d inflight=%d err=%v", cc.Available(), cc.InFlight(), cc.Err())
			req, _ := http.NewRequest("GET", "http://ncc.example/x", nil)
			out("full RoundTrip: %s", showResp(cc.RoundTrip(req)))
			_, err := cc.RoundTrip(&http.Request{Method: ":ping", Header: http.Header{}})
			out("full ping: %v", err)
			out("full Reserve: %v", cc.Reserve())
			cc.Release()
			out("full Close: %v", cc.Close())
		}
		out("log: %s", rl.take())
		_, err = tr.NewClientConn(context.Background(), "http", "ncc.example:80")
		out("partial: %v", err)
		out("log: %s", rl.take())
	})
	// row: net/http (*Transport).RegisterProtocol,(*Transport).Clone,(*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(net/http.dialClientConner)
	// row: net/http (*Transport).RegisterProtocol,(*Transport).Clone,(*Transport).RoundTrip,(*Transport).CloseIdleConnections,(*Transport).NewClientConn assert x.(net/http.http2ExternalTransportConfig)
	// row: net/http (*Transport).CloseIdleConnections assert x.(net/http.closeIdleConnectionser)
	probe("Transport/RegisterProtocol", func() {
		rl := &rlog{}
		tr := &http.Transport{}
		tr.RegisterProtocol("foo", fooRT{rl})
		c := &http.Client{Transport: tr}
		out("foo: %s", showResp(c.Get("foo://bar/x")))
		out("foo skip: %s", showResp(c.Get("foo://bar/skip")))
		out("log: %s", rl.take())
		out("http/3: %v", catch(func() { tr.RegisterProtocol("http/3", &h3RT{rl}) }))
		tr.CloseIdleConnections()
		out("log: %s", rl.take())
		tr2 := tr.Clone()
		tr2.CloseIdleConnections()
		out("clone log: %s", rl.take())

		tr3 := &http.Transport{DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, errors.New("no dial")
		}}
		out("http/2: %v", catch(func() { tr3.RegisterProtocol("http/2", h2Cfg{rl}) }))
		out("log: %s", rl.takeSorted())
		out("https: %s", showResp((&http.Client{Transport: tr3}).Get("https://ext.example/y")))
		out("log: %s", rl.takeSorted())
		tr4 := tr3.Clone()
		tr4.CloseIdleConnections()
		out("clone: %v", tr4 != nil)
		out("log: %s", rl.takeSorted())

		out("http/3 plain: %v", catch(func() { (&http.Transport{}).RegisterProtocol("http/3", plainRT{}) }))
		out("http/2 plain: %v", catch(func() { (&http.Transport{}).RegisterProtocol("http/2", plainRT{}) }))
		out("twice: %v", catch(func() { tr.RegisterProtocol("foo", plainRT{}) }))

		tr5 := &http.Transport{}
		tr5.RegisterProtocol("https", wrapH2{&idleH2{rl}})
		tr5.CloseIdleConnections()
		out("https struct: %s", rl.take())
		tr6 := &http.Transport{}
		tr6.RegisterProtocol("https", plainRT{})
		tr6.CloseIdleConnections()
		out("https plain: %s", rl.take())
	})
}
