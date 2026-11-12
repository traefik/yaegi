package main

import "fmt"

func (f *Foo) Boo() { fmt.Println(f.name, "Boo") }

type Foo struct {
	name string
	fun  func(f *Foo)
}

func main() {
	t := &Foo{name: "foo"}
	t.Boo()
}

// Output:
// foo Boo
