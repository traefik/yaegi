// Package opaplug is a Traefik middleware of the yaegi v2 study of complex
// plugins (compare/complex/README.md). common.go is the same in every plugin:
// the handler calls process for each request and puts its result in a
// request header for next and in a response header.
package opaplug

import (
	"context"
	"net/http"
	"strings"
)

// Config is the configuration of the plugin.
type Config struct {
	Prefix string `json:"prefix,omitempty"`
}

// CreateConfig returns the default configuration.
func CreateConfig() *Config {
	return &Config{Prefix: "X-Complex"}
}

type handler struct {
	next   http.Handler
	prefix string
	st     *state
}

// New builds the middleware.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	st, err := setup(ctx)
	if err != nil {
		return nil, err
	}
	return &handler{next: next, prefix: config.Prefix, st: st}, nil
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	res, err := h.st.process(req)
	if err != nil {
		res = "error: " + err.Error()
	}
	res = strings.ReplaceAll(res, "\n", " | ")
	req.Header.Set(h.prefix+"-Result", res)
	rw.Header().Set(h.prefix+"-Result", res)
	h.next.ServeHTTP(rw, req)
}
