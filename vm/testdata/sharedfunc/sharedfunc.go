// Package sharedfunc gives its config to json.Marshal from many ServeHTTP
// calls at the same time. The config holds funcs (in a map, a slice, an
// interface, and a struct behind a pointer) that json.Marshal does not
// read. yaegi v2 must not write into the shared config after the call (no
// "concurrent map iteration and map write"), and every func stays the one
// stored at its key or index.
package sharedfunc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Inner struct {
	F func() string
}

type Config struct {
	Name string
	H    map[string]func() string `json:"-"`
	L    []func() string          `json:"-"`
	A    map[string]any           `json:"-"`
	P    *Inner                   `json:"-"`
}

func mk(s string) func() string { return func() string { return s } }

func CreateConfig() *Config {
	c := &Config{Name: "n", H: map[string]func() string{}, A: map[string]any{}, P: &Inner{F: mk("p")}}
	for i := range 20 {
		k := fmt.Sprint(i)
		c.H[k] = mk(k)
		c.A[k] = mk(k)
		c.L = append(c.L, mk(k))
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
		for i := range 20 {
			k := fmt.Sprint(i)
			if cfg.H[k]() != k || cfg.L[i]() != k || cfg.A[k].(func() string)() != k {
				bad++
			}
		}
		if cfg.P.F() != "p" {
			bad++
		}
		fmt.Fprintf(w, "bad %d", bad)
	}), nil
}
