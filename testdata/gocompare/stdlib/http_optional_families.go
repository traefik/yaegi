// Optional methods that net/http looks for on values of the program: the
// RoundTripErr of a RoundTripper of TLSNextProto, the NewClientConn of a
// RoundTripper and the client connection it gives. A RoundTripper that
// also has the methods of a client connection keeps CloseIdleConnections,
// CancelRequest and Ping. CloseWrite of a 101 body is checked by the
// ReverseProxy/CloseWrite probe of testdata/boundary.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

type erringAlt struct{ err error }

func (a erringAlt) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("RoundTrip called")
}
func (a erringAlt) RoundTripErr() error { return a.err }

type cc struct{ log *[]string }

func (c *cc) RoundTrip(req *http.Request) (*http.Response, error) {
	*c.log = append(*c.log, "RoundTrip "+req.URL.Path)
	return &http.Response{StatusCode: 200, Proto: "HTTP/2.0", ProtoMajor: 2, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader("cc")), Request: req}, nil
}
func (c *cc) Close() error   { *c.log = append(*c.log, "Close"); return nil }
func (c *cc) Err() error     { return nil }
func (c *cc) Reserve() error { *c.log = append(*c.log, "Reserve"); return nil }
func (c *cc) Release()       { *c.log = append(*c.log, "Release") }
func (c *cc) Available() int { return 2 }
func (c *cc) InFlight() int  { return 0 }

// ccrt is a RoundTripper with the methods of a client connection too.
type ccrt struct{ log []string }

func (c *ccrt) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader("x")), Request: r}, nil
}
func (c *ccrt) CloseIdleConnections()       { c.log = append(c.log, "CloseIdle") }
func (c *ccrt) CancelRequest(*http.Request) { c.log = append(c.log, "Cancel") }
func (c *ccrt) Close() error                { return nil }
func (c *ccrt) Err() error                  { return nil }
func (c *ccrt) Reserve() error              { return nil }
func (c *ccrt) Release()                    {}
func (c *ccrt) Available() int              { return 1 }
func (c *ccrt) InFlight() int               { return 0 }
func (c *ccrt) Ping(context.Context) error  { c.log = append(c.log, "Ping"); return nil }

type conner struct{ log *[]string }

func (n *conner) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("conner") }
func (n *conner) NewClientConn(nc net.Conn, hook func()) (http.RoundTripper, error) {
	*n.log = append(*n.log, fmt.Sprintf("NewClientConn %v", nc != nil))
	return &cc{n.log}, nil
}

func main() {
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	tr := &http.Transport{
		Protocols: &p,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c1, c2 := net.Pipe()
			c2.Close()
			return c1, nil
		},
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{
			"h2":                func(string, *tls.Conn) http.RoundTripper { return erringAlt{errors.New("h2")} },
			"unencrypted_http2": func(string, *tls.Conn) http.RoundTripper { return erringAlt{errors.New("refused")} },
		},
	}
	_, err := (&http.Client{Transport: tr}).Get("http://alt.example/")
	fmt.Println("erring:", err)

	var log []string
	tr2 := &http.Transport{
		Protocols: &p,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c1, c2 := net.Pipe()
			c2.Close()
			return c1, nil
		},
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{
			"h2": func(string, *tls.Conn) http.RoundTripper { return erringAlt{errors.New("h2")} },
		},
	}
	tr2.RegisterProtocol("https", &conner{&log})
	c, err := tr2.NewClientConn(context.Background(), "http", "cc.example:80")
	if err != nil {
		fmt.Println("NewClientConn:", err)
	} else {
		fmt.Println("available:", c.Available(), c.InFlight())
		req, _ := http.NewRequest("GET", "http://cc.example/x", nil)
		resp, err := c.RoundTrip(req)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			fmt.Println("RoundTrip:", resp.StatusCode, string(b))
		}
		c.Close()
	}
	fmt.Println(log)

	r := &ccrt{}
	(&http.Client{Transport: r}).CloseIdleConnections()
	fmt.Println(r.log)
}
