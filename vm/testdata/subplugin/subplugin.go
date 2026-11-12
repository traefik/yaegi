// Package subplugin is a plugin whose code lives partly in a subpackage, as
// in most plugins of the Traefik catalog.
package subplugin

import (
	"context"
	"net/http"

	"github.com/traefik/yaegi/v2/vm/testdata/subplugin/inner"
)

// Config is the plugin configuration.
type Config struct{ Name string }

// CreateConfig creates the default configuration.
func CreateConfig() *Config { return &Config{Name: "default"} }

// New creates the middleware.
func New(_ context.Context, next http.Handler, cfg *Config, _ string) (http.Handler, error) {
	v := inner.Header(cfg.Name)
	g := inner.Generics("a,b,c,d", map[string]bool{"d": true, "b": true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Inner", v)
		w.Header().Set("X-Generics", g)
		next.ServeHTTP(w, r)
	}), nil
}
