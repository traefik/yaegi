package main

import (
	"fmt"
	"reflect"
)

func main() {
	a := int32(12)
	fmt.Println(reflect.TypeOf(a))
}

// Output:
// int32
