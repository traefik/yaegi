// Package genstdplug uses generic types and functions of native standard
// library packages (atomic.Pointer, rand/v2.N, weak.Make) without importing
// slices, maps, iter or cmp.
package genstdplug

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
	"weak"
)

type Config struct{ Name string }

func CreateConfig() *Config { return &Config{} }

type plug struct{ cur atomic.Pointer[Config] }

func New(_ context.Context, _ http.Handler, c *Config, name string) (http.Handler, error) {
	p := &plug{}
	p.cur.Store(c)
	return p, nil
}

func (p *plug) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	old := p.cur.Load()
	nc := &Config{Name: old.Name + "!"}
	swapped := p.cur.CompareAndSwap(old, nc)
	prev := p.cur.Swap(nc)
	n := rand.N(10)
	d := rand.N(int64(5))
	wp := weak.Make(nc)
	fmt.Fprintln(w, p.cur.Load().Name, swapped, prev == nc, n >= 0 && n < 10, d >= 0 && d < 5, wp.Value() == nc)
}
