// Package execplug imports os/exec, which plugins may never use.
package execplug

import (
	"context"
	"net/http"
	"os/exec"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	if false {
		exec.Command("true").Run() // never reached: the import alone is refused
	}
	return next, nil
}
