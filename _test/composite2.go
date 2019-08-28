package main

import "fmt"

var a = &[]*T{{"hello"}}

type T struct{ name string }

func main() {
	fmt.Println((*a)[0])
}

// Output:
// &{hello}
