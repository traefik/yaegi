package clos1

import (
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func TestFunctionCall(t *testing.T) {
	i := interp.New(interp.Options{GoPath: "./_pkg"})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	_, err := i.Eval(`import "foo/bar"`)
	if err != nil {
		t.Fatal(err)
	}

	fnv, err := i.Eval(`bar.NewSample()`)
	if err != nil {
		t.Fatal(err)
	}

	fn, ok := fnv.Interface().(func(string, string) func(string) string)
	if !ok {
		t.Fatal("conversion failed")
	}

	fn2 := fn("hello", "world")
	val := fn2("truc")

	expected := "herev1helloworldtruc"
	if val != expected {
		t.Errorf("Got: %q, want: %q", val, expected)
	}
}
