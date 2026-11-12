package main

import (
	"fmt"
	"reflect"
)

func main() {
	a := T(12)
	fmt.Println(reflect.TypeOf(a))
}

type T int

// Output:
// int
