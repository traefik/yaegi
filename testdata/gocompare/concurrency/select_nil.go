// select: nil channels are never ready, default runs when no case is ready,
// an empty-case select with default, send cases, and the classic loop that
// sets a channel to nil once it is closed to merge two inputs.
package main

import "fmt"

func main() {
	var nilc chan int
	select {
	case v := <-nilc:
		fmt.Println("bad recv", v)
	case nilc <- 1:
		fmt.Println("bad send")
	default:
		fmt.Println("default with nil channels")
	}

	c := make(chan int, 1)
	select {
	case v := <-c:
		fmt.Println("bad", v)
	default:
		fmt.Println("empty buffer: default")
	}
	c <- 7
	select {
	case v, ok := <-c:
		fmt.Println("got", v, ok)
	default:
		fmt.Println("bad default")
	}
	c <- 8
	select {
	case c <- 9:
		fmt.Println("bad send on full")
	default:
		fmt.Println("full buffer: default")
	}
	<-c
	select {
	case c <- 9:
		fmt.Println("sent 9, len", len(c))
	default:
		fmt.Println("bad default")
	}

	// Only default.
	select {
	default:
		fmt.Println("only default")
	}

	// Closed channel is always ready.
	cl := make(chan string)
	close(cl)
	for i := 0; i < 3; i++ {
		select {
		case s, ok := <-cl:
			fmt.Printf("closed ready %q %v\n", s, ok)
		default:
			fmt.Println("bad")
		}
	}

	// Ready case plus nil case: always the ready one.
	r := make(chan int, 10)
	hits := 0
	for i := 0; i < 10; i++ {
		r <- i
	}
	for i := 0; i < 10; i++ {
		select {
		case <-nilc:
			fmt.Println("bad nil")
		case v := <-r:
			hits += v
		}
	}
	fmt.Println("hits", hits)

	// Merge two inputs, disabling each after close.
	a := make(chan int)
	b := make(chan int)
	go func() {
		for i := 0; i < 5; i++ {
			a <- i
		}
		close(a)
	}()
	go func() {
		for i := 100; i < 103; i++ {
			b <- i
		}
		close(b)
	}()
	sumA, sumB, nA, nB := 0, 0, 0, 0
	for a != nil || b != nil {
		select {
		case v, ok := <-a:
			if !ok {
				a = nil
				continue
			}
			sumA += v
			nA++
		case v, ok := <-b:
			if !ok {
				b = nil
				continue
			}
			sumB += v
			nB++
		}
	}
	fmt.Println("merge", nA, sumA, nB, sumB)

	// Send through select to a goroutine.
	out := make(chan int)
	done := make(chan int)
	go func() {
		s := 0
		for v := range out {
			s += v
		}
		done <- s
	}()
	for i := 1; i <= 10; i++ {
		select {
		case out <- i * i:
		case <-nilc:
			fmt.Println("bad")
		}
	}
	close(out)
	fmt.Println("squares", <-done)

	// Non-blocking send on an unbuffered channel with no receiver.
	u := make(chan int)
	select {
	case u <- 1:
		fmt.Println("bad")
	default:
		fmt.Println("no receiver: default")
	}
}
