package main

import (
	"github.com/containous/yaegi/interp"
)

func main() {
	i := interp.New(interp.Opt{})
	i.Use(interp.ExportValue, interp.ExportType)
	i.Eval(`import "github.com/containous/yaegi/interp"`)
	i.Eval(`i := interp.New(interp.Opt{})`)
	i.Eval(`i.Eval("println(42)")`)
}

// Output:
// 42
