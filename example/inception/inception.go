package main

import (
	"log"

	"github.com/containous/dyngo/interp"
)

func main() {
	i := interp.New(interp.Opt{})
	i.Use(interp.ExportValue, interp.ExportType)
	if _, err := i.Eval(`import "github.com/containous/dyngo/interp"`); err != nil {
		log.Fatal(err)
	}
	if _, err := i.Eval(`i := interp.New(interp.Opt{})`); err != nil {
		log.Fatal(err)
	}
	if _, err := i.Eval(`i.Eval("println(42)")`); err != nil {
		log.Fatal(err)
	}
}

// Output:
// 42
