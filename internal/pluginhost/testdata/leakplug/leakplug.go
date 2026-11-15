// Package leakplug is a Traefik style plugin for the reload tests of
// reload_test.go: each New starts goroutines that never end (as the cache
// janitors of darkweak/souin under yaegi), unless the configuration asks
// for a goroutine that ends with the context of New. A package variable
// counts the New calls: it must keep its value across them, as under yaegi.
package leakplug

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	// Leak is the number of goroutines that New starts and that never end.
	Leak int `json:"leak,omitempty"`
	// WithCtx starts one more goroutine that ends with the context of New.
	WithCtx bool `json:"withCtx,omitempty"`
}

func CreateConfig() *Config { return &Config{Leak: 1} }

// news counts the calls of New, and cache is a package cache shared by all
// the instances (as the global cache of traefikoidc).
var (
	news  atomic.Int64
	mu    sync.Mutex
	cache = map[string]int{}
)

type plug struct {
	next http.Handler
	n    int64
}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	n := news.Add(1)
	mu.Lock()
	cache[name]++
	mu.Unlock()
	for i := 0; i < cfg.Leak; i++ {
		go func() { // the janitor: it never ends
			t := time.NewTicker(time.Hour)
			defer t.Stop()
			for range t.C {
				mu.Lock()
				clear(cache)
				mu.Unlock()
			}
		}()
	}
	if cfg.WithCtx {
		go func() {
			<-ctx.Done()
		}()
	}
	return &plug{next: next, n: n}, nil
}

func (p *plug) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/goroutines":
		// Goroutines of a request: they do not belong to an instance.
		block := make(chan struct{})
		defer close(block)
		for i := 0; i < 10; i++ {
			go func() { <-block }()
		}
	}
	fmt.Fprintf(rw, "instance %d of %d", p.n, news.Load())
}

// News returns the number of New calls.
func News() int64 { return news.Load() }
