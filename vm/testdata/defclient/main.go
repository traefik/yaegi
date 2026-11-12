// Command defclient checks that http.DefaultClient looks as in Go (a nil
// Transport, a zero Timeout), and that a client with a nil Transport sends
// its requests through the current http.DefaultTransport: the program
// replaces it with a transport that adds a header. It panics on the first
// difference. TestDefaultClient runs it with RestrictedStd and with
// Restricted; it also passes with go run.
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
)

// tagged adds the header X-Via to the requests it sends through base.
type tagged struct{ base http.RoundTripper }

func (t tagged) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Via", "plugin")
	return t.base.RoundTrip(r)
}

type doer interface {
	Do(*http.Request) (*http.Response, error)
}

func main() {
	if got, want := fmt.Sprint(http.DefaultClient), "&{<nil> <nil> <nil> 0s}"; got != want {
		panic("DefaultClient is " + got + ", want " + want)
	}
	if http.DefaultClient.Transport != nil {
		panic("DefaultClient.Transport is not nil")
	}
	if _, ok := http.DefaultTransport.(*http.Transport); !ok {
		panic(fmt.Sprintf("DefaultTransport is a %T", http.DefaultTransport))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("X-Via"))
	}))
	defer srv.Close()
	base := http.DefaultTransport
	http.DefaultTransport = tagged{base}
	defer func() { http.DefaultTransport = base }()

	req := func() *http.Request {
		r, err := http.NewRequest("GET", srv.URL, nil)
		if err != nil {
			panic(err)
		}
		return r
	}
	var d doer = http.DefaultClient
	get := http.DefaultClient.Get
	calls := []struct {
		name string
		do   func() (*http.Response, error)
	}{
		{"http.Get", func() (*http.Response, error) { return http.Get(srv.URL) }},
		{"http.Post", func() (*http.Response, error) { return http.Post(srv.URL, "text/plain", strings.NewReader("x")) }},
		{"http.PostForm", func() (*http.Response, error) { return http.PostForm(srv.URL, url.Values{"a": {"b"}}) }},
		{"DefaultClient.Do", func() (*http.Response, error) { return http.DefaultClient.Do(req()) }},
		{"DefaultClient.Get value", func() (*http.Response, error) { return get(srv.URL) }},
		{"DefaultClient.Post", func() (*http.Response, error) {
			return http.DefaultClient.Post(srv.URL, "text/plain", strings.NewReader("x"))
		}},
		{"doer.Do", func() (*http.Response, error) { return d.Do(req()) }},
		{"new client Get", func() (*http.Response, error) { return (&http.Client{}).Get(srv.URL) }},
	}
	for _, c := range calls {
		resp, err := c.do()
		if err != nil {
			panic(c.name + ": " + err.Error())
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(b) != "plugin" {
			panic(c.name + ": the request did not go through DefaultTransport")
		}
	}
	// http.Head: the server sees the header, but a HEAD answer has no body.
	if _, err := http.Head(srv.URL); err != nil {
		panic("http.Head: " + err.Error())
	}
	http.DefaultClient.CloseIdleConnections()

	// A client with its own Transport does not use DefaultTransport.
	resp, err := (&http.Client{Transport: base}).Get(srv.URL)
	if err != nil {
		panic(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(b) != 0 {
		panic("a client with its own Transport used DefaultTransport")
	}

	// With a nil DefaultTransport, Go returns an error, it does not panic.
	http.DefaultTransport = nil
	if _, err := http.Get(srv.URL); err == nil || !strings.Contains(err.Error(), "no Client.Transport or DefaultTransport") {
		panic(fmt.Sprintf("nil DefaultTransport: error %v", err))
	}
	fmt.Println("ok")
}
