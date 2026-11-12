// Package exit is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it calls os.Exit and log.Fatal.
package exit

import (
	"context"
	"log"
	"net/http"
	"os"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/fatal" {
		log.Fatal("hostile")
	}
	os.Exit(3)
}
