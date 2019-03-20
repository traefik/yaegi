package main

import (
	"log"

	"github.com/containous/yaegi/interp"
)

func main() {
	i := interp.New(interp.Opt{})
	i.Use(interp.ExportValue)
	if _, err := i.Eval(`import "github.com/containous/yaegi/interp"`); err != nil {
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
