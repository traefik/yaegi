// fmt recovers the panic of a String method: the traceback of the later
// unrecovered panic shows its own stack, not the stack of the first panic
// (TestTraceback).
package main

import "fmt"

type T struct{}

func (T) String() string { panic("boom in String") }

func main() {
	fmt.Println(T{})
	panic("final")
}
