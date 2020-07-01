package main

import (
	"fmt"
	"strconv"
)

type Int int

func (I Int) String() string {
	return "foo-" + strconv.Itoa(int(I))
}

func main() {
	var i Int = 3
	fmt.Println(i)
}

// Output:
// foo-3
