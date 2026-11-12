package main

import (
	"fmt"
	"reflect"
)

type A struct {
	Test string `tag:"test"`
}

func main() {
	a := A{}
	t := reflect.TypeOf(a)
	f, ok := t.FieldByName("Test")
	if !ok {
		return
	}

	fmt.Println(f.Tag.Get("tag"))
}

// Output:
// test
