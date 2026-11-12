// Package loadbomb is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): a chain of types that doubles in size at each step, which go/types would expand for ever.
package loadbomb

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

type (
	T0  struct{ a, b int }
	T1  struct{ a, b T0 }
	T2  struct{ a, b T1 }
	T3  struct{ a, b T2 }
	T4  struct{ a, b T3 }
	T5  struct{ a, b T4 }
	T6  struct{ a, b T5 }
	T7  struct{ a, b T6 }
	T8  struct{ a, b T7 }
	T9  struct{ a, b T8 }
	T10 struct{ a, b T9 }
	T11 struct{ a, b T10 }
	T12 struct{ a, b T11 }
	T13 struct{ a, b T12 }
	T14 struct{ a, b T13 }
	T15 struct{ a, b T14 }
	T16 struct{ a, b T15 }
	T17 struct{ a, b T16 }
	T18 struct{ a, b T17 }
	T19 struct{ a, b T18 }
	T20 struct{ a, b T19 }
	T21 struct{ a, b T20 }
	T22 struct{ a, b T21 }
	T23 struct{ a, b T22 }
	T24 struct{ a, b T23 }
	T25 struct{ a, b T24 }
	T26 struct{ a, b T25 }
	T27 struct{ a, b T26 }
	T28 struct{ a, b T27 }
	T29 struct{ a, b T28 }
	T30 struct{ a, b T29 }
	T31 struct{ a, b T30 }
	T32 struct{ a, b T31 }
	T33 struct{ a, b T32 }
	T34 struct{ a, b T33 }
	T35 struct{ a, b T34 }
	T36 struct{ a, b T35 }
	T37 struct{ a, b T36 }
	T38 struct{ a, b T37 }
	T39 struct{ a, b T38 }
	T40 struct{ a, b T39 }
)

var big T40

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { _ = big }
