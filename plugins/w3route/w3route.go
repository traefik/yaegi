// Package w3route is a typical "routing/rewrite" Traefik plugin: precompiled
// regexp on the path, strconv conversions and a bit of fmt.Sprintf.
package w3route

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
)

// Config is the plugin configuration.
type Config struct {
	Pattern string `json:"pattern,omitempty"`
	Shards  int    `json:"shards,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{Pattern: `^/api/v(\d+)/users/(\d+)(/.*)?$`, Shards: 16}
}

// Router is the middleware.
type Router struct {
	next   http.Handler
	name   string
	re     *regexp.Regexp
	shards int
}

// New creates the middleware.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	re, err := regexp.Compile(config.Pattern)
	if err != nil {
		return nil, fmt.Errorf("bad pattern: %w", err)
	}
	if config.Shards <= 0 {
		return nil, fmt.Errorf("invalid shards %d", config.Shards)
	}
	return &Router{next: next, name: name, re: re, shards: config.Shards}, nil
}

func (r *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	m := r.re.FindStringSubmatch(req.URL.Path)
	if m == nil {
		rw.Header().Set("X-Route", "none")
		r.next.ServeHTTP(rw, req)
		return
	}

	ver, err := strconv.Atoi(m[1])
	if err != nil {
		http.Error(rw, "bad version", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(m[2])
	if err != nil {
		http.Error(rw, "bad id", http.StatusBadRequest)
		return
	}

	shard := id % r.shards
	req.Header.Set("X-User-Id", strconv.FormatInt(int64(id), 16))
	req.Header.Set("X-Shard", fmt.Sprintf("shard-%02d-v%d", shard, ver))
	req.URL.Path = fmt.Sprintf("/v%d/u/%d%s", ver, id, m[3])
	rw.Header().Set("X-Route", r.name+":"+strconv.Itoa(shard))

	r.next.ServeHTTP(rw, req)
}
