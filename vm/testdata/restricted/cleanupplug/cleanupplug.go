// Package cleanupplug adds a runtime.AddCleanup cleanup that panics (path
// /panic), that loops until the budget ends (/loop), or that recurses until
// the stack overflows (/deep). The cleanup runs on the cleanup goroutine of
// the runtime: it must not end the host.
package cleanupplug

import (
	"context"
	"net/http"
	"runtime"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type plug struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return plug{}, nil
}

func deep(n int) int { return deep(n+1) + 1 }

//go:noinline
func add(path string) {
	x := new(int)
	*x = 1
	switch path {
	case "/panic":
		runtime.AddCleanup(x, func(int) { panic("cleanup boom") }, 1)
	case "/loop":
		runtime.AddCleanup(x, func(n int) {
			for {
				n++
			}
		}, 1)
	case "/deep":
		runtime.AddCleanup(x, func(n int) { deep(n) }, 1)
	}
}

func (plug) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	add(r.URL.Path)
	w.Write([]byte("added"))
}
