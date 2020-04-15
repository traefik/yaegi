package main

type T struct {
	c1 chan string
}

func main() {
	t := &T{}
	t.c1 = make(chan string)

	go func() {
		select {
		case t.c1 <- "done":
		}
	}()

	msg1 := <-t.c1
	println("received from c1:", msg1)
}

// Output:
// received from c1: done
