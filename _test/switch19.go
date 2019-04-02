package main

import "fmt"

type fii interface {
	Hello()
}

type Bir struct{}

func (b Bir) Yo() {
	fmt.Println("Yo", b)
}

func (b Bir) Hello() {
	fmt.Println("Hello", b)
}

type Boo struct {
	Name string
}

func (b Boo) Hello() {
	fmt.Println("Hello", b)
	fmt.Println(b.Name)
}

type Bar struct{}

func (b Bar) Hello() { fmt.Println("b:", b) }

func inCall(foo fii) {
	fmt.Println("inCall")
	switch a := foo.(type) {
	case Boo, Bir:
		a.Hello()
	case Bir:
		a.Yo()
	default:
		fmt.Println("a:", a)
	}
}

func main() {
	boo := Bir{}
	inCall(boo)
	inCall(Bar{})
}

// Error:
// 37:2: duplicate case Bir in type switch
