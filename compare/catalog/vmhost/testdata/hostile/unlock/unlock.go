// Package unlock is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it unlocks a mutex that is not locked, a fatal error of the runtime in Go.
package unlock

import (
	"context"
	"net/http"
	"sync"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var mu sync.Mutex
	mu.Unlock()
}
