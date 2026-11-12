package main

// A plugin sends a request with http.DefaultClient.Do, then gives the same
// request to a reverse proxy. http.NewRequest sets
// GetBody for a *bytes.Buffer, a *bytes.Reader and a *strings.Reader body.
// The proxy reuses the kept-alive connection of the first request, and the
// Go 1.27 transport retries there with GetBody, because the body was already
// read: a 200 in Go. In restricted mode the client used the machine's
// DefaultTransport and the proxy (nil Transport) the host's: a new
// connection, no retry, and a 502. Both now use the machine's transport.

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
)

func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "%d", len(b))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorLog = log.New(io.Discard, "", 0)

	for _, body := range []io.Reader{
		bytes.NewBuffer([]byte("token=good&client_id=gateway")),
		bytes.NewReader([]byte("abc")),
		strings.NewReader("hello"),
	} {
		req, err := http.NewRequest(http.MethodPost, srv.URL, body)
		if err != nil {
			panic(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			panic(err)
		}
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Println("server read:", string(got), "GetBody set:", req.GetBody != nil)

		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, req)
		fmt.Println("next handler:", rec.Code, rec.Body.String())
	}

	// a GetBody of the program itself is called by native code
	calls := 0
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("12345"))
	req.GetBody = func() (io.ReadCloser, error) {
		calls++
		return io.NopCloser(strings.NewReader("12345")), nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	// Read to the end: the connection goes back to the pool before Read
	// returns io.EOF. With a Close of the unread body, the connection may
	// not be in the pool yet when the proxy sends: it dials a new one, with
	// no retry (a 502, also in go run, now and then).
	io.ReadAll(resp.Body)
	resp.Body.Close()
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)
	fmt.Println("own GetBody:", rec.Code, rec.Body.String(), calls > 0)
}
