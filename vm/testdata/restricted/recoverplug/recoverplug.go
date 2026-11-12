// Package recoverplug tries to stop the budget and the stack overflow
// errors with recover. The request path selects the case.
package recoverplug

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deferrecover": // defer recover() in a later deferred call
			deferRecover()
		case "/recover": // recover() in a later deferred call
			plainRecover()
		case "/stack": // recover() after a deferred endless recursion
			stackRecover()
		case "/stackdefer": // defer recover() after a deferred endless recursion
			stackDeferRecover()
		}
		w.Write([]byte("returned"))
	}), nil
}

func spin() {
	n := 0
	for {
		if n++; n < 0 {
			break
		}
	}
}

func rec(n int) int { return rec(n+1) + 1 }

func deferRecover() {
	defer func() { defer recover() }()
	defer spin()
	panic("x")
}

func plainRecover() {
	defer func() { recover() }()
	defer spin()
	panic("x")
}

func stackRecover() {
	defer func() { recover() }()
	defer func() { rec(0) }()
	panic("x")
}

func stackDeferRecover() {
	defer func() { defer recover() }()
	defer func() { rec(0) }()
	panic("x")
}
