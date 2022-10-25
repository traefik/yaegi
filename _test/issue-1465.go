package main

import (
	"fmt"
)

func SomeFunc[T int | string](defaultValue T) T {
	switch v := any(&defaultValue).(type) {
	case *string:
		*v = *v + " abc"
	case *int:
		*v -= 234
	}
	return defaultValue
}

func main() {
	fmt.Println(SomeFunc("test"))
	fmt.Println(SomeFunc(1234))
}

// Output:
// test abc
// 1000
