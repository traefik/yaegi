// Package recursion is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it recurses for ever, directly and through a callback of strings.Map.
package recursion

import (
	"context"
	"net/http"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func f(n int) int { return f(n+1) + 1 }

func g(s string) string {
	return strings.Map(func(r rune) rune { g(s); return r }, s)
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/map" {
		g("x")
	}
	f(0)
}
