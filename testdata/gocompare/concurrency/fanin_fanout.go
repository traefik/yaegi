// Fan-out and fan-in: one source read by several workers, results merged
// into one channel with a WaitGroup that closes it; the final results are
// sorted or summed so that they do not depend on the scheduler.
package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

func source(n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 1; i <= n; i++ {
			out <- i
		}
	}()
	return out
}

func worker(id int, in <-chan int) <-chan [2]int {
	out := make(chan [2]int)
	go func() {
		defer close(out)
		for v := range in {
			out <- [2]int{v, v * v * v}
		}
	}()
	return out
}

func merge[T any](cs ...<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	for _, c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for v := range c {
				out <- v
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func main() {
	src := source(40)
	var workers []<-chan [2]int
	for i := 0; i < 5; i++ {
		workers = append(workers, worker(i, src))
	}
	var res [][2]int
	for r := range merge(workers...) {
		res = append(res, r)
	}
	sort.Slice(res, func(i, j int) bool { return res[i][0] < res[j][0] })
	var sum int64
	for _, r := range res {
		sum += int64(r[1])
	}
	fmt.Println(len(res), res[0], res[39], sum)

	// Merge of string channels, sorted.
	mk := func(prefix string, n int) <-chan string {
		c := make(chan string, n)
		for i := 0; i < n; i++ {
			c <- fmt.Sprintf("%s%02d", prefix, i)
		}
		close(c)
		return c
	}
	var all []string
	for s := range merge(mk("b", 3), mk("a", 4), mk("c", 2)) {
		all = append(all, s)
	}
	sort.Strings(all)
	fmt.Println(strings.Join(all, " "))

	// Merge of no channel closes at once.
	n := 0
	for range merge[int]() {
		n++
	}
	fmt.Println("empty merge", n)

	// Fan-out with index-addressed results: each worker writes its own slot.
	words := strings.Fields("the quick brown fox jumps over the lazy dog again and again")
	counts := make([]map[rune]int, 4)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := map[rune]int{}
			for i := w; i < len(words); i += 4 {
				for _, r := range words[i] {
					m[r]++
				}
			}
			counts[w] = m
		}()
	}
	wg.Wait()
	total := map[rune]int{}
	for _, m := range counts {
		for r, c := range m {
			total[r] += c
		}
	}
	fmt.Println(total)

	// Fan-in with a quit channel: stop after the 25th value, sum them.
	quit := make(chan struct{})
	gen := func(base int) <-chan int {
		c := make(chan int)
		go func() {
			defer close(c)
			for {
				select {
				case c <- base:
				case <-quit:
					return
				}
			}
		}()
		return c
	}
	merged := merge(gen(1), gen(1), gen(1))
	got := 0
	for v := range merged {
		got += v
		if got == 25 {
			close(quit)
			break
		}
	}
	for range merged {
	}
	fmt.Println("taken", got)
}
