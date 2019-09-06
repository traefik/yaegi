package main

import (
	goflag "flag"
	"fmt"
)

func Foo(goflag *goflag.Flag) {
	fmt.Println(goflag)
}

func main() {
	g := &goflag.Flag{}
	Foo(g)
}

// Output:
// &{  <nil> }
