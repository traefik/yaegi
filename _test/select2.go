// Go's _select_ lets you wait on multiple channel
// operations. Combining goroutines and channels with
// select is a powerful feature of Go.

package main

import (
	"fmt"
)

func main() {

	// For our example we'll select across two channels.
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
