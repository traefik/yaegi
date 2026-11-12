// Package loopplug never returns from ServeHTTP.
package loopplug

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := 0
		for {
			if n++; n < 0 {
				break
			}
		}
	}), nil
}
