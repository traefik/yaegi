package main

import (
	"fmt"
	"sync"
	"time"
)

// send runs the same select statement concurrently from several goroutines,
// each one with its own channel and value. The two nil channel cases are never
// ready; they make the statement large enough to not use the fixed size buffer.
func send(c chan int, v int, done chan struct{}) bool {
	var never chan int
	select {
	case c <- v:
		return true
	case never <- v:
		return false
	case <-never:
		return false
	case <-done:
		return false
	}
}

func main() {
	const workers, rounds = 8, 1000
	done := make(chan struct{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	wrong, stuck := 0, 0

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			c := make(chan int, rounds) // only used by this goroutine
			for r := 0; r < rounds; r++ {
				if !send(c, w, done) {
					mu.Lock()
					stuck++
					mu.Unlock()
					return
				}
			}
			n := len(c)
			mu.Lock()
			wrong += rounds - n // values sent to another goroutine's channel
			for i := 0; i < n; i++ {
				if <-c != w {
					wrong++
				}
			}
			mu.Unlock()
		}(w)
	}

	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		close(done)
		<-finished
	}
	fmt.Println("wrong:", wrong, "stuck:", stuck)
}

// Output:
// wrong: 0 stuck: 0
