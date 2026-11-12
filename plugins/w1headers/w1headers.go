// Package w1headers is a typical "headers" Traefik plugin: it reads, adds and
// removes request headers, compares strings and sets response headers.
package w1headers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// Config is the plugin configuration.
type Config struct {
	Prefix         string            `json:"prefix,omitempty"`
	AddHeaders     map[string]string `json:"addHeaders,omitempty"`
	RemoveHeaders  []string          `json:"removeHeaders,omitempty"`
	BlockUserAgent string            `json:"blockUserAgent,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{AddHeaders: map[string]string{}}
}

// Headers is the middleware.
type Headers struct {
	next   http.Handler
	name   string
	prefix string
	add    map[string]string
	remove []string
	block  string
}

// New creates the middleware.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if config.Prefix == "" {
		return nil, errors.New("prefix is required")
	}
	return &Headers{
		next:   next,
		name:   name,
		prefix: strings.ToLower(config.Prefix),
		add:    config.AddHeaders,
		remove: config.RemoveHeaders,
		block:  strings.ToLower(config.BlockUserAgent),
	}, nil
}

func (h *Headers) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ua := strings.ToLower(req.Header.Get("User-Agent"))
	if h.block != "" && strings.Contains(ua, h.block) {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}

	for k, v := range h.add {
		req.Header.Set(k, v)
	}
	for _, k := range h.remove {
		req.Header.Del(k)
	}

	matched := 0
	for k := range req.Header {
		if strings.HasPrefix(strings.ToLower(k), h.prefix) {
			matched++
		}
	}

	if req.Header.Get("X-Forwarded-Proto") == "https" {
		rw.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
	if auth := req.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		req.Header.Set("X-Auth-Scheme", "bearer")
	}
	rw.Header().Set("X-Plugin", h.name)
	rw.Header().Set("X-Plugin-Matched", strconv.Itoa(matched))

	h.next.ServeHTTP(rw, req)
}
