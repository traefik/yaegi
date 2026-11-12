package main

import "fmt"

type fii interface {
	Hello()
}

type Boo struct {
	Name string
}

type Bar struct{}

func (b Bar) Hello() { fmt.Println("b:", b) }

func (b Boo) Hello() {
	fmt.Println("Hello", b)
	fmt.Println(b.Name)
}

func inCall(foo fii) {
	fmt.Println("inCall")
	switch a := foo.(type) {
	case Boo:
		a.Hello()
	default:
		fmt.Println("a:", a)
	}
}

func main() {
	boo := Boo{"foo"}
	inCall(boo)
	inCall(Bar{})
}

// Output:
// inCall
// Hello {foo}
// foo
// inCall
// a: {}
