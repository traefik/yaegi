package main

import (
	"fmt"
	"testing"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func TestGetFunc(t *testing.T) {
	i := interp.New()
	i.GoPath("./_gopath/")
	i.Use(stdlib.Symbols)

	if _, err := i.Eval(`import "github.com/foo/bar"`); err != nil {
		t.Fatal(err)
	}

	val, err := i.Eval(`bar.NewFoo`)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(val.Call(nil))
}
