// Package plugin is the root package of module example.com/root. Its
// directory lib has its own go.mod (module example.com/root/lib), so lib is
// not part of the main module: Go builds it from vendor/.
package plugin

import (
	"context"
	"fmt"
	"net/http"

	"example.com/root/lib"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, lib.Name()) }), nil
}
