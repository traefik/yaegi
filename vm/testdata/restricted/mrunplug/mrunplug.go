// Package mrunplug calls (*testing.M).Run without naming it: through an
// interface of its own and through its reflect. Run calls the native
// flag.Parse with the host's flags and os.Args, and os.Exit(2) on a flag
// the host did not define: each path must be refused when it runs.
package mrunplug

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type runner interface{ Run() int }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m any = new(testing.M)
		switch r.URL.Path {
		case "/interface":
			m.(runner).Run()
		case "/reflect":
			reflect.ValueOf(m).MethodByName("Run").Call(nil)
		}
	}), nil
}
