// Package jsoncycplug gives json.Marshal values that contain themselves.
// A compiled program gets an UnsupportedValueError ("encountered a cycle");
// before the fix, a plugin killed the host with a fatal Go stack overflow
// (AOT mode: pointers, default mode: maps and slices). The path of the
// request is /<shape>; the body is the error of json.Marshal.
package jsoncycplug

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type node struct{ Next *node }

type left struct{ R *right }

type right struct{ L *left }

func value(shape string) any {
	switch shape {
	case "self":
		n := &node{}
		n.Next = n
		return n
	case "two":
		l := &left{}
		l.R = &right{L: l}
		return l
	case "map":
		m := map[string]any{}
		m["m"] = m
		return m
	case "slice":
		s := []any{nil}
		s[0] = s
		return s
	}
	return nil
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := json.Marshal(value(strings.TrimPrefix(r.URL.Path, "/")))
		fmt.Fprint(w, err)
	}), nil
}
