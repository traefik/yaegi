package main

import (
	"fmt"
)

func SomeFunc(defaultValue any) any {
	switch v := defaultValue.(type) {
	case string:
		return v + " abc"
	case int:
		return v - 234
	}
	panic("whoops")
}

func main() {
	fmt.Println(SomeFunc(1234))
	fmt.Println(SomeFunc("test"))
}

// Output:
// 1000
// test abc
