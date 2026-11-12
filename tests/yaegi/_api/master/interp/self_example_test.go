package interp_test

import (
	"log"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func ExampleInterpreter_self() {
	i := interp.New(interp.Options{})

	if err := i.Use(stdlib.Symbols); err != nil {
		log.Fatal(err)
	}
	if err := i.Use(interp.Symbols); err != nil {
		log.Fatal(err)
	}

	_, err := i.Eval(`import (
	"fmt"
	"log"

	// Import interp to gain access to Self.
	"github.com/traefik/yaegi/interp"
)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = i.Eval(`
		// Evaluate code directly.
		fmt.Println("Hello Yaegi from Go")

		// Evaluate code indirectly via the Self access point.
		_, err := interp.Self.Eval("fmt.Println(\"Hello Yaegi from Yaegi\")")
		if err != nil {
			log.Fatal(err)
		}
`)
	if err != nil {
		log.Fatal(err)
	}

	// Output:
	//
	// Hello Yaegi from Go
	// Hello Yaegi from Yaegi
}
