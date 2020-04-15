package main

type T struct {
	c1 chan string
}

func main() {
	t := &T{}
	t.c1 = make(chan string)

	go func(c chan string) { c <- "done" }(t.c1)

	select {
	case msg1 := <-t.c1:
		println("received from c1:", msg1)
	}
	println("Bye")
}

// Output:
// received from c1: done
// Bye
