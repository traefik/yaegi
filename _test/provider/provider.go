package provider

import "fmt"

const (
	SomeString = "constant string"
)

type T1 struct {
	Name string
}

func (t *T1) Info() {
	fmt.Println(t.Name)
}

func Bar() {
	Foo()
}

func Sample() {
	fmt.Println("Hello from Provider")
}
