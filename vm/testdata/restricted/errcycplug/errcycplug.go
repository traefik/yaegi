// Package errcycplug walks cyclic error chains with errors.Is, errors.As
// and errors.AsType. A compiled program loops or dies with a stack overflow
// on these chains; a plugin must stop with ErrBudget or ErrStackOverflow.
// The path of the request is /<func>/<chain>: func is is, as or astype,
// chain is self, multi, native or join.
package errcycplug

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// self is its own Unwrap() error: a chain without end, and no call and no
// loop in interpreted code.
type self struct{}

func (s *self) Error() string { return "self" }
func (s *self) Unwrap() error { return s }

// multi is its own Unwrap() []error.
type multi []error

func (m multi) Error() string   { return "multi" }
func (m multi) Unwrap() []error { return m }

// native gets the Unwrap method of a native error, that returns the
// native error itself.
type native struct{ *fs.PathError }

type target struct{}

func (target) Error() string { return "target" }

func chain(name string) error {
	switch name {
	case "self":
		return &self{}
	case "multi":
		m := make(multi, 1)
		m[0] = m
		return m
	case "native":
		pe := &fs.PathError{Op: "x"}
		n := native{pe}
		pe.Err = n
		return n
	case "join":
		m := make(multi, 1)
		j := errors.Join(m)
		m[0] = j
		return j
	}
	return nil
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn, name, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		err := chain(name)
		switch fn {
		case "is":
			fmt.Fprint(w, errors.Is(err, io.EOF))
		case "as":
			var t target
			fmt.Fprint(w, errors.As(err, &t))
		case "astype":
			_, ok := errors.AsType[target](err)
			fmt.Fprint(w, ok)
		case "ok":
			// A chain that ends: the plugin still works.
			fmt.Fprint(w, errors.Is(fmt.Errorf("a: %w", errors.Join(target{}, io.EOF)), io.EOF))
		}
	}), nil
}
