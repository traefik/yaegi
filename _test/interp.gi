package main

import (
	"github.com/containous/yaegi/interp"
)

func main() {
	i := interp.New(interp.Opt{})
	i.EvalInc(`println("Hello")`)
}

// Output:
// Hello
