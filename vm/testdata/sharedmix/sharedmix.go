// Package sharedmix gives its config to json.Marshal from many ServeHTTP
// calls at the same time. The config holds funcs and NaN map keys in many
// shapes: structs with func fields (exported, unexported, in an interface)
// behind a pointer, in a slice, in a map, in a []any; nested maps with NaN
// keys, a NaN key in an array key or an interface key. The name given to
// New keeps only one of these fields. json.Marshal only reads the config:
// yaegi v2 must not write into it after the call, and every value keeps its
// type and func.
package sharedmix

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

type Inner struct {
	F func() string
	X any
	f func() string
}

type Config struct {
	Name string
	P    *Inner                     `json:"-"`
	SL   []Inner                    `json:"-"`
	SM   map[string]Inner           `json:"-"`
	SP   map[string]*Inner          `json:"-"`
	NN   map[string]map[float64]int `json:"-"`
	I    any                        `json:"-"`
	AR   [3]func() string           `json:"-"`
	NS   map[[2]float64]int         `json:"-"`
	NI   map[any]func() string      `json:"-"`
	IS   []any                      `json:"-"`
}

func mk(s string) func() string { return func() string { return s } }

func in(s string) Inner { return Inner{F: mk(s), X: mk(s), f: mk(s)} }

func CreateConfig() *Config {
	ip := in("p")
	c := &Config{Name: "n", P: &ip, SM: map[string]Inner{}, SP: map[string]*Inner{},
		NN: map[string]map[float64]int{"a": {math.NaN(): 1, 2: 2}},
		I:  map[float64]int{math.NaN(): 1, 2: 2},
		AR: [3]func() string{mk("0"), mk("1"), mk("2")},
		NS: map[[2]float64]int{{math.NaN(), 1}: 1, {2, 2}: 2},
		NI: map[any]func() string{math.NaN(): mk("nan"), 1: mk("1")},
	}
	for i := range 10 {
		k := fmt.Sprint(i)
		c.SL = append(c.SL, in(k))
		c.SM[k] = in(k)
		v := in(k)
		c.SP[k] = &v
		c.IS = append(c.IS, in(k), mk(k))
	}
	return c
}

func New(_ context.Context, _ http.Handler, cfg *Config, name string) (http.Handler, error) {
	full := *cfg
	*cfg = Config{Name: "n"}
	switch name {
	case "P":
		cfg.P = full.P
	case "SL":
		cfg.SL = full.SL
	case "SM":
		cfg.SM = full.SM
	case "SP":
		cfg.SP = full.SP
	case "NN":
		cfg.NN = full.NN
	case "I":
		cfg.I = full.I
	case "AR":
		cfg.AR = full.AR
	case "NS":
		cfg.NS = full.NS
	case "NI":
		cfg.NI = full.NI
	case "IS":
		cfg.IS = full.IS
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := json.Marshal(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var c2 Config
		if err := json.Unmarshal([]byte(`{"Name":"x"}`), &c2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		bad := 0
		ok := func(x Inner, k string) bool { return x.F() == k && x.X.(func() string)() == k && x.f() == k }
		for i := range 10 {
			k := fmt.Sprint(i)
			if cfg.SL != nil && !ok(cfg.SL[i], k) || cfg.SM != nil && !ok(cfg.SM[k], k) || cfg.SP != nil && !ok(*cfg.SP[k], k) {
				bad++
			}
			if cfg.IS != nil {
				if x, isIn := cfg.IS[2*i].(Inner); !isIn || !ok(x, k) || cfg.IS[2*i+1].(func() string)() != k {
					bad++
				}
			}
		}
		if cfg.P != nil && !ok(*cfg.P, "p") || cfg.NN != nil && len(cfg.NN["a"]) != 2 || cfg.I != nil && len(cfg.I.(map[float64]int)) != 2 ||
			cfg.AR[0] != nil && cfg.AR[1]() != "1" || cfg.NS != nil && len(cfg.NS) != 2 || cfg.NI != nil && (len(cfg.NI) != 2 || cfg.NI[1]() != "1") {
			bad++
		}
		fmt.Fprintf(w, "bad %d", bad)
	}), nil
}
