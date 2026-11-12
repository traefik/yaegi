package vm

import (
	"context"
	"testing"
)

// TestContextValueAliasOpaque checks that a map, slice, struct with a
// pointer or pointer that a Context of the program gives from Value, through
// context.WithCancel, aliases the program's value with -aot and -opaque, as
// in Go. The result of the program method in an any used to be a native
// copy there (plainType accepted any map, slice or struct), so writes on one
// side were not seen on the other.
func TestContextValueAliasOpaque(t *testing.T) {
	const want = `map: map[a:1 b:2 c:3] map[a:1 b:2 c:3]
slice: [10 20] [10 20]
struct with pointer: 5 5 1
pointer: {7 y} {7 y}
plain struct: {1 x} {9 x}
array: [1 2] [9 2]
`
	for _, c := range []struct {
		name string
		opts Options
	}{
		{"aot", Options{AOT: true}},
		{"opaque", Options{OpaqueAny: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := c.opts
			opts.Dir = ".."
			p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/ctxaliasplug")
			if err != nil {
				t.Fatal(err)
			}
			res, err := p.Call(context.Background(), "Run")
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := res[0].Interface(); got != want {
				t.Errorf("got:\n%v\nwant:\n%s", got, want)
			}
		})
	}
}
