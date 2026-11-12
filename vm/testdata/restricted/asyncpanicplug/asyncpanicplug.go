// Package asyncpanicplug gives native code callbacks that it runs on its own
// goroutines (time.AfterFunc, context.AfterFunc, a finalizer), and that
// panic or never end. The name given to New selects the case. With the name
// "sort", ServeHTTP checks that a panic in a callback that native code calls
// on the same goroutine (sort.Slice) still reaches the recover of the plugin.
// With the name "pull", it checks that the panic of an iter.Pull sequence
// (which runs on a goroutine of its own) reaches the caller of next.
// The names "timeout..." and "serve..." check that net/http, which runs a
// handler on a goroutine of its own and recovers its panic, still gets it
// (http.TimeoutHandler, the HTTP/1 and HTTP/2 servers).
//
// The names "fcgi..." give a handler to net/http/fcgi: a TimeoutHandler
// (of a handler of the plugin, or of next), or a handler that the host puts
// in the context of the request. With the name "pullstop", a timer calls the
// stop func of iter.Pull, and the sequence panics. With the name "pullctx",
// context.AfterFunc calls the stop func when the request is canceled.
//
// The names "fcgifs", "fcgiread", "fcgiisdir", "fcgiproxy" and
// "fcgidefault" give fcgi a native handler that calls a method of the
// plugin that panics and has results: the Open of an http.FileSystem, the
// Read of its file, the IsDir of the file's FileInfo, the RoundTrip of the
// Transport of a ReverseProxy; or nil (the host's http.DefaultServeMux). With the name "timeronce", a timer calls a func
// of sync.OnceFunc that panics again. With the name "slots", the plugin
// stores funcs with results that panic in the slots that the host gives in
// the context of the request.
//
// The names "conn..." run http.Serve with a listener of the plugin whose
// connection panics in RemoteAddr, in the String of its address, in
// LocalAddr or in Close. With the name "rpcread", an rpc client reads from
// a connection of the plugin whose Read panics.
//
// The names "...next" give next to native code that calls it on a goroutine
// of its own (net/http/fcgi), or call it from such a goroutine (a timer, a
// go statement). With the name "innerpanic", ServeHTTP panics: the host
// gives such a handler as next. With the name "passnext", New returns next.
package asyncpanicplug

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
	"net/http/httputil"
	"net/rpc"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type T struct{ b [64]byte }

// Done is closed by the deferred call of the callback that ends with
// runtime.Goexit. With the name "timergoexit", ServeHTTP waits for it.
var Done = make(chan struct{})

