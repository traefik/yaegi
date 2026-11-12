// Package flagduptmplplug is flagdupplug with text/template, which makes
// the standard library run as AOT copies in default mode: flag must stay
// native all the same, with a flag.CommandLine for each instance.
package flagduptmplplug

import (
	"context"
	"flag"
	"net/http"
	"text/template"
)

var dup = flag.Bool("dup", false, "a flag of each instance")

var tmpl = template.Must(template.New("t").Parse("ok {{.}}\n"))

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = tmpl.Execute(w, *dup)
	}), nil
}
