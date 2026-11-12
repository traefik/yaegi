package main

import "fmt"

func main() {
	a := [...]string{9: "hello"}
	fmt.Printf("%v %T\n", a, a)
}

// Output:
// [         hello] [10]string
