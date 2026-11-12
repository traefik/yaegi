// Package sharedifaceptr gives its config to json.Marshal from many
// ServeHTTP calls at the same time. The config holds pointers in an
// interface, in a []*W, in a map[string]*W and in a []any; each pointed W
// holds an interpreted named value (a struct T or an int N) in an
// interface. json.Marshal only reads the config: yaegi v2 must not write into
// it after the call, and every value keeps its dynamic type (T, not
// struct { X int }; N, not int).
package sharedifaceptr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type T struct{ X int }

type N int

type W struct{ V any }

type Config struct {
	P any
	L []*W
	M map[string]*W
	A []any
}

func CreateConfig() *Config {
	return &Config{
		P: &W{V: T{1}},
		L: []*W{{V: T{2}}, {V: N(3)}},
		M: map[string]*W{"a": {V: T{4}}, "b": {V: N(5)}},
		A: []any{&W{V: T{6}}, &W{V: N(7)}},
	}
}

// local holds the shared pointers in a struct of the request.
type local struct {
	P any
	L []*W
	A []any
}

func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := json.Marshal(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d := local{P: cfg.P, L: cfg.L, A: cfg.A}
		if _, err := json.Marshal(&d); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		bad := 0
		isT := func(v any, x int) {
			if t, ok := v.(T); !ok || t.X != x {
				bad++
			}
		}
		isN := func(v any, x int) {
			if n, ok := v.(N); !ok || int(n) != x {
				bad++
			}
		}
		if p, ok := cfg.P.(*W); ok {
			isT(p.V, 1)
		} else {
			bad++
		}
		isT(cfg.L[0].V, 2)
		isN(cfg.L[1].V, 3)
		isT(cfg.M["a"].V, 4)
		isN(cfg.M["b"].V, 5)
		for i, x := range []int{6, 7} {
			p, ok := cfg.A[i].(*W)
			switch {
			case !ok:
				bad++
			case i == 0:
				isT(p.V, x)
			default:
				isN(p.V, x)
			}
		}
		fmt.Fprintf(w, "bad %d", bad)
	}), nil
}
