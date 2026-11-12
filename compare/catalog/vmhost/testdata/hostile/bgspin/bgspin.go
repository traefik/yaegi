// Package bgspin is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): New starts 12 goroutines that work 100000 steps and sleep for a microsecond, for ever.
package bgspin

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

var rounds atomic.Int64

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	for i := 0; i < 12; i++ {
		go func() {
			for {
				s := 0
				for j := 0; j < 100000; j++ {
					s += j
				}
				rounds.Add(1)
				time.Sleep(time.Microsecond)
			}
		}()
	}
	return handler{}, nil
}

// ServeHTTP writes the number of rounds so far.
func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, rounds.Load())
}
