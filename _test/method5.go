package main

import "fmt"

type Foo struct {
}

func (Foo) Show() {
	fmt.Println("Foo Showed")
}

func (f Foo) Call() {
	fmt.Println("Foo Called")
	f.Show()
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

func (Baz) Show() {
	fmt.Println("Baz Showed")
}

func main() {
	Foo{}.Call()
	Bar{}.Call()
	Baz{}.Call()
}

// Output:
// Foo Called
// Foo Showed
// Foo Called
// Foo Showed
// Baz Called
