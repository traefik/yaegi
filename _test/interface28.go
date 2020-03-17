package main

import "fmt"

var errs = []error{nil}

func main() {
	fmt.Println(errs)
}

// Output:
// [<nil>]
