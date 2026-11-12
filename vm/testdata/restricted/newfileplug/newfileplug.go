// Package newfileplug calls os.NewFile: with the number of the host stdout,
// with the number of a file that it opened itself, through a function
// value, and with an invalid number (Go returns nil). Each request answers
// the result, or the panic.
package newfileplug

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// try runs f and returns its result, or its panic.
func try(f func() string) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	return f()
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var res string
		switch r.URL.Path {
		case "/stdout":
			res = try(func() string { return fmt.Sprintf("%T", os.NewFile(1, "stdout")) })
		case "/own":
			pr, pw, err := os.Pipe()
			if err != nil {
				res = err.Error()
				break
			}
			defer pr.Close()
			defer pw.Close()
			res = try(func() string { return fmt.Sprintf("%T", os.NewFile(pw.Fd(), "pipe")) })
		case "/value":
			newFile := os.NewFile
			res = try(func() string { return fmt.Sprintf("%T", newFile(1, "stdout")) })
		case "/invalid":
			res = try(func() string { return fmt.Sprint(os.NewFile(^uintptr(0), "invalid") == nil) })
		}
		fmt.Fprint(w, res)
	}), nil
}
