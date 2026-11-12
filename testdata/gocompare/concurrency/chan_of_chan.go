// Channels of channels: a request carries its own reply channel, a server
// goroutine answers each request, and replies are read in request order.
package main

import (
	"fmt"
	"strings"
)

type request struct {
	op    string
	a, b  int
	reply chan int
}

func server(reqs <-chan request, quit <-chan chan string) {
	count := 0
	for {
		select {
		case r := <-reqs:
			count++
			switch r.op {
			case "add":
				r.reply <- r.a + r.b
			case "mul":
				r.reply <- r.a * r.b
			case "sub":
				r.reply <- r.a - r.b
			default:
				r.reply <- -1
			}
		case q := <-quit:
			q <- fmt.Sprintf("served %d", count)
			return
		}
	}
}

func main() {
	reqs := make(chan request)
	quit := make(chan chan string)
	go server(reqs, quit)
	ops := []string{"add", "mul", "sub", "div"}
	for i := 0; i < 8; i++ {
		r := request{op: ops[i%4], a: i + 10, b: i, reply: make(chan int)}
		reqs <- r
		fmt.Println(r.op, r.a, r.b, "=", <-r.reply)
	}

	// Send many requests first, with buffered replies, then read in order.
	var replies []chan int
	for i := 0; i < 5; i++ {
		rc := make(chan int, 1)
		replies = append(replies, rc)
		reqs <- request{op: "mul", a: i, b: i, reply: rc}
	}
	var out []string
	for _, rc := range replies {
		out = append(out, fmt.Sprint(<-rc))
	}
	fmt.Println(strings.Join(out, ","))

	q := make(chan string)
	quit <- q
	fmt.Println(<-q)

	// A chain of channels: each goroutine passes value+1 to the next.
	const n = 50
	first := make(chan int)
	in := first
	for i := 0; i < n; i++ {
		next := make(chan int)
		go func(in <-chan int, out chan<- int) {
			out <- 1 + <-in
		}(in, next)
		in = next
	}
	first <- 0
	fmt.Println("chain", <-in)

	// chan chan chan.
	ccc := make(chan chan chan int, 1)
	cc := make(chan chan int, 1)
	c := make(chan int, 1)
	c <- 7
	cc <- c
	ccc <- cc
	fmt.Println(<-<-<-ccc)
	fmt.Printf("%T\n", ccc)
}
