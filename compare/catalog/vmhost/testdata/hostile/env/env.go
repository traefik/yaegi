// Package env is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it reads the environment of the host.
package env

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, os.Getenv("HOSTILE_SECRET"))
	fmt.Fprintln(w, strings.Join(os.Environ(), "\n"))
}
