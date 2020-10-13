package main

type T struct {
	c1 chan string
	c2 chan string
}

func main() {
	t := &T{}
	t.c2 = make(chan string)

	go func(c chan string) { c <- "done" }(t.c2)

	select {
	case msg := <-t.c1:
		println("received from c1:", msg)
	case <-t.c2:
	}
	println("Bye")
}

// Output:
// Bye
