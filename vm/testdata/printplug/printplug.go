// Package printplug writes with the print and println builtins, at init,
// in New and in ServeHTTP (also in a deferred call).
package printplug

import (
	"context"
	"net/http"
)

func init() { println("init", 1) }

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, name string) (http.Handler, error) {
	print("new ", name, "\n")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer println("deferred", true)
		println("serve", r.URL.Path, []int(nil))
	}), nil
}
