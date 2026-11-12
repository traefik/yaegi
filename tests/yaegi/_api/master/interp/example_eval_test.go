package interp_test

import (
	"fmt"
	"log"

	"github.com/traefik/yaegi/interp"
)

// Generic example.
func Example_eval() {
	// Create a new interpreter context
	i := interp.New(interp.Options{})

	// Run some code: define a new function
	_, err := i.Eval("func f(i int) int { return 2 * i }")
	if err != nil {
		log.Fatal(err)
	}

	// Access the interpreted f function with Eval
	v, err := i.Eval("f")
	if err != nil {
		log.Fatal(err)
	}

	// Returned v is a reflect.Value, so we can use its interface
	f, ok := v.Interface().(func(int) int)
	if !ok {
		log.Fatal("type assertion failed")
	}

	// Use interpreted f as it was pre-compiled
	fmt.Println(f(2))

	// Output:
	// 4
}
