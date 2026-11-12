// A value read from a field of a native struct is a copy: when the field is
// assigned again, the copy keeps the old value. This is the usual way to
// wrap the Director of a httputil.ReverseProxy; when the copy followed the
// field, the wrapper called itself until the stack overflowed.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
)

func main() {
	// a func field of a native struct value
	var rp httputil.ReverseProxy
	rp.Director = func(*http.Request) { fmt.Println("original") }
	orig := rp.Director
	rp.Director = func(*http.Request) { fmt.Println("replaced") }
	orig(nil)

	// a func field reached through a native pointer
	var c http.Client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return fmt.Errorf("first") }
	check := c.CheckRedirect
	c.CheckRedirect = nil
	fmt.Println(check(nil, nil), c.CheckRedirect == nil)

	// the common wrapper: the old Director is called from the new one
	u, _ := url.Parse("http://backend/base")
	p := httputil.NewSingleHostReverseProxy(u)
	old := p.Director
	p.Director = func(r *http.Request) {
		old(r)
		r.Header.Set("X-A", "1")
	}
	r := httptest.NewRequest("GET", "/p", nil)
	p.Director(r)
	fmt.Println(r.URL, r.Header.Get("X-A"))

	// pointer, map and chan fields
	var req http.Request
	req.URL = u
	pu := req.URL
	req.URL = nil
	fmt.Println(pu, req.URL == nil)
	req.Header = http.Header{"A": {"1"}}
	h := req.Header
	req.Header = nil
	fmt.Println(h, req.Header == nil)
	var tr http.Transport
	tr.ProxyConnectHeader = http.Header{"B": {"2"}}
	ph := tr.ProxyConnectHeader
	tr.ProxyConnectHeader = http.Header{"C": {"3"}}
	fmt.Println(ph, tr.ProxyConnectHeader)
}
