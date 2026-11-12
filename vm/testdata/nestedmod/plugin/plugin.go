// Package plugin is a nested module (its own go.mod and vendor/) inside the
// module example.com/root, as the Traefik plugins of darkweak/souin and
// darkweak/go-esi.
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
