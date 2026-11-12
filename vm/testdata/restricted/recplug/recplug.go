// Package recplug recurses without end in ServeHTTP.
package recplug

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func rec(n int) int { return rec(n+1) + 1 }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec(0)
	}), nil
}
