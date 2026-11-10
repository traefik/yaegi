package main

import "fmt"

type T struct {
	I any
}

func main() {
	t := T{"test"}
	fmt.Println(t)
}

// Output:
// {test}
