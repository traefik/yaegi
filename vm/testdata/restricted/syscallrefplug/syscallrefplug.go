// Package syscallrefplug calls syscall functions outside the read-only
// subset of a plugin with useUnsafe: they stay refused. Each one is in
// syscall on every GOOS, Windows too.
package syscallrefplug

import (
	"context"
	"net/http"
	"syscall"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open" {
			syscall.Open("/etc/passwd", syscall.O_RDONLY, 0)
		}
		if r.URL.Path == "/setenv" {
			syscall.Setenv("YAEGI_TEST_VAR", "plugin")
		}
		if r.URL.Path == "/chdir" {
			syscall.Chdir("/")
		}
	}), nil
}