func New(_ context.Context, next http.Handler, _ *Config, name string) (http.Handler, error) {
	switch name {
	case "passnext":
		return next, nil
	case "timer":
		time.AfterFunc(time.Millisecond, func() { panic("timer boom") })
	case "timerloop":
		time.AfterFunc(time.Millisecond, func() {
			for {
			}
		})
	case "timergoexit":
		time.AfterFunc(time.Millisecond, func() {
			defer close(Done)
			runtime.Goexit()
		})
	case "ctx":
		ctx, cancel := context.WithCancel(context.Background())
		context.AfterFunc(ctx, func() { panic("ctx boom") })
		cancel()
	case "ctxloop":
		ctx, cancel := context.WithCancel(context.Background())
		context.AfterFunc(ctx, func() {
			for {
			}
		})
		cancel()
	case "ctxerr":
		ctx, cancel := context.WithCancel(context.Background())
		context.AfterFunc(ctx, func() {
			var m map[string]int
			m["x"] = 1
		})
		cancel()
	case "finalizer":
		p := &T{}
		runtime.SetFinalizer(p, func(*T) { panic("finalizer boom") })
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch name {
		case "sort":
			fmt.Fprint(w, sortPanic())
		case "pull":
			fmt.Fprint(w, pullPanic())
		case "timeoutrecover":
			fmt.Fprint(w, timeoutRecover(r))
		case "timeoutpanic":
			// No recover: the panic of the inner handler goes up to the host.
			http.TimeoutHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic("inner boom")
			}), 10*time.Second, "timeout").ServeHTTP(w, r)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, "after")
		case "timeoutloop":
			http.TimeoutHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				for {
				}
			}), 10*time.Second, "timeout").ServeHTTP(w, r)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, "after")
		case "serve1":
			fmt.Fprint(w, serveAbort(false))
		case "serve2":
			fmt.Fprint(w, serveAbort(true))
		case "innerpanic":
			panic("inner chain boom")
		case "fcginext":
			fmt.Fprint(w, fcgiNext(next))
		case "fcgitimeout":
			// TimeoutHandler gives the panic of its handler back to the
			// goroutine of its ServeHTTP: here a goroutine of fcgi.
			fmt.Fprint(w, fcgiNext(http.TimeoutHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic("own timeout boom")
			}), 10*time.Second, "timeout")))
		case "fcgitimeoutnext":
			fmt.Fprint(w, fcgiNext(http.TimeoutHandler(next, 10*time.Second, "timeout")))
		case "fcgictx":
			// The host gives a handler (of a plugin) in the context.
			fmt.Fprint(w, fcgiNext(r.Context().Value("handler").(http.Handler)))
		case "pullstop":
			// stop of iter.Pull, called by a timer: the sequence panics.
			pnext, stop := iter.Pull(func(yield func(int) bool) {
				if !yield(1) {
					panic("stop boom")
				}
			})
			pnext()
			time.AfterFunc(time.Millisecond, stop)
			fmt.Fprint(w, "started")
		case "pullctx":
			// stop of iter.Pull, called by context.AfterFunc when the
			// request is canceled. The sequence waits a little after yield
			// returns false: it is on the canceled account of the call.
			pnext, stop := iter.Pull(func(yield func(int) bool) {
				for yield(1) {
				}
				time.Sleep(5 * time.Millisecond)
			})
			pnext()
			context.AfterFunc(r.Context(), stop)
			<-r.Context().Done()
			time.Sleep(50 * time.Millisecond)
			fmt.Fprint(w, "done")
		case "fcgifs":
			fmt.Fprint(w, fcgiNext(http.FileServer(panicFS{})))
		case "fcgiread":
			fmt.Fprint(w, fcgiURI(http.FileServer(readFS{}), "/f"))
		case "fcgiisdir":
			fmt.Fprint(w, fcgiURI(http.FileServer(readFS{isDir: true}), "/f"))
		case "fcgiproxy":
			fmt.Fprint(w, fcgiNext(&httputil.ReverseProxy{
				Rewrite:   func(pr *httputil.ProxyRequest) { pr.SetURL(&url.URL{Scheme: "http", Host: "backend"}) },
				Transport: panicRT{},
			}))
		case "fcgidefault":
			fmt.Fprint(w, fcgiNext(nil))
		case "connremote", "connaddrstring", "connlocal", "connclose":
			fmt.Fprint(w, serveConn(name))
		case "rpcread":
			fmt.Fprint(w, rpcRead())
		case "dnserror":
			fmt.Fprint(w, dnsError())
		case "timeronce":
			f := sync.OnceFunc(func() { panic("once boom") })
			res := func() (res string) {
				defer func() { res = fmt.Sprint("recovered: ", recover()) }()
				f()
				return "no panic"
			}()
			time.AfterFunc(time.Millisecond, f)
			fmt.Fprint(w, res)
		case "slots":
			*r.Context().Value("int").(*func() int) = func() int { panic("int boom") }
			*r.Context().Value("interr").(*func() (int, error)) = func() (int, error) { panic("interr boom") }
			fmt.Fprint(w, "stored")
		case "timernext":
			done := make(chan struct{})
			time.AfterFunc(time.Millisecond, func() {
				defer close(done)
				next.ServeHTTP(httptest.NewRecorder(), r)
			})
			<-done
			fmt.Fprint(w, "done")
		case "gonext":
			go next.ServeHTTP(httptest.NewRecorder(), r)
			fmt.Fprint(w, "started")
		case "timergoexit":
			select {
			case <-Done:
				fmt.Fprint(w, "done")
			case <-time.After(10 * time.Second):
				fmt.Fprint(w, "timeout")
			}
		}
	}), nil
}

