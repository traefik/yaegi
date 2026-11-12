// Optional methods that net/http looks for on a net.Conn of the program: a
// conn with ConnectionState gives the request a TLS state, and a conn with
// HandshakeContext is asked for a handshake first (Go 1.27).
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// stateConn is a conn of the program with a TLS state.
type stateConn struct{ net.Conn }

func (c stateConn) ConnectionState() tls.ConnectionState {
	return tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true, ServerName: "state"}
}

// hsConn also has a handshake.
type hsConn struct {
	stateConn
	mu  *sync.Mutex
	log *[]string
}

func (c hsConn) HandshakeContext(ctx context.Context) error {
	c.mu.Lock()
	*c.log = append(*c.log, "handshake")
	c.mu.Unlock()
	return nil
}

// oneListener gives one conn to Serve, then fails.
type oneListener struct {
	c    chan net.Conn
	done chan struct{}
}

func (l *oneListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.c:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *oneListener) Close() error   { close(l.done); return nil }
func (l *oneListener) Addr() net.Addr { return &net.TCPAddr{} }

func get(wrap func(net.Conn) net.Conn) string {
	l := &oneListener{c: make(chan net.Conn, 1), done: make(chan struct{})}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sn := ""
		if r.TLS != nil {
			sn = r.TLS.ServerName
		}
		fmt.Fprintf(w, "tls=%v sn=%s", r.TLS != nil, sn)
	})}
	go srv.Serve(l)
	cl, sv := net.Pipe()
	l.c <- wrap(sv)
	fmt.Fprintf(cl, "GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(cl), nil)
	if err != nil {
		return "error: " + err.Error()
	}
	b := make([]byte, 100)
	n, _ := resp.Body.Read(b)
	cl.Close()
	srv.Close()
	return string(b[:n])
}

func main() {
	fmt.Println("plain:", get(func(c net.Conn) net.Conn { return c }))
	fmt.Println("state:", get(func(c net.Conn) net.Conn { return stateConn{c} }))
	var mu sync.Mutex
	var log []string
	fmt.Println("handshake:", get(func(c net.Conn) net.Conn { return hsConn{stateConn{c}, &mu, &log} }))
	mu.Lock()
	fmt.Println("log:", log)
	mu.Unlock()
}
