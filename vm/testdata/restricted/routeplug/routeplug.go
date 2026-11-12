// Package routeplug formats its own types with fmt and uses reflect on host
// objects: fmt goes to its AOT copy when the host links it, reflect stays
// native (Set of an unexported field of a host object panics).
package routeplug

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type T struct{ N int }

func (t T) String() string { return fmt.Sprintf("T!%d", t.N) }

type P struct{ A, B int }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%T %v %#v|", T{1}, T{2}, P{3, 4})
		v := reflect.ValueOf(r).Elem()
		v.FieldByName("Method").SetString("PUT")
		fmt.Fprintf(w, "%s %s|", r.Method, v.Type())
		func() {
			defer func() { fmt.Fprint(w, recover()) }()
			ctx := v.FieldByName("ctx")
			ctx.Set(reflect.Zero(ctx.Type()))
		}()
	}), nil
}