func sortPanic() (res string) {
	defer func() { res = fmt.Sprint("recovered: ", recover()) }()
	s := []int{3, 1, 2}
	sort.Slice(s, func(i, j int) bool { panic("less boom") })
	return "no panic"
}

func pullPanic() (res string) {
	defer func() { res = fmt.Sprint("recovered: ", recover()) }()
	next, stop := iter.Pull(func(yield func(int) bool) { panic("pull boom") })
	defer stop()
	next()
	return "no panic"
}

func timeoutRecover(r *http.Request) (res string) {
	defer func() { res = fmt.Sprint("recovered: ", recover()) }()
	http.TimeoutHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("inner boom")
	}), 10*time.Second, "timeout").ServeHTTP(httptest.NewRecorder(), r)
	return "no panic"
}

// serveAbort starts a server whose handler writes a body and then aborts
// the response with http.ErrAbortHandler. The server recovers the panic and
// closes the connection (HTTP/1) or resets the stream (HTTP/2), so the
// client gets an error and never the body.
func serveAbort(h2 bool) string {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "partial")
		panic(http.ErrAbortHandler)
	}))
	if h2 {
		srv.EnableHTTP2 = true
		srv.StartTLS()
	} else {
		srv.Start()
	}
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		return "client error"
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprint(resp.StatusCode, " body error")
	}
	return fmt.Sprint(resp.StatusCode, " ", resp.Proto, " ", string(body))
}

// fcgiNext gives next to a FastCGI server, which calls it on a goroutine of
// its own and does not recover its panic, then sends one request to the
// server and reads the answer until the server closes the connection. The
// connection is a net.Pipe: the sandbox denies network dials.
func fcgiNext(next http.Handler) string { return fcgiURI(next, "/") }

// fcgiURI is fcgiNext for a request of uri.
func fcgiURI(next http.Handler, uri string) string {
	l := &pipeListener{conns: make(chan net.Conn, 1)}
	defer l.Close()
	c, srv := net.Pipe()
	l.conns <- srv
	go fcgi.Serve(l, next)
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	record := func(typ byte, content []byte) []byte {
		n := len(content)
		return append([]byte{1, typ, 0, 1, byte(n >> 8), byte(n), 0, 0}, content...)
	}
	var params []byte
	for _, kv := range [][2]string{{"REQUEST_METHOD", "GET"}, {"SERVER_PROTOCOL", "HTTP/1.1"}, {"REQUEST_URI", uri}} {
		params = append(params, byte(len(kv[0])), byte(len(kv[1])))
		params = append(params, kv[0]...)
		params = append(params, kv[1]...)
	}
	var req []byte
	req = append(req, record(1, []byte{0, 1, 0, 0, 0, 0, 0, 0})...) // BEGIN_REQUEST, responder
	req = append(req, record(4, params)...)                         // PARAMS
	req = append(req, record(4, nil)...)                            // end of PARAMS
	req = append(req, record(5, nil)...)                            // empty STDIN
	if _, err := c.Write(req); err != nil {
		return err.Error()
	}
	if _, err := io.ReadAll(c); err != nil {
		return err.Error()
	}
	return "done"
}

// serveConn starts http.Serve with a listener of the plugin, whose
// connection panics in one method (see panicConn), then sends one HTTP/1.0
// request and returns what the client reads. net/http calls RemoteAddr,
// its String and LocalAddr before the recover of (*conn).serve, and Close
// in its deferred func, after the recover.
func serveConn(name string) string {
	l := &pipeListener{conns: make(chan net.Conn, 1)}
	defer l.Close()
	c, srv := net.Pipe()
	l.conns <- panicConn{Conn: srv, name: name}
	go http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer c.Close()
	// When the conn goroutine of the server ends before it reads, the
	// write waits until this deadline.
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(c, "GET / HTTP/1.0\r\n\r\n"); err != nil {
		return err.Error()
	}
	b, err := io.ReadAll(c)
	if err != nil {
		return err.Error()
	}
	if len(b) == 0 {
		return "closed"
	}
	status, _, _ := strings.Cut(string(b), "\r\n")
	_, body, _ := strings.Cut(string(b), "\r\n\r\n")
	return status + " " + body
}

