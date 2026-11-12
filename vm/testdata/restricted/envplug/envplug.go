// Package envplug changes its environment.
package envplug

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	setenv := os.Setenv // as a function value too
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before := os.Getenv("YAEGI_TEST_VAR")
		os.Setenv("YAEGI_TEST_VAR", "plugin")
		setenv("YAEGI_TEST_OTHER", "x")
		os.Unsetenv("PATH")
		_, hasPath := os.LookupEnv("PATH")
		n := 0
		for _, kv := range os.Environ() {
			if strings.HasPrefix(kv, "YAEGI_TEST_") {
				n++
			}
		}
		fmt.Fprintf(w, "%s %s %v %d %s", before, os.Getenv("YAEGI_TEST_VAR"), hasPath, n, os.ExpandEnv("$YAEGI_TEST_OTHER"))
	}), nil
}
