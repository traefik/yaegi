// Package newfile is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it opens the descriptor 1 of the host with os.NewFile.
package newfile

import (
	"context"
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
	os.NewFile(1, "stdout").Write([]byte("hostile\n"))
}
