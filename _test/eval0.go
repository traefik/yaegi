package main

import (
	"log"

	"github.com/containous/yaegi/interp"
)

func main() {
	log.SetFlags(log.Lshortfile)
	i := interp.New(interp.Options{})
	if _, err := i.EvalInc(`func f() (int, int) { return 1, 2 }`); err != nil {
		log.Fatal(err)
	}
	if _, err := i.EvalInc(`a, b := f()`); err != nil {
		log.Fatal(err)
	}
	if _, err := i.EvalInc(`println(a, b)`); err != nil {
		log.Fatal(err)
	}
}

// Output:
// 1 2
