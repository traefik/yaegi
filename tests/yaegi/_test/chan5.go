package main

import "time"

func main() {

	// For our example we'll select across two channels.
	c1 := make(chan string)
	c2 := make(chan string)

	// Each channel will receive a value after some amount
	// of time, to simulate e.g. blocking RPC operations
	// executing in concurrent goroutines.
	go func() {
		//time.Sleep(1 * time.Second)
		time.Sleep(1e9)
		c1 <- "one"
	}()
	go func() {
		time.Sleep(2e9)
		c2 <- "two"
	}()

	msg1 := <-c1
	println(msg1)

	msg2 := <-c2
	println(msg2)
}
