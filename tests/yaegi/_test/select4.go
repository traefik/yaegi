package main

func main() {
	c1 := make(chan string)

	go func() { c1 <- "done" }()

	select {
	case msg1 := <-c1:
		println("received from c1:", msg1)
	}
	println("Bye")
}

// Output:
// received from c1: done
// Bye
