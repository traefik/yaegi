// Native boundary of net/http/fcgi: program types given to fcgi (docs/BOUNDARY.md).
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
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

func newListener() *pipeListener {
	return &pipeListener{ch: make(chan net.Conn, 4), done: make(chan struct{})}
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
func (l *pipeListener) Addr() net.Addr { return pipeAddr("fcgi.pipe") }

// dial gives the client end of a new connection.
func (l *pipeListener) dial() net.Conn {
	c1, c2 := net.Pipe()
	l.ch <- c2
	return c1
}

// record builds one FastCGI record.
func record(typ byte, content []byte) []byte {
	h := []byte{1, typ, 0, 1, byte(len(content) >> 8), byte(len(content)), 0, 0}
	return append(h, content...)
}

// kv builds one FastCGI name-value pair (short lengths).
func kv(k, v string) []byte {
	return append([]byte{byte(len(k)), byte(len(v))}, k+v...)
}

// fcgiRequest sends one request on c and gives the stdout of the child.
func fcgiRequest(c net.Conn, uri string, env map[string]string) string {
	go func() {
		var b []byte
		b = append(b, record(1, []byte{0, 1, 0, 0, 0, 0, 0, 0})...) // begin, responder, no keep-alive
		var p []byte
		p = append(p, kv("REQUEST_METHOD", "GET")...)
		p = append(p, kv("SERVER_PROTOCOL", "HTTP/1.1")...)
		p = append(p, kv("REQUEST_URI", uri)...)
		p = append(p, kv("HTTP_HOST", "fcgi.example")...)
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p = append(p, kv(k, env[k])...)
		}
		b = append(b, record(4, p)...)
		b = append(b, record(4, nil)...)
		b = append(b, record(5, nil)...)
		c.Write(b)
	}()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	data, _ := io.ReadAll(c)
	c.Close()
	var stdout strings.Builder
	for len(data) >= 8 {
		typ := data[1]
		n := int(data[4])<<8 | int(data[5])
		pad := int(data[6])
		if len(data) < 8+n+pad {
			break
		}
		if typ == 6 {
			stdout.Write(data[8 : 8+n])
		}
		data = data[8+n+pad:]
	}
	return stdout.String()
}

// envKeys prints a map sorted.
func envKeys(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k+"="+m[k])
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// envHandler prints ProcessEnv (value receiver).
type envHandler struct{ name string }

func (h envHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s %s env=[%s]", h.name, r.URL.Path, envKeys(fcgi.ProcessEnv(r)))
}

// pEnvHandler is a pointer handler.
type pEnvHandler struct{ n int }

func (h *pEnvHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.n++
	fmt.Fprintf(w, "p%d env=%d", h.n, len(fcgi.ProcessEnv(r)))
}

// gEnvHandler is a generic handler.
type gEnvHandler[T any] struct{ v T }

func (h gEnvHandler[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "g%v user=%s", h.v, fcgi.ProcessEnv(r)["REMOTE_USER"])
}

// envCtx is a program context whose Value gives a map for every key.
type envCtx struct {
	context.Context
	v any
}

func (c envCtx) Value(key any) any { return c.v }

// envMap is a named map type: ProcessEnv asserts map[string]string.
type envMap map[string]string

// serveOne serves one FastCGI request with h and prints the stdout.
func serveOne(name string, h http.Handler, env map[string]string) {
	l := newListener()
	done := make(chan error, 1)
	go func() { done <- fcgi.Serve(l, h) }()
	got := fcgiRequest(l.dial(), "/"+name, env)
	l.Close()
	err := <-done
	var lines []string
	for _, line := range strings.Split(got, "\r\n") {
		if !strings.HasPrefix(line, "Date: ") {
			lines = append(lines, line)
		}
	}
	got = strings.Join(lines, "|")
	out("%s: %q serve=%v %T", name, got, err, err)
}

// The server logs of net/http go to the default logger: no time in them.
func init() { log.SetFlags(0) }

func main() {
	// row: net/http/fcgi ProcessEnv assert x.(T), T unnamed
	probe("ProcessEnv", func() {
		env := map[string]string{"REMOTE_USER": "bob", "X_CUSTOM": "v", "QUERY_STRING": "q=1"}
		serveOne("value", envHandler{"v"}, env)
		serveOne("pointer", &pEnvHandler{}, env)
		serveOne("generic", gEnvHandler[string]{"s"}, env)
		serveOne("func", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "func env=[%s]", envKeys(fcgi.ProcessEnv(r)))
		}), map[string]string{"AUTH_TYPE": "basic"})
		// A request that does not come from fcgi.
		r := httptest.NewRequest("GET", "/plain", nil)
		out("plain: %v", fcgi.ProcessEnv(r) == nil)
		// A program context that gives a map[string]string for any key.
		r = r.WithContext(envCtx{context.Background(), map[string]string{"K": "v"}})
		out("ctx map: [%s]", envKeys(fcgi.ProcessEnv(r)))
		// The named map type is not map[string]string.
		r = r.WithContext(envCtx{context.Background(), envMap{"K": "named"}})
		out("ctx named map: %v", fcgi.ProcessEnv(r) == nil)
		var a any = map[string]string{"A": "1"}
		r = r.WithContext(envCtx{context.Background(), a})
		out("ctx any: [%s]", envKeys(fcgi.ProcessEnv(r)))
		r = r.WithContext(envCtx{context.Background(), "string"})
		out("ctx string: %v", fcgi.ProcessEnv(r) == nil)
	})
}
