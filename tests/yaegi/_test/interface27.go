package main

import "fmt"

var errs = map[int]error{0: nil}

func main() {
	fmt.Println(errs)
}

// Output:
// map[0:<nil>]
