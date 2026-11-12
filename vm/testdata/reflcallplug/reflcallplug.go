// Package reflcallplug is a plugin whose subpackage calls the plugin's
// functions with reflect.Value.Call. The plugin package
// itself does not import reflect: the results and arguments must keep
// their named types anyway.
package reflcallplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/traefik/yaegi/v2/vm/testdata/reflcallplug/call"
)

// Builder is a named func type, like TreeBuilder of vulcand/predicate.
type Builder func() int

// S, L and M are named struct, slice and map types.
type (
	S struct{ A int }
	L []int
	M map[string]int
)

// Config is the plugin configuration.
type Config struct{}

// CreateConfig creates the default configuration.
func CreateConfig() *Config { return &Config{} }

// Report lists, one per line, the dynamic type of each result and whether
// the type assertion to the named type works.
func Report() string {
	var b strings.Builder
	out := call.Call(func() Builder { return func() int { return 42 } })
	f, ok := out.(Builder)
	fmt.Fprintf(&b, "%T %v", out, ok)
	if ok {
		fmt.Fprintf(&b, " %d", f())
	}
	b.WriteString("\n")
	s, ok := call.Call(func() S { return S{1} }).(S)
	fmt.Fprintf(&b, "S %v %v\n", ok, s.A)
	l, ok := call.Call(func() L { return L{1, 2} }).(L)
	fmt.Fprintf(&b, "L %v %v\n", ok, len(l))
	mv, ok := call.Call(func() M { return M{"a": 1} }).(M)
	fmt.Fprintf(&b, "M %v %v\n", ok, mv["a"])
	_, ok = call.Call(func() *S { return &S{} }).(*S)
	fmt.Fprintf(&b, "*S %v\n", ok)
	// Arguments: a named value goes in, the parameter is named or any.
	_, ok = call.Call(func(f Builder) Builder { return f }, Builder(func() int { return 1 })).(Builder)
	fmt.Fprintf(&b, "arg Builder %v\n", ok)
	_, ok = call.Call(func(v any) any { return v }, L{1}).(L)
	fmt.Fprintf(&b, "arg any L %v\n", ok)
	_, ok = call.Call(func(v S) any { return v }, S{2}).(S)
	fmt.Fprintf(&b, "arg S %v\n", ok)
	fmt.Fprintf(&b, "Out %s\n", call.Out(func() Builder { return nil }))
	return b.String()
}

// New creates the middleware. It writes Report in the response body.
func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, Report())
	}), nil
}
