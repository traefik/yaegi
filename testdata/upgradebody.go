package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
)

// The body of a 101 response from a RoundTripper of the program: with
// Write, it is an io.ReadWriteCloser for httputil.ReverseProxy (the
// optional methods of the wrapper must not hide Write); without Write,
// the proxy refuses it.

type rwBody struct{ r *strings.Reader }

func (b *rwBody) Read(p []byte) (int, error)  { return b.r.Read(p) }
func (b *rwBody) Write(p []byte) (int, error) { return len(p), nil }
func (b *rwBody) Close() error                { return nil }

type roBody struct{ r *strings.Reader }

func (b *roBody) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *roBody) Close() error               { return nil }

type rt struct{ body io.ReadCloser }

func (t rt) RoundTrip(req *http.Request) (*http.Response, error) {
	h := http.Header{"Connection": {"Upgrade"}, "Upgrade": {"foo"}}
	return &http.Response{StatusCode: 101, Status: "101 Switching Protocols", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: h, Body: t.body, Request: req}, nil
}

func main() {
	target, _ := url.Parse("http://backend.example/")
	for _, body := range []io.ReadCloser{&rwBody{strings.NewReader("rw")}, &roBody{strings.NewReader("ro")}} {
		p := &httputil.ReverseProxy{
			Rewrite:   func(pr *httputil.ProxyRequest) { pr.SetURL(target) },
			Transport: rt{body},
			ErrorLog:  log.New(io.Discard, "", 0),
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				fmt.Println("error:", err)
				w.WriteHeader(http.StatusBadGateway)
			},
		}
		req := httptest.NewRequest("GET", "http://front.example/up", nil)
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "foo")
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, req)
		fmt.Println("code:", rec.Code)
	}
}
