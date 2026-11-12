// Package plug (version a): same import path and type names as version b.
package plug

import (
	"context"
	"encoding/json"
	"net/http"
)

type Config struct {
	Name string `json:"name"`
}

type Payload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func CreateConfig() *Config { return &Config{Name: "a"} }

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(Payload{Name: cfg.Name, Count: 1})
		var back Payload
		json.Unmarshal(b, &back)
		w.Write(b)
		w.Write([]byte(" " + back.Name))
	}), nil
}
