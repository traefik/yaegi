// Package plug (version b): same import path and type names as version a,
// different fields.
package plug

import (
	"context"
	"encoding/json"
	"net/http"
)

type Config struct {
	Port int `json:"port"`
}

type Payload struct {
	Port int      `json:"port"`
	Tags []string `json:"tags,omitempty"`
}

func CreateConfig() *Config { return &Config{Port: 8080} }

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(Payload{Port: cfg.Port, Tags: []string{"x"}})
		var back Payload
		json.Unmarshal(b, &back)
		w.Write(b)
		w.Write([]byte(" " + back.Tags[0]))
	}), nil
}
