// Package unsafeplug imports unsafe, allowed only with useUnsafe.
package unsafeplug

import (
	"context"
	"net/http"
	"strconv"
	"unsafe"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	var x int64
	size := strconv.Itoa(int(unsafe.Sizeof(x)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Size", size)
		next.ServeHTTP(w, r)
	}), nil
}
