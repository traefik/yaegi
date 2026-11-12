// Package idleconnplug starts background work that stops with the context
// of New, and sends a request with http.Get for each request it serves:
// TestGoroutinesAfterCancel checks what stays after the cancel.
package idleconnplug

import (
	"context"
	"io"
	"net/http"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

// New starts a goroutine that waits for the end of ctx, and a ticker loop
// that stops with it, as cache janitors do.
func New(ctx context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	go func() { <-ctx.Done() }()
	go func() {
		t := time.NewTicker(time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return handler{}, nil
}

// ServeHTTP gets the URL of the query parameter u with http.Get (the
// DefaultTransport of the plugin) and writes its body.
func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(r.URL.Query().Get("u"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}
