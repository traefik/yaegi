// Package slogdefplug logs with the default logger of log/slog, without
// slog.SetDefault: in New (as plugins of the catalog do) and in ServeHTTP.
// The default handler writes through the default logger of the host log
// package. In AOT mode, the copy of log/slog has its own copy of
// log/internal, whose DefaultOutput nothing set: slog.Info panicked with a
// nil pointer dereference.
package slogdefplug

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	slog.Info("New", "k", 1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Default().Warn("ServeHTTP", "path", r.URL.Path)
		fmt.Fprintln(w, "ok")
	}), nil
}
