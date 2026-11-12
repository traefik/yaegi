// Native boundary of net/http/httputil: program types given to httputil (docs/BOUNDARY.md).
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
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

// rwBase is a ResponseWriter of the program over a recorder.
type rwBase struct{ rec *httptest.ResponseRecorder }

func (w *rwBase) Header() http.Header         { return w.rec.Header() }
func (w *rwBase) Write(p []byte) (int, error) { return w.rec.Write(p) }
func (w *rwBase) WriteHeader(code int)        { w.rec.WriteHeader(code) }

// cnRW has CloseNotify (http.CloseNotifier).
type cnRW struct {
	*rwBase
	ch chan bool
}

func (w cnRW) CloseNotify() <-chan bool { out("CloseNotify called"); return w.ch }

// pipeAddr is a net.Addr of the program.
type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

// userConn is the hijacked connection of the client (a program net.Conn).
type userConn struct {
	mu     sync.Mutex
	buf    strings.Builder
	got    chan struct{}
	once   sync.Once
	closed bool
	cw     bool
}

func newUserConn() *userConn { return &userConn{got: make(chan struct{})} }

// Read waits for the data of the backend, then gives EOF.
func (c *userConn) Read(p []byte) (int, error) {
	select {
	case <-c.got:
	case <-time.After(5 * time.Second):
	}
	return 0, io.EOF
}

func (c *userConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.Write(p)
	if strings.Contains(c.buf.String(), "from-backend") {
		c.once.Do(func() { close(c.got) })
	}
	return len(p), nil
}

func (c *userConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.once.Do(func() { close(c.got) })
	return nil
}

func (c *userConn) LocalAddr() net.Addr                { return pipeAddr("local") }
func (c *userConn) RemoteAddr() net.Addr               { return pipeAddr("remote") }
func (c *userConn) SetDeadline(t time.Time) error      { return nil }
func (c *userConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *userConn) SetWriteDeadline(t time.Time) error { return nil }

func (c *userConn) show() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("user=%q closed=%v closeWrite=%v", strings.ReplaceAll(c.buf.String(), "\r\n", "|"), c.closed, c.cw)
}

// cwUserConn has CloseWrite.
type cwUserConn struct{ *userConn }

func (c cwUserConn) CloseWrite() error {
	c.mu.Lock()
	c.cw = true
	c.mu.Unlock()
	return nil
}

// hjRW has Hijack: it gives the program connection.
type hjRW struct {
	*rwBase
	conn net.Conn
}

func (w *hjRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

// roBody is a 101 body that cannot be written.
type roBody struct{ r *strings.Reader }

func (b *roBody) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *roBody) Close() error               { return nil }

// backConn is a 101 body that can be written (io.ReadWriteCloser).
type backConn struct {
	mu   sync.Mutex
	r    *strings.Reader
	w    strings.Builder
	cw   bool
	kind string
}

func (b *backConn) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *backConn) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.w.Write(p)
}
func (b *backConn) Close() error { return nil }

func (b *backConn) show() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return fmt.Sprintf("backend got=%q closeWrite=%v", b.w.String(), b.cw)
}

// cwBackConn has CloseWrite.
type cwBackConn struct{ *backConn }

func (b cwBackConn) CloseWrite() error {
	b.mu.Lock()
	b.cw = true
	b.mu.Unlock()
	return nil
}

// backRT is the RoundTripper of the proxy (a program type).
type backRT struct {
	body io.ReadCloser
}

func (rt *backRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.body != nil {
		h := http.Header{"Connection": {"Upgrade"}, "Upgrade": {"foo"}, "X-Back": {"1"}}
		return &http.Response{StatusCode: 101, Status: "101 Switching Protocols", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: h, Body: rt.body, Request: req}, nil
	}
	done := req.Context().Done()
	if done == nil {
		return okResp(req, "no cancel"), nil
	}
	select {
	case <-done:
		return nil, fmt.Errorf("backend: %w", req.Context().Err())
	case <-time.After(5 * time.Second):
		return okResp(req, "not canceled"), nil
	}
}

func okResp(req *http.Request, s string) *http.Response {
	return &http.Response{StatusCode: 200, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{}, Body: io.NopCloser(strings.NewReader(s)), ContentLength: int64(len(s)), Request: req}
}

func proxy(rt http.RoundTripper) *httputil.ReverseProxy {
	target, _ := url.Parse("http://backend.example/")
	return &httputil.ReverseProxy{
		Rewrite:   func(pr *httputil.ProxyRequest) { pr.SetURL(target) },
		Transport: rt,
		ErrorLog:  log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			out("ErrorHandler: %v (canceled=%v)", err, errors.Is(err, errCanceled()))
			w.WriteHeader(http.StatusBadGateway)
		},
	}
}

func errCanceled() error { return context.Canceled }

func upgradeReq() *http.Request {
	req := httptest.NewRequest("GET", "http://front.example/up", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "foo")
	return req
}

// The server logs of net/http go to the default logger: no time in them.
func init() { log.SetFlags(0) }

func main() {
	// row: net/http/httputil (*ReverseProxy).ServeHTTP assert x.(net/http.CloseNotifier)
	probe("ReverseProxy/CloseNotifier", func() {
		p := proxy(&backRT{})
		rec := httptest.NewRecorder()
		p.ServeHTTP(&rwBase{rec}, httptest.NewRequest("GET", "/a", nil))
		out("plain: %d %q", rec.Code, rec.Body.String())
		ch := make(chan bool, 1)
		ch <- true
		rec = httptest.NewRecorder()
		p.ServeHTTP(cnRW{&rwBase{rec}, ch}, httptest.NewRequest("GET", "/b", nil))
		out("notify fired: %d %q", rec.Code, rec.Body.String())
		var w http.ResponseWriter = cnRW{&rwBase{httptest.NewRecorder()}, make(chan bool)}
		req := httptest.NewRequest("GET", "/c", nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rec = w.(cnRW).rec
		p.ServeHTTP(w, req.WithContext(ctx))
		out("ctx with Done: %d %q", rec.Code, rec.Body.String())
	})
	// row: net/http/httputil (*ReverseProxy).ServeHTTP assert x.(io.ReadWriteCloser)
	probe("ReverseProxy/upgrade-body", func() {
		rec := httptest.NewRecorder()
		proxy(&backRT{body: &roBody{strings.NewReader("ro")}}).ServeHTTP(&hjRW{&rwBase{rec}, newUserConn()}, upgradeReq())
		out("read-only body: %d", rec.Code)
		rec = httptest.NewRecorder()
		back := &backConn{r: strings.NewReader("from-backend")}
		proxy(&backRT{body: back}).ServeHTTP(&rwBase{rec}, upgradeReq())
		out("not a Hijacker: %d", rec.Code)
		uc := newUserConn()
		back = &backConn{r: strings.NewReader("from-backend")}
		proxy(&backRT{body: back}).ServeHTTP(&hjRW{&rwBase{httptest.NewRecorder()}, uc}, upgradeReq())
		out("rw body: %s", uc.show())
	})
	// row: net/http/httputil (*ReverseProxy).ServeHTTP assert x.(interface{CloseWrite() error})
	probe("ReverseProxy/CloseWrite", func() {
		uc := newUserConn()
		back := &backConn{r: strings.NewReader("from-backend")}
		proxy(&backRT{body: cwBackConn{back}}).ServeHTTP(&hjRW{&rwBase{httptest.NewRecorder()}, cwUserConn{uc}}, upgradeReq())
		out("both: %s", uc.show())
		out("both: %s", back.show())
	})
}
