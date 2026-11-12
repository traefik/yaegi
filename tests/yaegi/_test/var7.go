package main

import (
	"fmt"
)

type T struct {
	Name string
}

var m = make(map[string]*T)

func main() {
	fmt.Println(m)
}

// Output:
// map[]
