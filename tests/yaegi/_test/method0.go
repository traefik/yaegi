package main

import "fmt"

type Foo struct {
}

func (Foo) Call() {
	fmt.Println("Foo Called")
}

type Bar struct {
	Foo
}

type Baz struct {
	Foo
}

func (Baz) Call() {
	fmt.Println("Baz Called")
}

func main() {
	Foo{}.Call()
	Bar{}.Call()
	Baz{}.Call()
}

// Output:
// Foo Called
// Foo Called
// Baz Called
