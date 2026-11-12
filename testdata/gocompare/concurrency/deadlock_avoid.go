// Patterns that avoid deadlocks and goroutine leaks, with fixed results:
// locks taken in a fixed order, buffered result channels so senders never
// block, try-send with select default, a done channel that frees blocked
// senders, and sync.Mutex values copied by value are separate.
package main

import (
	"fmt"
	"sync"
)

type account struct {
	id  int
	mu  sync.Mutex
	bal int
}

func transfer(from, to *account, amount int) {
	first, second := from, to
	if to.id < from.id {
		first, second = to, from
	}
	first.mu.Lock()
	defer first.mu.Unlock()
	second.mu.Lock()
	defer second.mu.Unlock()
	from.bal -= amount
	to.bal += amount
}

func main() {
	accs := []*account{{id: 0, bal: 1000}, {id: 1, bal: 1000}, {id: 2, bal: 1000}}
	var wg sync.WaitGroup
	for i := 0; i < 300; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			from := accs[i%3]
			to := accs[(i+1)%3]
			if i%2 == 0 {
				from, to = to, from
			}
			transfer(from, to, i%7)
		}()
	}
	wg.Wait()
	total := 0
	for _, a := range accs {
		total += a.bal
	}
	fmt.Println("total", total)

	// Buffered results: the senders never block even if nobody reads all.
	results := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- i
		}()
	}
	wg.Wait()
	fmt.Println("buffered", len(results))
	for i := 0; i < 3; i++ {
		<-results
	}
	fmt.Println("left", len(results))

	// Try-send drops values when full.
	box := make(chan int, 2)
	dropped := 0
	for i := 0; i < 5; i++ {
		select {
		case box <- i:
		default:
			dropped++
		}
	}
	fmt.Println("kept", len(box), "dropped", dropped, <-box, <-box)

	// A done channel frees senders blocked on an unbuffered channel.
	done := make(chan struct{})
	work := make(chan int)
	var exited sync.WaitGroup
	for i := 0; i < 5; i++ {
		exited.Add(1)
		go func() {
			defer exited.Done()
			for j := 0; ; j++ {
				select {
				case work <- i*100 + j:
				case <-done:
					return
				}
			}
		}()
	}
	count := 0
	for range 20 {
		<-work
		count++
	}
	close(done)
	exited.Wait()
	fmt.Println("read", count, "all senders exited")

	// Copies of a struct with a Mutex are independent (taken before lock).
	type guarded struct {
		mu sync.Mutex
		n  int
	}
	g1 := &guarded{n: 1}
	g2 := &guarded{n: 2}
	g1.mu.Lock()
	ok := g2.mu.TryLock()
	fmt.Println("independent", ok)
	g2.mu.Unlock()
	g1.mu.Unlock()

	// Recursive work with a WaitGroup passed down.
	var tree func(depth int, wg *sync.WaitGroup, out chan<- int)
	tree = func(depth int, wg *sync.WaitGroup, out chan<- int) {
		defer wg.Done()
		out <- depth
		if depth == 0 {
			return
		}
		wg.Add(2)
		go tree(depth-1, wg, out)
		go tree(depth-1, wg, out)
	}
	nodes := make(chan int, 1<<7)
	var tw sync.WaitGroup
	tw.Add(1)
	go tree(6, &tw, nodes)
	tw.Wait()
	close(nodes)
	counts := make([]int, 7)
	for d := range nodes {
		counts[d]++
	}
	fmt.Println("tree", counts)
}
