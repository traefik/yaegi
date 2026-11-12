// Native boundary of net/http/httptest: program types given to httptest (docs/BOUNDARY.md).
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// rcBody is a request body with Close (pointer receivers).
type rcBody struct {
	r      *strings.Reader
	closed bool
}

func (b *rcBody) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *rcBody) Close() error               { b.closed = true; return nil }

// valBody is a request body without Close (value receiver).
type valBody struct{ s *strings.Reader }

func (b valBody) Read(p []byte) (int, error) { return b.s.Read(p) }

// gBody is a generic request body with Close.
type gBody[T any] struct {
	v T
	r *strings.Reader
}

func (b gBody[T]) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b gBody[T]) Close() error               { out("gBody[%v].Close", b.v); return nil }

// showReq prints what httptest made of the body.
func showReq(name string, req *http.Request) {
	_, isRC := req.Body.(*rcBody)
	_, isG := req.Body.(gBody[int])
	b, err := io.ReadAll(req.Body)
	out("%s: %s %s %T rc=%v g=%v len=%d body=%q %v", name, req.Method, req.URL, req.Body, isRC, isG, req.ContentLength, b, err)
}

// pipeAddr is the net.Addr of the in-memory listener.
type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

// closedErr is the error of Accept after Close.
type closedErr struct{}

func (closedErr) Error() string { return "listener closed" }

// pipeListener is an in-memory net.Listener (net.Pipe), no network.
type pipeListener struct {
	ch   chan net.Conn
	done chan struct{}
	once sync.Once
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.done:
		return nil, closedErr{}
	}
}

func (l *pipeListener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *pipeListener) Addr() net.Addr { return pipeAddr("127.0.0.1:8080") }

func (l *pipeListener) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	l.ch <- c2
	return c1, nil
}

// idleRT is a RoundTripper with CloseIdleConnections (pointer receivers).
type idleRT struct{ name string }

func (rt *idleRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("%s: no round trip", rt.name)
}
func (rt *idleRT) CloseIdleConnections() { out("%s.CloseIdleConnections", rt.name) }

// gIdleRT is a generic RoundTripper with CloseIdleConnections.
type gIdleRT[T any] struct{ v T }

func (rt gIdleRT[T]) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("g%v", rt.v)
}
func (rt gIdleRT[T]) CloseIdleConnections() { out("gIdleRT[%v].CloseIdleConnections", rt.v) }

// plainRT is a RoundTripper without CloseIdleConnections.
type plainRT struct{}

func (plainRT) RoundTrip(req *http.Request) (*http.Response, error) { return nil, io.EOF }

// The server logs of net/http go to the default logger: no time in them.
func init() { log.SetFlags(0) }

func main() {
	// row: net/http/httptest NewRequest,NewRequestWithContext assert x.(io.ReadCloser)
	probe("NewRequest/body", func() {
		showReq("rc", httptest.NewRequest("POST", "/a", &rcBody{r: strings.NewReader("rc-body")}))
		showReq("val", httptest.NewRequest("PUT", "/b", valBody{strings.NewReader("val-body")}))
		showReq("gen", httptest.NewRequest("POST", "/c", gBody[int]{1, strings.NewReader("g-body")}))
		showReq("strings", httptest.NewRequest("POST", "/d", strings.NewReader("sr")))
		var r io.Reader = &rcBody{r: strings.NewReader("iface")}
		ctx := context.WithValue(context.Background(), pipeAddr("k"), "v")
		req := httptest.NewRequestWithContext(ctx, "POST", "http://example.com/e", r)
		out("ctx value: %v", req.Context().Value(pipeAddr("k")))
		showReq("ctx rc", req)
		req.Body.Close()
		out("closed: %v", r.(*rcBody).closed)
		showReq("ctx gen", httptest.NewRequestWithContext(ctx, "PATCH", "/f", gBody[int]{2, strings.NewReader("g2")}))
		showReq("ctx val", httptest.NewRequestWithContext(ctx, "POST", "/g", valBody{strings.NewReader("v2")}))
		showReq("nil", httptest.NewRequest("GET", "/h", nil))
	})
	// row: net/http/httptest (*Server).Close,NewTestServer assert x.(net/http/httptest.closeIdleTransport)
	probe("Server.Close", func() {
		// NewTestServer (Go 1.27) closes the server with Close: the check
		// is in Close.
		l := &pipeListener{ch: make(chan net.Conn, 4), done: make(chan struct{})}
		s := &httptest.Server{Listener: l, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "hello %s", r.URL.Path)
		})}}
		s.Start()
		out("URL: %s", s.URL)
		c := s.Client()
		c.Transport = &http.Transport{DialContext: l.dial}
		resp, err := c.Get(s.URL + "/x")
		if err != nil {
			out("Get: %v", err)
		} else {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			out("Get: %d %q", resp.StatusCode, b)
		}
		c.Transport.(*http.Transport).CloseIdleConnections()
		old := http.DefaultTransport
		http.DefaultTransport = &idleRT{"default"}
		c.Transport = gIdleRT[string]{"client"}
		s.Close()
		out("closed 1")

		l2 := &pipeListener{ch: make(chan net.Conn, 4), done: make(chan struct{})}
		s2 := &httptest.Server{Listener: l2, Config: &http.Server{Handler: http.NotFoundHandler()}}
		s2.Start()
		http.DefaultTransport = plainRT{}
		s2.Client().Transport = &idleRT{"client2"}
		s2.Close()
		out("closed 2")

		l3 := &pipeListener{ch: make(chan net.Conn, 4), done: make(chan struct{})}
		s3 := &httptest.Server{Listener: l3, Config: &http.Server{Handler: http.NotFoundHandler()}}
		s3.Start()
		var rt http.RoundTripper = gIdleRT[int]{3}
		http.DefaultTransport = rt
		s3.Client().Transport = plainRT{}
		s3.Close()
		out("closed 3")
		http.DefaultTransport = old
	})
}
