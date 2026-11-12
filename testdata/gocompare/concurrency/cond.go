// sync.Cond: a deterministic ping-pong handshake with Signal, a barrier with
// Broadcast, and a bounded queue whose producers and consumers wait on
// conditions. Only facts that do not depend on the scheduler are printed.
package main

import (
	"fmt"
	"sort"
	"sync"
)

type queue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	items    []int
	max      int
}

func newQueue(max int) *queue {
	q := &queue{max: max}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

func (q *queue) put(v int) {
	q.mu.Lock()
	for len(q.items) == q.max {
		q.notFull.Wait()
	}
	q.items = append(q.items, v)
	q.notEmpty.Signal()
	q.mu.Unlock()
}

func (q *queue) get() int {
	q.mu.Lock()
	for len(q.items) == 0 {
		q.notEmpty.Wait()
	}
	v := q.items[0]
	q.items = q.items[1:]
	q.notFull.Signal()
	q.mu.Unlock()
	return v
}

func main() {
	// Ping-pong: turn says who may run next.
	var mu sync.Mutex
	c := sync.NewCond(&mu)
	turn := 0
	var log []string
	done := make(chan bool)
	for p := 0; p < 2; p++ {
		go func() {
			for i := 0; i < 5; i++ {
				mu.Lock()
				for turn%2 != p {
					c.Wait()
				}
				log = append(log, fmt.Sprintf("p%d:%d", p, i))
				turn++
				c.Broadcast()
				mu.Unlock()
			}
			done <- true
		}()
	}
	<-done
	<-done
	fmt.Println(log)

	// Barrier: all goroutines wait until ready is set.
	ready := false
	arrived := 0
	var wg sync.WaitGroup
	seen := make([]bool, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			arrived++
			c.Broadcast()
			for !ready {
				c.Wait()
			}
			seen[i] = ready
			mu.Unlock()
		}()
	}
	mu.Lock()
	for arrived < 10 {
		c.Wait()
	}
	ready = true
	c.Broadcast()
	mu.Unlock()
	wg.Wait()
	fmt.Println("arrived", arrived, seen)

	// Bounded queue.
	q := newQueue(3)
	var prod sync.WaitGroup
	for p := 0; p < 4; p++ {
		prod.Add(1)
		go func() {
			defer prod.Done()
			for i := 0; i < 25; i++ {
				q.put(p*100 + i)
			}
		}()
	}
	var got []int
	for i := 0; i < 100; i++ {
		got = append(got, q.get())
	}
	prod.Wait()
	sort.Ints(got)
	sum := 0
	for _, v := range got {
		sum += v
	}
	fmt.Println(len(got), sum, got[0], got[99], len(q.items))

	// Cond with an RWMutex read locker.
	var rw sync.RWMutex
	rc := sync.NewCond(rw.RLocker())
	flag := false
	fin := make(chan string)
	go func() {
		rw.RLock()
		for !flag {
			rc.Wait()
		}
		rw.RUnlock()
		fin <- "reader woke"
	}()
	rw.Lock()
	flag = true
	rw.Unlock()
	rc.Broadcast()
	// The reader holds the read lock until it waits, so the writer sets the
	// flag after the reader is in the wait list: Broadcast wakes it.
	fmt.Println(<-fin)
	fmt.Printf("%T\n", rc.L)
}