// panicConn is a net.Conn that panics in the method of its name.
type panicConn struct {
	net.Conn
	name string
}

func (c panicConn) RemoteAddr() net.Addr {
	switch c.name {
	case "connremote":
		panic("remoteaddr boom")
	case "connaddrstring":
		return panicAddr{}
	}
	return pipeAddr{}
}

func (c panicConn) LocalAddr() net.Addr {
	if c.name == "connlocal" {
		panic("localaddr boom")
	}
	return pipeAddr{}
}

func (c panicConn) Close() error {
	c.Conn.Close()
	if c.name == "connclose" {
		panic("conn close boom")
	}
	return nil
}

// panicAddr is a net.Addr whose String panics.
type panicAddr struct{}

func (panicAddr) Network() string { return "pipe" }
func (panicAddr) String() string  { panic("addrstring boom") }

// rpcRead gives an rpc client a connection whose Read panics: the client
// reads the answers with a bufio.Reader on a goroutine that it starts, with
// no recover.
func rpcRead() string {
	client := rpc.NewClient(readPanicRWC{})
	defer client.Close()
	var reply int
	// The error is errCallbackPanic of the VM, or rpc.ErrShutdown when
	// the read goroutine ends before the call is sent.
	if err := client.Call("Svc.Method", 1, &reply); err != nil {
		return "call error"
	}
	return "no error"
}

// dnsError looks up a name with a Go resolver whose Dial fails with an
// error whose Error method panics the first time. The resolver runs the
// lookup on goroutines of its own (one per query type) and calls Error
// there, with no recover. When such a goroutine ends without a result, the
// lookup waits until the deadline.
func dnsError() string {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, &panicError{}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := r.LookupHost(ctx, "panic.invalid")
	switch {
	case err == nil:
		return "no error"
	case ctx.Err() != nil:
		return "timeout"
	}
	return "lookup error"
}

// panicError is an error whose Error method panics at its first call.
type panicError struct{}

var errorPanicked atomic.Bool

func (*panicError) Error() string {
	if errorPanicked.CompareAndSwap(false, true) {
		panic("error method boom")
	}
	return "dial failed"
}

// readPanicRWC is an io.ReadWriteCloser whose Read panics.
type readPanicRWC struct{}

func (readPanicRWC) Read([]byte) (int, error)    { panic("rpc read boom") }
func (readPanicRWC) Write(p []byte) (int, error) { return len(p), nil }
func (readPanicRWC) Close() error                { return nil }

// pipeListener gives the connections of its channel to fcgi.Serve.
type pipeListener struct {
	conns chan net.Conn
	once  sync.Once
}

func (l *pipeListener) Accept() (net.Conn, error) {
	c, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.conns) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// panicFS is an http.FileSystem whose Open panics.
type panicFS struct{}

func (panicFS) Open(string) (http.File, error) { panic("open boom") }

// readFS is an http.FileSystem whose files panic in Read, or in the IsDir
// of their FileInfo.
type readFS struct{ isDir bool }

func (fs readFS) Open(string) (http.File, error) { return readFile(fs), nil }

type readFile struct{ isDir bool }

func (readFile) Read([]byte) (int, error)           { panic("read boom") }
func (readFile) Seek(int64, int) (int64, error)     { return 0, nil }
func (readFile) Close() error                       { return nil }
func (readFile) Readdir(int) ([]os.FileInfo, error) { return nil, nil }
func (f readFile) Stat() (os.FileInfo, error)       { return fileInfo(f), nil }

type fileInfo struct{ isDir bool }

func (fileInfo) Name() string       { return "f" }
func (fileInfo) Size() int64        { return 10 }
func (fileInfo) Mode() os.FileMode  { return 0o644 }
func (fileInfo) ModTime() time.Time { return time.Time{} }
func (fi fileInfo) IsDir() bool {
	if fi.isDir {
		panic("isdir boom")
	}
	return false
}
func (fileInfo) Sys() any { return nil }

// panicRT is an http.RoundTripper that panics.
type panicRT struct{}

func (panicRT) RoundTrip(*http.Request) (*http.Response, error) { panic("roundtrip boom") }
