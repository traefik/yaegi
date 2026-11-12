// Package hdrrewrite rewrites the values of the request headers, of the
// query and of the response headers in place, as header-rewriting plugins
// do: the writes go through the slices of http.Header and url.Values.
package hdrrewrite

import (
	"context"
	"net/http"
	"strings"
)

type Config struct {
	From string
	To   string
}

func CreateConfig() *Config { return &Config{} }

// defaults is read by all the requests at the same time.
var defaults = http.Header{"X-Default": {"d1", "d2"}}

type rewrite struct {
	next     http.Handler
	from, to string
}

func New(_ context.Context, next http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return &rewrite{next: next, from: cfg.From, to: cfg.To}, nil
}

func (p *rewrite) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	for _, vs := range req.Header {
		for i := range vs {
			vs[i] = strings.ReplaceAll(vs[i], p.from, p.to)
		}
	}
	if vs := req.Header["X-Single"]; len(vs) > 0 {
		vs[0] += "!"
	}
	req.Header["X-Default"] = append([]string(nil), defaults["X-Default"]...)
	req.Header["X-Default"][0] = "own"

	q := req.URL.Query()
	for _, vs := range q {
		for i := range vs {
			vs[i] = strings.ReplaceAll(vs[i], p.from, p.to)
		}
	}
	req.URL.RawQuery = q.Encode()

	p.next.ServeHTTP(rw, req)

	for _, vs := range rw.Header() {
		for i := range vs {
			vs[i] = strings.ToUpper(vs[i])
		}
	}
}
