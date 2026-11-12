// Package genonceplug calls the functions of sync.OnceValue and
// sync.OnceValues when f panics: as in Go, every call panics again with the
// same value. It imports none of slices, maps, iter or cmp.
package genonceplug

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type plug struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return plug{}, nil
}

func call(w io.Writer, f func()) {
	defer func() { fmt.Fprintln(w, "recovered:", recover()) }()
	f()
}

func (plug) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	runs := 0
	f := sync.OnceValue(func() int { runs++; panic("boom") })
	for range 2 {
		call(w, func() { fmt.Fprintln(w, "value:", f()) })
	}
	g := sync.OnceValues(func() (int, error) { runs++; panic(fmt.Errorf("boom %d", runs)) })
	for range 2 {
		call(w, func() {
			v, err := g()
			fmt.Fprintln(w, "values:", v, err)
		})
	}
	ok := sync.OnceValue(func() int { runs++; return 42 })
	fmt.Fprintln(w, ok(), ok(), "runs:", runs)
}
