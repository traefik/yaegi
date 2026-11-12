package main

import "fmt"

type T struct {
	b []byte
}

func main() {
	t := T{nil}
	fmt.Println(t)
}

// Output:
// {[]}
