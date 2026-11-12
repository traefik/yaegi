package main

import "fmt"

type T struct {
	m uint16
}

var t = T{1<<2 | 1<<3}

func main() {
	fmt.Println(t)
}

// Output:
// {12}
