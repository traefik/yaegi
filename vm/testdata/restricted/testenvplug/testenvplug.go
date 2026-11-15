// Package testenvplug tries to change the working directory and the
// environment of the host with the methods Chdir and Setenv of package
// testing, without naming them in its code: through testing.TB, its
// reflect, an interface of its own, a method expression and a generic
// function. Each path must be refused when it runs.
package testenvplug

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type setenver interface{ Setenv(key, value string) }

// setenv calls Setenv through the constraint of a type parameter.
func setenv[S setenver](s S, key string) { s.Setenv(key, "1") }

// embeds promotes the methods of *testing.common through *testing.B.
type embeds struct{ *testing.B }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dir, key := r.URL.Query().Get("dir"), r.URL.Query().Get("key")
		var t testing.TB = new(testing.T)
		var b testing.TB = new(testing.B)
		var f any = new(testing.F)
		switch r.URL.Path {
		case "/tbchdir":
			t.Chdir(dir)
		case "/tbsetenv":
			t.Setenv(key, "1")
		case "/bchdir":
			b.Chdir(dir)
		case "/bsetenv":
			b.Setenv(key, "1")
		case "/reflectchdir":
			reflect.ValueOf(t).MethodByName("Chdir").Call([]reflect.Value{reflect.ValueOf(dir)})
		case "/reflectsetenv":
			reflect.ValueOf(b).MethodByName("Setenv").Call([]reflect.Value{reflect.ValueOf(key), reflect.ValueOf("1")})
		case "/interface":
			f.(setenver).Setenv(key, "1")
		case "/embed":
			var e any = embeds{new(testing.B)}
			e.(setenver).Setenv(key, "1")
		case "/methodexpr":
			testing.TB.Setenv(b, key, "1")
		case "/generic":
			setenv(new(testing.F), key)
		}
	}), nil
}
