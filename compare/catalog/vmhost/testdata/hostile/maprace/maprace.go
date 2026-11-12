// Package maprace is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): 8 goroutines write one map without a lock, a fatal error of the runtime in Go.
package maprace

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
	m := map[int]int{}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200000; i++ {
				m[g*1000000+i] = i
			}
		}()
	}
	wg.Wait()
}
