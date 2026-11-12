package main

import "fmt"

type Pong struct{}

type Message struct{ s string }

func f(msg interface{}) {
	switch msg := msg.(type) {
	case *Pong:
	case *Message:
		fmt.Println("message", msg.s)
	default:
	}
	fmt.Println("after")
}

func main() {
	f(&Pong{})
	f(&Message{"a"})
	f(1)
}

// Output:
// after
// message a
// after
// after
