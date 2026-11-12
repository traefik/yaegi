// A buffered channel as a semaphore and as a token bucket: at most N holders
// at once (checked with atomics, only the bound is printed), a mutex built
// on a channel, and a barrier built on channels.
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type chanMutex chan struct{}

func newChanMutex() chanMutex { return make(chanMutex, 1) }
func (m chanMutex) Lock()     { m <- struct{}{} }
func (m chanMutex) Unlock()   { <-m }
func (m chanMutex) TryLock() bool {
	select {
	case m <- struct{}{}:
		return true
	default:
		return false
	}
}

func main() {
	const limit = 3
	sem := make(chan struct{}, limit)
	var cur, peak atomic.Int32
	var wg sync.WaitGroup
	results := make([]int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			n := cur.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			results[i] = i * 2
			cur.Add(-1)
		}()
	}
	wg.Wait()
	fmt.Println(results)
	fmt.Println("peak within limit", peak.Load() >= 1 && peak.Load() <= limit, "free", len(sem), cap(sem))

	// Channel mutex.
	m := newChanMutex()
	counter := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Lock()
			counter += i
			m.Unlock()
		}()
	}
	wg.Wait()
	fmt.Println("counter", counter)
	fmt.Println(m.TryLock(), m.TryLock())
	m.Unlock()
	fmt.Println(m.TryLock())
	m.Unlock()

	// Token bucket: tokens are prefilled, each taker takes one.
	tokens := make(chan int, 5)
	for i := 0; i < 5; i++ {
		tokens <- i
	}
	taken := make([]int, 0, 5)
	for i := 0; i < 5; i++ {
		select {
		case t := <-tokens:
			taken = append(taken, t)
		default:
			fmt.Println("bad")
		}
	}
	select {
	case <-tokens:
		fmt.Println("bad")
	default:
		fmt.Println("bucket empty", taken)
	}

	// Barrier on channels: N goroutines each wait until all arrived.
	const n = 6
	arrive := make(chan int)
	release := make(chan struct{})
	after := make([]bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			arrive <- i
			<-release
			after[i] = true
		}()
	}
	sum := 0
	for i := 0; i < n; i++ {
		sum += <-arrive
	}
	fmt.Println("all arrived", sum, after)
	close(release)
	wg.Wait()
	fmt.Println("released", after)
}
