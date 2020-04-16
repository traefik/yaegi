package main

import "fmt"

type T struct {
	I interface{}
}

func main() {
	t := T{"test"}
	fmt.Println(t)
}

// Output:
// {test}
