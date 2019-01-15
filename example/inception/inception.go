package main

import (
	"log"

	"github.com/containous/dyngo/interp"
)

func main() {
	i := interp.New(interp.Opt{})
	i.Use(interp.ExportValue, interp.ExportType)
	_, err := i.Eval(`import "github.com/containous/dyngo/interp"`)
	if err != nil {
		log.Fatal(err)
	}
	_, err = i.Eval(`i := interp.New(interp.Opt{})`)
	if err != nil {
		log.Fatal(err)
	}
	_, err = i.Eval(`i.Eval("println(42)")`)
	if err != nil {
		log.Fatal(err)
	}
}

// Output:
// 42
