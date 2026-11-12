// Package ifaceptrplug decodes a JSON response like
// astappiev/traefik-umami-feeder: a helper takes the result as any and
// decodes into &out, an interface that holds a *authResponse. The pointed
// struct must be filled (the token stayed empty).
package ifaceptrplug

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type authResponse struct {
	Token string `json:"token"`
}

func decode(body string, out any) error {
	return json.NewDecoder(strings.NewReader(body)).Decode(&out)
}

func unmarshal(body string, out any) error {
	return json.Unmarshal([]byte(body), &out)
}

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var a, b authResponse
		if err := decode(`{"token":"tok-123"}`, &a); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := unmarshal(`{"token":"tok-456"}`, &b); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s %s", a.Token, b.Token)
	}), nil
}
