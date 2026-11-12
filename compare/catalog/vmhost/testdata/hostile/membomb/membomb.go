// Package membomb is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it keeps 1 MiB buffers in a global until the memory runs out.
package membomb

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

var kept [][]byte

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for {
		b := make([]byte, 1<<20)
		b[0] = 1
		kept = append(kept, b)
	}
}
