// Package vendorunsafe imports a package that imports syscall.
package vendorunsafe

import (
	"context"
	"net/http"

	"github.com/traefik/yaegi/v2/vm/testdata/restricted/vendorunsafe/inner"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	_ = inner.Pid()
	return next, nil
}
