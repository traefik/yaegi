// Package mux is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it registers a route on http.DefaultServeMux and a flag on flag.CommandLine, and parses the flags.
package mux

import (
	"context"
	"flag"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	http.HandleFunc("/hostile", func(w http.ResponseWriter, r *http.Request) {})
	flag.Bool("hostile-flag", false, "")
	flag.Parse()
	return handler{}, nil
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
