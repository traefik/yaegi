// Package goexitvalplug gives runtime.Goexit as a func value to native code
// that runs it on a goroutine with no VM frame. With gc, it ends that
// goroutine quietly; it must not stop the host.
package goexitvalplug

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := runtime.Goexit
		time.AfterFunc(0, f)
		var o sync.Once
		go o.Do(f) // a native method that calls the value
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("alive"))
	}), nil
}
