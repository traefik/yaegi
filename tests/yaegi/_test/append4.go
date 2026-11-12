package main

import "fmt"

func main() {
	a := []*int{}
	a = append(a, nil)
	fmt.Println(a)
}

// Output:
// [<nil>]
