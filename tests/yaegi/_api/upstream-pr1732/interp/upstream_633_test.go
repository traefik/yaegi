package interp_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

const upstream633Src = `
package main

import (
	"fmt"
	"harness633/harness633"
)

type A struct{ B string }

func (a A) String() string { return "A<" + a.B + ">" }

var S fmt.Stringer = A{B: "v"}
var P fmt.Stringer = &A{B: "p"}

func main() {
	// Binary function taking an empty interface parameter: the concrete value
	// must be received, not the interpreter interface wrapper (#633).
	fmt.Println(harness633.TypeOf(S))
	fmt.Println(harness633.TypeOf(P))
	fmt.Println(harness633.Marshal(S))
	fmt.Println(harness633.Marshal(P))
	// Containers of interface-held interpreted values are passed untouched:
	// in-place mutation contracts (sort.Slice) take priority over marshaling,
	// which fails loudly instead of corrupting data.
	fmt.Println(harness633.MarshalErr([]fmt.Stringer{S}))

	// Binary function taking a non-empty interface parameter: the interpreted
	// method must still be dispatched.
	fmt.Println(harness633.String(S), harness633.String(P))

	// The concrete value behind the interface wrapper stays intact.
	fmt.Println(S.(A).B, P.(*A).B)
}
`

func TestUpstream633(t *testing.T) {
	var stdout bytes.Buffer
	i := interp.New(interp.Options{Stdout: &stdout})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	i.Use(interp.Exports{"harness633/harness633": {
		"TypeOf": reflect.ValueOf(func(v interface{}) string { return fmt.Sprintf("%T", v) }),
		"Marshal": reflect.ValueOf(func(v interface{}) string {
			b, err := json.Marshal(v)
			if err != nil {
				return err.Error()
			}
			return string(b)
		}),
		"String": reflect.ValueOf(func(v fmt.Stringer) string { return v.String() }),
		"MarshalErr": reflect.ValueOf(func(v interface{}) string {
			_, err := json.Marshal(v)
			if err != nil {
				return "marshal error: " + err.Error()
			}
			return "no error"
		}),
	}})

	if _, err := i.Eval(upstream633Src); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"struct { B string }",  // TypeOf(S)
		"*struct { B string }", // TypeOf(P)
		`{"B":"v"}`,            // Marshal(S)
		`{"B":"p"}`,            // Marshal(P)
		"marshal error: json: unsupported type: func() string", // containers pass through untouched
		"A<v> A<p>", // String(S), String(P)
		"v p",       // concrete value behind wrapper intact
	}, "\n")
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
