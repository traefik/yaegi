// Package byteloop is a middleware that reads each byte of the response,
// as a plugin that rewrites the body does (TestBudgetRateByteLoop).
package byteloop

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type wrapper struct {
	http.ResponseWriter
	n int64
}

func (w *wrapper) Write(b []byte) (int, error) {
	for _, c := range b {
		if c == 'x' {
			w.n++
		}
	}
	return w.ResponseWriter.Write(b)
}

type mw struct{ next http.Handler }

func (m mw) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.next.ServeHTTP(&wrapper{ResponseWriter: w}, r)
}

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return mw{next}, nil
}
