package clos1

import (
	"testing"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func TestFunctionCall(t *testing.T) {
	i := interp.New(interp.Options{GoPath: "./_pkg"})
	i.Use(stdlib.Symbols)

	_, err := i.EvalInc(`import "foo/bar"`)
	if err != nil {
		t.Fatal(err)
	}

	fnv, err := i.EvalInc(`bar.NewSample()`)
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
