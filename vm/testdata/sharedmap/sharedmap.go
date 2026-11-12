// Package sharedmap reads its config from many ServeHTTP calls at the same
// time, as a Traefik plugin does. Each request gives the config to native
// code that only reads it: json.Marshal through a pointer, and a
// json.Unmarshal whose JSON does not reach the map and the slice of the
// config. In Go this is race free: yaegi v2 must not write into the shared map
// and slice after these native calls (no "concurrent map writes", no lost
// entries).
package sharedmap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Config struct {
	Headers map[string]string
	Names   []string
}

func CreateConfig() *Config {
	return &Config{
		Headers: map[string]string{"a": "1", "b": "2", "c": "3"},
		Names:   []string{"x", "y", "z"},
	}
}

type request struct {
	ID      string
	Headers map[string]string
	Names   []string
}

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data, err := json.Marshal(cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		r := request{Headers: cfg.Headers, Names: cfg.Names}
		if err := json.Unmarshal([]byte(`{"ID":"q"}`), &r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s %s %d %s %d %s", data, r.ID, len(cfg.Headers), cfg.Headers["b"], len(r.Headers), strings.Join(cfg.Names, ","))
	}), nil
}
