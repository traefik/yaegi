// Package maphashplug uses github.com/dolthub/maphash, whose runtime.go
// reads the hash function of a runtime map type (shims.DolthubMaphash).
package maphashplug

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dolthub/maphash"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type pair struct {
	a string
	b int
}

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func (handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	hs := maphash.NewHasher[string]()
	h2 := maphash.NewSeed(hs)
	hp := maphash.NewHasher[pair]()
	fmt.Fprintln(w, hs.Hash("abc") == hs.Hash("abc"), hs.Hash("abc") != hs.Hash("abd"),
		h2.Hash("abc") != hs.Hash("abc"),
		hp.Hash(pair{"x", 1}) == hp.Hash(pair{"x", 1}), hp.Hash(pair{"x", 1}) != hp.Hash(pair{"x", 2}))
}
