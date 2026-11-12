package main

import "fmt"

type fii interface {
	Hello()
}

type Boo struct {
	Name string
}

type Bir struct {
	Boo
}

func (b Boo) Hello() {
	fmt.Println("Hello", b)
	fmt.Println(b.Name)
}

func inCall(foo fii) {
	fmt.Println("inCall")
	foo.Hello()
}

func main() {
	bir := Bir{Boo{"foo"}}
	inCall(bir)
}

// Output:
// inCall
// Hello {foo}
// foo
