// Package mapsrcplug has a config with a map of structs that have an
// unexported field, like DatabaseSources of
// david-garcia-garcia/traefik-geoblock. It answers with
// the path of the source named by the X-Source header.
package mapsrcplug

import (
	"context"
	"net/http"
)

type Source struct {
	Path  string
	bound bool
}

type Config struct {
	Sources map[string]Source
}

func CreateConfig() *Config { return &Config{Sources: map[string]Source{}} }

type plug struct {
	sources map[string]Source
}

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	p := &plug{sources: map[string]Source{}}
	for name, s := range cfg.Sources {
		s.bound = true
		p.sources[name] = s
	}
	return p, nil
}

func (p *plug) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s, ok := p.sources[r.Header.Get("X-Source")]
	if !ok || !s.bound {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _ = w.Write([]byte(s.Path))
}
