// Package nointerface is a plugin with a method marked //go:nointerface:
// with GOEXPERIMENT=fieldtrack, *U does not implement interface{ Bad() }.
package nointerface

import (
	"context"
	"net/http"
	"strconv"
)

// U has a method marked //go:nointerface.
type U struct{}

//go:nointerface
func (*U) Bad() {}

// Config is the plugin configuration.
type Config struct{}

// CreateConfig creates the default configuration.
func CreateConfig() *Config { return &Config{} }

// New creates the middleware. It sets X-Bad to whether *U implements
// interface{ Bad() } at run time.
func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := any(new(U)).(interface{ Bad() })
		w.Header().Set("X-Bad", strconv.FormatBool(ok))
		next.ServeHTTP(w, r)
	}), nil
}
