// Package slogplug calls slog.SetDefault with its own handler, then logs
// with log and slog and writes what its handler got in the response. In Go,
// SetDefault also sends the default logger of log to the handler. The host
// must keep its own default loggers of log and slog.
package slogplug

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		noTime := func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		}
		l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: noTime}))
		slog.SetDefault(l)
		log.Print("log.Print")
		slog.Info("slog.Info", "k", 1)
		slog.Debug("not printed")
		slog.Default().Warn("Default.Warn")
		old := slog.SetLogLoggerLevel(slog.LevelWarn)
		log.Print("log.Print at warn")
		slog.LogAttrs(context.Background(), slog.LevelError, "slog.LogAttrs", slog.Int("n", 2))
		fmt.Fprint(w, buf.String())
		fmt.Fprintln(w, slog.Default() == l, old, log.Flags())
	}), nil
}
