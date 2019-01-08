package main

import (
	"fmt"
)

func main() {
	c1 := make(chan string)
	c2 := make(chan string)

	go func() {
		toSend := "hello"
		select {
		case c2 <- toSend:
			fmt.Println("Sent", toSend, "to c2")
		}
	}()

	select {
	case msg1 := <-c1:
		fmt.Println("received from c1:", msg1)
	case msg2 := <-c2:
		fmt.Println("received from c2:", msg2)
	}
	fmt.Println("Bye")
}

// Output:
// Sent hello to c2
// received from c2: hello
// Bye
