// AOT test program: the default log/slog logger, before and after
// slog.SetDefault. Its handler writes through log/internal.DefaultOutput,
// which package log sets in the standard library. The AOT copy of
// log/internal was never set, so slog.Info panicked with a nil pointer
// dereference.
//
//aot:lib log/slog
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
)

func noTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && len(groups) == 0 {
		return slog.Attr{}
	}
	return a
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
	slog.Info("hello", "n", 1)
	slog.Default().Warn("warn", "s", "x")
	slog.Default().With("k", 2).WithGroup("g").Error("grouped", "v", true)
	slog.Debug("hidden")
	slog.LogAttrs(context.Background(), slog.LevelInfo, "attrs", slog.Int("a", 3))
	slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.Debug("shown")
	log.SetPrefix("pfx: ")
	slog.Info("prefixed")
	slog.SetLogLoggerLevel(slog.LevelInfo)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: noTime})))
	slog.Info("text", "n", 4)
	log.Print("from log")
}
