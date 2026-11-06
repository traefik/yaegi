package main

import (
	"fmt"
	"sync"
	"time"
)

// recv runs the same select statement concurrently from several goroutines,
// each one with its own channel.
func recv(c chan int, done chan struct{}) (int, bool) {
	select {
	case v := <-c:
		return v, true
	case <-done:
		return 0, false
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
				c <- w
				v, ok := recv(c, done)
				mu.Lock()
				if !ok {
					stuck++
				} else if v != w {
					wrong++
				}
				mu.Unlock()
				if !ok {
					return
				}
			}
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
