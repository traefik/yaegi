package main

import "fmt"

type Pong struct{}

func f(x interface{}) int {
	r := 0
	switch v := x.(type) {
	case int, string:
	case nil:
	case *Pong:
		r = 1
	case float64:
		fmt.Println("float64", v)
		r = 2
	default:
	}
	return r
}

func g(x interface{}) {
	switch v := x.(type) {
	default:
	case int:
		fmt.Println("int", v)
	case string:
	}
	fmt.Println("after g")
}

func h(x interface{}) {
	switch v := x.(type) {
	case int:
		fmt.Println("int", v)
	case nil:
	}
	fmt.Println("after h")
}

func main() {
	fmt.Println(f(1), f("a"), f(nil), f(&Pong{}), f(1.5), f(true))
	g(1)
	g("a")
	g(true)
	h(1)
	h(nil)
}

// Output:
// float64 1.5
// 0 0 0 1 2 0
// int 1
// after g
// after g
// after g
// int 1
// after h
// after h
