// Package reloadplug is a Traefik style plugin for TestReloadSandboxLimits:
// each New starts a goroutine that ends with the context of New (as the
// refresh loops of many catalog plugins), and some paths try to pass the
// limits of the host (budget, sandbox, goroutines).
package reloadplug

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Config struct {
	Name string `json:"name,omitempty"`
}

func CreateConfig() *Config { return &Config{Name: "reload"} }

type plug struct {
	next http.Handler
	name string
}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return &plug{next: next, name: cfg.Name}, nil
}

func (p *plug) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/spin":
		n := 0
		for {
			n++
		}
	case "/read":
		_, err := os.ReadFile("/etc/hostname")
		fmt.Fprint(rw, err)
	case "/write":
		fmt.Fprint(rw, os.WriteFile("/tmp/../etc/reloadplug", []byte("x"), 0o644))
	case "/env":
		fmt.Fprintf(rw, "HOME=%q", os.Getenv("HOME"))
	case "/goroutines":
		block := make(chan struct{})
		defer close(block)
		for i := 0; i < 100; i++ {
			go func() { <-block }()
		}
		fmt.Fprint(rw, "100 goroutines started")
	default:
		fmt.Fprintf(rw, "%s ok", p.name)
	}
}
