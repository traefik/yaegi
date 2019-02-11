package main

import "fmt"

type fii interface {
	Hello()
}

type Boo struct {
	Name string
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
	boo := Boo{"foo"}
	inCall(boo)
}

// Output:
// inCall
// Hello {foo}
// foo
