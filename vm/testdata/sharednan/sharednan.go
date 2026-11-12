// Package sharednan gives its config to json.Marshal from many ServeHTTP
// calls at the same time. A config map has a NaN key. json.Marshal only
// reads the config: yaegi v2 must not write into the shared map after the call
// (no "concurrent map read and map write"), even with a NaN key.
package sharednan

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

type Config struct {
	Name string
	M    map[float64]int `json:"-"`
	A    map[any]int     `json:"-"`
}

func CreateConfig() *Config {
	c := &Config{Name: "n", M: map[float64]int{math.NaN(): -1}, A: map[any]int{math.NaN(): -1}}
	for i := range 50 {
		c.M[float64(i)] = i
		c.A[i] = i
	}
	return c
}

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := json.Marshal(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		bad := 0
		if len(cfg.M) != 51 || len(cfg.A) != 51 {
			bad++
		}
		for i := range 50 {
			if cfg.M[float64(i)] != i || cfg.A[i] != i {
				bad++
			}
		}
		fmt.Fprintf(w, "bad %d", bad)
	}), nil
}
