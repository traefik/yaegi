package main

import (
	"fmt"
)

func main() {
	c1 := make(chan string)
	c2 := make(chan string)
	a := 0

	go func() {
		toSend := "hello"
		select {
		case c2 <- toSend:
			a++
		}
	}()

	select {
	case msg1 := <-c1:
		fmt.Println("received from c1:", msg1)
	case msg2 := <-c2:
		fmt.Println("received from c2:", msg2)
	}
	fmt.Println("Bye", a)
}

// Output:
// received from c2: hello
// Bye 1
