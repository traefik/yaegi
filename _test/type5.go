package main

import (
	"fmt"
	"reflect"
)

type T int

func main() {
	a := T(12)
	fmt.Println(reflect.TypeOf(a))
}

// Output:
// int
