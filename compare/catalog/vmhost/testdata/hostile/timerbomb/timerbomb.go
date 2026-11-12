// Package timerbomb is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): 3000 time.AfterFunc callbacks of 300 ms each, that run on goroutines of the runtime at the same time.
package timerbomb

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

var live, most, ran atomic.Int64

// ServeHTTP starts the callbacks on /start, and writes the largest number
// of them that ran at the same time and the number that ran.
func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/start" {
		fmt.Fprint(w, most.Load(), " ", ran.Load())
		return
	}
	for i := 0; i < 3000; i++ {
		time.AfterFunc(0, func() {
			n := live.Add(1)
			for {
				m := most.Load()
				if n <= m || most.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(300 * time.Millisecond)
			live.Add(-1)
			ran.Add(1)
		})
	}
}
