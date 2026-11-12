// Package gopanic is a hostile plugin of the corpus of the catalog harness
// (compare/catalog/vmhost/hostile.go): it panics in a goroutine and in a time.AfterFunc callback, which ends a Go program.
package gopanic

import (
	"context"
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
	go func() { panic("hostile goroutine") }()
	time.AfterFunc(0, func() { panic("hostile timer") })
}
