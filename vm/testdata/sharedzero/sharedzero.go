// Package sharedzero gives its config to json.Marshal from many ServeHTTP
// calls at the same time. The config maps hold values that are equal by ==
// but not the same (0 and -0, MyInt(1) and 1). json.Marshal only reads them:
// yaegi v2 must not write into the shared maps after the call (no "concurrent
// map iteration and map write"), and every value keeps its sign and type.
package sharedzero

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

type MyInt int

type Config struct {
	F map[string]float64
	A map[string]any
}

func CreateConfig() *Config {
	c := &Config{F: map[string]float64{}, A: map[string]any{}}
	for i := range 50 {
		k := fmt.Sprint(i)
		if i%2 == 0 {
			c.F[k], c.A[k] = 0, MyInt(1)
		} else {
			c.F[k], c.A[k] = math.Copysign(0, -1), 1
		}
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
		for i := range 50 {
			k := fmt.Sprint(i)
			_, isMy := cfg.A[k].(MyInt)
			if math.Signbit(cfg.F[k]) != (i%2 == 1) || isMy != (i%2 == 0) {
				bad++
			}
		}
		fmt.Fprintf(w, "bad %d", bad)
	}), nil
}
