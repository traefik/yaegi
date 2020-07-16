package main

import (
	"github.com/containous/yaegi/interp"
)

func main() {
	i := interp.New(interp.Opt{})
	i.Use(interp.ExportValue, interp.ExportType)
	i.EvalInc(`import "github.com/containous/yaegi/interp"`)
	i.EvalInc(`i := interp.New(interp.Opt{})`)
	i.EvalInc(`i.Eval("println(42)")`)
}

// Output:
// 42
