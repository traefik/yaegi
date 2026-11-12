package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	// Fan-out: each worker squares its input; results collected then sorted
	// so the output is deterministic.
	const n = 8
	results := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = i * i
		}(i)
	}
	wg.Wait()
	fmt.Println(results)

	// Channel pipeline with select and a done channel.
	ch := make(chan int)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			ch <- i
		}
		close(done)
	}()
	var got []int
	for {
		select {
		case v := <-ch:
			got = append(got, v)
		case <-done:
			sort.Ints(got)
			fmt.Println(got)
			return
		}
	}
}
