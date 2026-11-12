// Package nativememplug asks native code of the standard library for large
// results whose size is known from the arguments (strings.Repeat,
// bytes.Repeat, strings.Builder.Grow, bytes.Buffer.Grow, reflect.MakeSlice)
// or grows without end (io.ReadAll of an endless reader), by a static call,
// a function value, an interface method or a deferred call. Each route
// recovers the panic and writes it ("ok" without a panic). The size is the
// query string ?n=, in bytes.
package nativememplug

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

// endless is a reader that never ends.
type endless struct{}

func (endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

var sink, held any

// Function values of the native functions: a call through them is not a
// static call.
var (
	repeat  = strings.Repeat
	readAll = io.ReadAll
	grow    = (*strings.Builder).Grow
)

// try runs f and returns its panic, as text ("ok" without a panic).
func try(f func()) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	f()
	return "ok"
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	var f func()
	switch r.URL.Path {
	case "/repeat":
		f = func() { sink = strings.Repeat("x", n) }
	case "/repeat8":
		f = func() { sink = strings.Repeat("12345678", n/8) }
	case "/bytesrepeat":
		f = func() { sink = bytes.Repeat([]byte("x"), n) }
	case "/buildergrow":
		f = func() {
			var b strings.Builder
			b.Grow(n)
			sink = &b
		}
	case "/buffergrow":
		f = func() {
			var b bytes.Buffer
			b.Grow(n)
			sink = &b
		}
	case "/makeslice":
		f = func() { sink = reflect.MakeSlice(reflect.TypeOf([]byte(nil)), n, n).Interface() }
	case "/makeslicecap":
		f = func() { sink = reflect.MakeSlice(reflect.TypeOf([]int64(nil)), 0, n/8).Interface() }
	case "/readall":
		f = func() {
			b, err := io.ReadAll(io.LimitReader(endless{}, int64(n)))
			sink = b
			if err != nil {
				panic(err)
			}
		}
	case "/readallendless":
		f = func() { sink, _ = io.ReadAll(endless{}) }
	case "/ioutilreadall":
		// ioutil.ReadAll calls io.ReadAll natively.
		f = func() { sink, _ = ioutil.ReadAll(endless{}) }
	case "/repeatvalue":
		f = func() { sink = repeat("x", n) }
	case "/readallvalue":
		f = func() { sink, _ = readAll(endless{}) }
	case "/growvalue":
		f = func() {
			var b strings.Builder
			grow(&b, n)
			sink = &b
		}
	case "/buildergrowiface":
		f = func() {
			var g interface{ Grow(int) } = &strings.Builder{}
			g.Grow(n)
			sink = g
		}
	case "/buffergrowiface":
		f = func() {
			var g interface{ Grow(int) } = &bytes.Buffer{}
			g.Grow(n)
			sink = g
		}
	case "/repeatdefer":
		f = func() { defer strings.Repeat("x", n) }
	case "/readalldefer":
		f = func() { defer io.ReadAll(endless{}) }
	case "/buildergrowfree", "/buffergrowfree":
		// Grow(n) twice, with 40 MiB held in between: the second Grow
		// fits in the free capacity and allocates nothing.
		f = func() {
			var g interface{ Grow(int) } = &strings.Builder{}
			if r.URL.Path == "/buffergrowfree" {
				g = &bytes.Buffer{}
			}
			switch b := g.(type) {
			case *strings.Builder:
				b.Grow(n)
				held = make([]byte, 40<<20)
				b.Grow(n)
			case *bytes.Buffer:
				b.Grow(n)
				held = make([]byte, 40<<20)
				b.Grow(n)
			}
			sink = g
		}
	default:
		http.NotFound(w, r)
		return
	}
	res := try(f)
	sink, held = nil, nil
	fmt.Fprint(w, res)
}
