package main

import "fmt"

type X struct{}

func (x X) Error() string { return "hi" }

type Y struct{ X }

func main() {
	y := Y{}
	var i interface{} = y
	if e, ok := i.(error); ok {
		fmt.Println("error msg:", e.Error())
	} else {
		fmt.Println("assert to error: ok=false")
	}
	var e2 error = y
	fmt.Println("assign to error:", e2.Error())
}

// Output:
// error msg: hi
// assign to error: hi
