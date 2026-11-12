// Package gobomb is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it starts goroutines that never end, in a loop.
package gobomb

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for {
		go func() { select {} }()
	}
}
