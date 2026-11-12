// Panics of channel operations, recovered: send on a closed channel, close
// of a closed channel, close of a nil channel, with their exact messages
// and the runtime.Error interface.
package main

import (
	"errors"
	"fmt"
	"runtime"
)

func try(name string, f func()) {
	defer func() {
		r := recover()
		if r == nil {
			fmt.Println(name, "no panic")
			return
		}
		err, isErr := r.(error)
		_, isRT := r.(runtime.Error)
		fmt.Printf("%s: %v (error=%v runtime.Error=%v)\n", name, r, isErr, isRT)
		if isErr {
			var re runtime.Error
			fmt.Println("  errors.As:", errors.As(err, &re), err.Error())
		}
	}()
	f()
}

func main() {
	try("send on closed", func() {
		c := make(chan int, 1)
		close(c)
		c <- 1
	})
	try("send on closed unbuffered", func() {
		c := make(chan string)
		close(c)
		c <- "x"
	})
	try("close closed", func() {
		c := make(chan int)
		close(c)
		close(c)
	})
	try("close nil", func() {
		var c chan int
		close(c)
	})
	try("close nil send-only", func() {
		var c chan<- int
		close(c)
	})
	try("close ok", func() {
		c := make(chan int, 3)
		c <- 1
		close(c)
	})
	try("select send on closed", func() {
		c := make(chan int, 1)
		close(c)
		select {
		case c <- 1:
		default:
			fmt.Println("default chosen")
		}
	})
	try("recv closed fine", func() {
		c := make(chan int)
		close(c)
		v, ok := <-c
		fmt.Println("  recv", v, ok)
	})
	// Panic in the second statement, the first one has run.
	n := 0
	try("partial", func() {
		c := make(chan int, 2)
		c <- 1
		n++
		close(c)
		n++
		c <- 2
		n++
	})
	fmt.Println("n =", n)

	// A goroutine that panics on a closed channel recovers and reports.
	res := make(chan string)
	c := make(chan int)
	close(c)
	go func() {
		defer func() {
			res <- fmt.Sprint("goroutine recovered: ", recover())
		}()
		c <- 5
	}()
	fmt.Println(<-res)
}
