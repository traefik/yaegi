// Package hostwrite is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it changes objects of standard library variables: the default HTTP client and transport, net.IPv4bcast.
package hostwrite

import (
	"context"
	"net"
	"net/http"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := http.DefaultClient
	c.Timeout = time.Nanosecond
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		t.MaxIdleConns = 1
	}
	ip := net.IPv4bcast
	ip[0] = 1
}
