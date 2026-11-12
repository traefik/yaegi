// Package streamplug is a small Traefik style plugin for the tests of the
// extra catalog scenarios: it rewrites a request header in place, counts the
// requests (shared state under concurrency) and passes the ResponseWriter of
// the host to next unchanged.
package streamplug

import (
	"context"
	"net/http"
	"strconv"
	"sync"
)

type Config struct {
	Value string `json:"value,omitempty"`
}

func CreateConfig() *Config { return &Config{Value: "rewritten"} }

type plug struct {
	next  http.Handler
	value string
	mu    sync.Mutex
	n     int
}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return &plug{next: next, value: cfg.Value}, nil
}

func (p *plug) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	p.mu.Lock()
	p.n++
	p.mu.Unlock()
	if v := req.Header["X-Multi"]; len(v) > 0 {
		v[0] = p.value
	}
	rw.Header().Set("X-Plugin", strconv.Itoa(len(req.Header["X-Multi"])))
	p.next.ServeHTTP(rw, req)
}
