package main

import "fmt"

type Pong struct{}

func f(x interface{}) {
	switch x.(type) {
	case *Pong:
	case int, string:
	case nil:
	case float64:
		fmt.Println("float64")
	default:
	}
	fmt.Println("after", x)
}

func g(x interface{}) {
	switch x.(type) {
	default:
	case int:
		fmt.Println("int")
	}
	fmt.Println("after g")
}

func main() {
	f(&Pong{})
	f(1)
	f("a")
	f(nil)
	f(1.5)
	f(true)
	g(1)
	g("a")
}

// Output:
// after &{}
// after 1
// after a
// after <nil>
// float64
// after 1.5
// after true
// int
// after g
// after g
