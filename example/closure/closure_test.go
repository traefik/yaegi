package clos1

import (
	"path/filepath"
	"testing"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

func TestFunctionCall(t *testing.T) {
	goPath, err := filepath.Abs("./")
	if err != nil {
		t.Fatal(err)
	}
	i := interp.New(interp.Opt{GoPath: goPath})
	i.Use(stdlib.Value, stdlib.Type)
	_, err = i.Eval(`import "foo/bar"`)
	if err != nil {
		t.Fatal(err)
	}
	fnv, err := i.Eval(`bar.NewSample()`)
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := fnv.Interface().(func(string, string) func(string))
	if !ok {
		t.Fatal("conversion failed")
	}
	fn2 := fn("hello", "world")
	fn2("truc")
}
