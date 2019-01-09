package main

import "time"
import "fmt"

func main() {
	c1 := make(chan string)
	c2 := make(chan string)

	go func() {
		time.Sleep(1e9)
		c1 <- "one"
	}()
	go func() {
		time.Sleep(2e9)
		c2 <- "two"
	}()

	for i := 0; i < 2; i++ {
		fmt.Println("start for")
		select {
		case msg1 := <-c1:
			fmt.Println("received", msg1)
			fmt.Println("finish 1")
		case msg2 := <-c2:
			fmt.Println("received #2", msg2)
		}
		fmt.Println("end for")
	}
	fmt.Println("Bye")
}
