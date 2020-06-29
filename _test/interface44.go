package main

import (
	"fmt"
	"strconv"
)

type Foo int

func (f Foo) String() string {
	return "foo-" + strconv.Itoa(int(f))
}

func print1(arg interface{}) {
	fmt.Println(arg)
}

func main() {
	var arg Foo = 3
	var f = print1
	f(arg)
}

// Output:
// foo-3
