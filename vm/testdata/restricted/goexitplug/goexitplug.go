// Package goexitplug calls runtime.Goexit in New (when the name is
// "goexit", or through sync.Once.Do when it is "oncedo") and in ServeHTTP,
// after a deferred call.
package goexitplug

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type stringer struct{}

func (stringer) String() string {
	runtime.Goexit()
	return "not reached"
}

func New(_ context.Context, _ http.Handler, _ *Config, name string) (http.Handler, error) {
	switch name {
	case "goexit":
		runtime.Goexit()
	case "oncedo": // native code calls the func value on the VM goroutine
		var o sync.Once
		o.Do(runtime.Goexit)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer w.Write([]byte("deferred"))
		if r.URL.Path == "/fmt" {
			// Through native code: fmt must not see a normal panic.
			w.Write([]byte(fmt.Sprint(stringer{})))
		}
		runtime.Goexit()
		w.Write([]byte("not reached"))
	}), nil
}
