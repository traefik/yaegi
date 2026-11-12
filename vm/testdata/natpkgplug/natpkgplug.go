// Package natpkgplug uses the native package natpkg (natpkg_test.go): it
// gives natpkg.Use a value of the plugin as natpkg.Sink, and prints a
// natpkg.State. Its handler also names itself, as a Traefik middleware
// names its traces.
package natpkgplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/traefik/yaegi/v2/vm/testdata/natpkg"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type sink struct{ lines []string }

func (s *sink) Info(msg string) { s.lines = append(s.lines, "info "+msg) }

func (s *sink) Error(err error, msg string) {
	s.lines = append(s.lines, "error "+msg+": "+err.Error())
}

type handler struct{ name string }

func (h handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s := &sink{}
	natpkg.Use(s)
	fmt.Fprintln(w, strings.Join(s.lines, "; "))
	fmt.Fprintln(w, natpkg.NewState("a", "1", "b", "2"))
}

func (h handler) Name() string { return h.name }

func New(_ context.Context, _ http.Handler, _ *Config, name string) (http.Handler, error) {
	return handler{name: name}, nil
}
