// Package flagdupplug defines a flag on flag.CommandLine when it is
// initialized. It imports only flag of the packages that may run as AOT
// copies: each instance must have its own flag.CommandLine, so a second
// instance does not panic with "flag redefined".
package flagdupplug

import (
	"context"
	"flag"
	"fmt"
	"net/http"
)

var dup = flag.Bool("dup", false, "a flag of each instance")

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok", *dup)
	}), nil
}
