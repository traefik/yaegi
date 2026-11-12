package main

import "fmt"

func main() {
	const (
		Foo = iota
		Bar
		Baz
	)

	const (
		Asm = iota
		C
		Java
		Go
	)

	fmt.Println(Foo, Bar, Baz)
	fmt.Println(Asm, C, Java, Go)
}

// Output:
// 0 1 2
// 0 1 2 3
