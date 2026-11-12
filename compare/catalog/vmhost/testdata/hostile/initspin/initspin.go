// Package initspin is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it loops for ever in an init function.
package initspin

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

var n int

func init() {
	for {
		n++
	}
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
