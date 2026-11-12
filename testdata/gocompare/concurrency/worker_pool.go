// Worker pools with sync.WaitGroup: results written by index, results
// collected through a channel and sorted, WaitGroup.Go, and the per-iteration
// loop variable captured by goroutines.
package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

func collatz(n int) int {
	steps := 0
	for n != 1 {
		if n%2 == 0 {
			n /= 2
		} else {
			n = 3*n + 1
		}
		steps++
	}
	return steps
}

func main() {
	// Index-addressed results.
	inputs := []int{1, 7, 27, 97, 871, 6171, 77031}
	res := make([]int, len(inputs))
	var wg sync.WaitGroup
	for i, n := range inputs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i] = collatz(n)
		}()
	}
	wg.Wait()
	fmt.Println(res)

	// Pool of 4 workers on a job channel, results through a channel.
	type result struct {
		job, worker, value int
	}
	jobs := make(chan int)
	out := make(chan result)
	var pool sync.WaitGroup
	for w := 0; w < 4; w++ {
		pool.Add(1)
		go func(id int) {
			defer pool.Done()
			for j := range jobs {
				out <- result{job: j, worker: id, value: j * j}
			}
		}(w)
	}
	go func() {
		for j := 0; j < 30; j++ {
			jobs <- j
		}
		close(jobs)
	}()
	go func() {
		pool.Wait()
		close(out)
	}()
	var all []result
	for r := range out {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].job < all[j].job })
	sum := 0
	workersOK := true
	for i, r := range all {
		if r.job != i || r.value != i*i {
			fmt.Println("bad", r)
		}
		if r.worker < 0 || r.worker > 3 {
			workersOK = false
		}
		sum += r.value
	}
	fmt.Println(len(all), sum, workersOK)

	// WaitGroup.Go.
	var g sync.WaitGroup
	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	upper := make([]string, len(words))
	for i, w := range words {
		g.Go(func() {
			upper[i] = strings.ToUpper(w)
		})
	}
	g.Wait()
	fmt.Println(upper)

	// Wait with zero counter returns at once; reuse of a WaitGroup.
	var z sync.WaitGroup
	z.Wait()
	for round := 0; round < 3; round++ {
		var mu sync.Mutex
		n := 0
		for i := 0; i < 20; i++ {
			z.Add(1)
			go func() {
				defer z.Done()
				mu.Lock()
				n += i
				mu.Unlock()
			}()
		}
		z.Wait()
		fmt.Println("round", round, n)
	}

	// Negative counter panics.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		var bad sync.WaitGroup
		bad.Done()
	}()

	// Sorted strings from many goroutines.
	names := make(chan string, 26)
	var ng sync.WaitGroup
	for c := 'z'; c >= 'a'; c-- {
		ng.Add(1)
		go func() {
			defer ng.Done()
			names <- string(c) + string(c-32)
		}()
	}
	ng.Wait()
	close(names)
	var ns []string
	for s := range names {
		ns = append(ns, s)
	}
	sort.Strings(ns)
	fmt.Println(strings.Join(ns, ""))
}
