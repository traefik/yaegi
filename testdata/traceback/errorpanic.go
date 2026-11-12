// An unrecovered panic with an error value in nested calls: yaegi prints
// "panic: " and the message once, as gc does (TestPanicLine).
package main

import "errors"

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	check(nil)
	check(errors.New("stop!"))
}
