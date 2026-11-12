// Goroutines, channels and select: buffered and unbuffered channels,
// directional channels, close and the comma-ok receive, select with default,
// nil channels in select, timeouts, fan-in and fan-out, sync primitives and
// a pipeline. Every result is deterministic.
package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func producer(n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := range n {
			out <- i
		}
	}()
	return out
}

func square(in <-chan int) <-chan int {
	out := make(chan int, 2)
	go func() {
		defer close(out)
		for v := range in {
			out <- v * v
		}
	}()
	return out
}

func merge(cs ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup
	for _, c := range cs {
		wg.Add(1)
		go func(c <-chan int) {
			defer wg.Done()
			for v := range c {
				out <- v
			}
		}(c)
	}
	go func() { wg.Wait(); close(out) }()
	return out
}

type result struct {
	id  int
	val string
}

func main() {
	// Pipeline.
	sum := 0
	for v := range square(producer(10)) {
		sum += v
	}
	fmt.Println("pipeline", sum)

	// Fan-in.
	var all []int
	for v := range merge(producer(3), producer(4), producer(5)) {
		all = append(all, v)
	}
	sort.Ints(all)
	fmt.Println("merge", all)

	// Buffered channel: len, cap, close, comma-ok.
	c := make(chan string, 3)
	c <- "a"
	c <- "b"
	fmt.Println(len(c), cap(c))
	close(c)
	v, ok := <-c
	fmt.Println(v, ok)
	v, ok = <-c
	fmt.Println(v, ok)
	v, ok = <-c
	fmt.Printf("%q %v\n", v, ok)

	// select with default: nothing ready.
	empty := make(chan int)
	select {
	case x := <-empty:
		fmt.Println("got", x)
	default:
		fmt.Println("default")
	}

	// select on nil channels: they are never ready.
	var nilc chan int
	ready := make(chan int, 1)
	ready <- 5
	select {
	case <-nilc:
		fmt.Println("nil recv")
	case nilc <- 1:
		fmt.Println("nil send")
	case x := <-ready:
		fmt.Println("ready", x)
	}

	// select send and receive in a loop, with channels turned off by nil.
	a, b := producer(3), producer(2)
	got := 0
	for a != nil || b != nil {
		select {
		case x, ok := <-a:
			if !ok {
				a = nil
				continue
			}
			got += x
		case x, ok := <-b:
			if !ok {
				b = nil
				continue
			}
			got += 100 * x
		}
	}
	fmt.Println("nil-out", got)

	// Timeout with time.After.
	never := make(chan int)
	select {
	case <-never:
		fmt.Println("never")
	case <-time.After(5 * time.Millisecond):
		fmt.Println("timeout")
	}

	// context cancellation seen from a goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		<-ctx.Done()
		done <- ctx.Err()
	}()
	cancel()
	fmt.Println("ctx", <-done)

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel2()
	<-ctx2.Done()
	fmt.Println("ctx2", ctx2.Err(), context.Cause(ctx2))

	// Worker pool with results sorted.
	jobs := make(chan int)
	results := make(chan result)
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- result{j, fmt.Sprintf("w?-%d", j*j)}
			}
			_ = w
		}()
	}
	go func() {
		for i := range 20 {
			jobs <- i
		}
		close(jobs)
	}()
	go func() { wg.Wait(); close(results) }()
	var rs []result
	for r := range results {
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].id < rs[j].id })
	fmt.Println(len(rs), rs[0], rs[19])

	// Mutex, RWMutex, atomic, Once, Cond.
	var mu sync.Mutex
	var rw sync.RWMutex
	var cnt int64
	var once sync.Once
	var onceN int
	m := map[int]int{}
	var wg2 sync.WaitGroup
	for i := range 50 {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			once.Do(func() { onceN++ })
			atomic.AddInt64(&cnt, 2)
			mu.Lock()
			m[i%5]++
			mu.Unlock()
			rw.RLock()
			_ = len(m)
			rw.RUnlock()
		}()
	}
	wg2.Wait()
	fmt.Println("sync", onceN, atomic.LoadInt64(&cnt), m)

	var cmu sync.Mutex
	cond := sync.NewCond(&cmu)
	readyN := 0
	var wg3 sync.WaitGroup
	for range 3 {
		wg3.Add(1)
		go func() {
			defer wg3.Done()
			cmu.Lock()
			for readyN == 0 {
				cond.Wait()
			}
			readyN++
			cmu.Unlock()
		}()
	}
	time.Sleep(time.Millisecond)
	cmu.Lock()
	readyN = 1
	cond.Broadcast()
	cmu.Unlock()
	wg3.Wait()
	fmt.Println("cond", readyN)

	// Unbuffered ping-pong keeps order.
	ping, pong := make(chan int), make(chan int)
	go func() {
		for v := range ping {
			pong <- v + 1
		}
		close(pong)
	}()
	var seq []int
	for i := 0; i < 5; i++ {
		ping <- i * 10
		seq = append(seq, <-pong)
	}
	close(ping)
	_, okp := <-pong
	fmt.Println("pingpong", seq, okp)

	// Channel of channels, and chan of func.
	reqs := make(chan chan int)
	go func() {
		n := 0
		for r := range reqs {
			n++
			r <- n
		}
	}()
	for range 3 {
		r := make(chan int)
		reqs <- r
		fmt.Print(<-r, " ")
	}
	close(reqs)
	fmt.Println()
	fc := make(chan func() string, 1)
	fc <- func() string { return "func over chan" }
	fmt.Println((<-fc)())

	// errgroup-like: first error wins.
	errc := make(chan error, 3)
	for i := range 3 {
		go func() {
			if i == 1 {
				errc <- fmt.Errorf("job %d failed", i)
				return
			}
			errc <- nil
		}()
	}
	var firstErr error
	for range 3 {
		if err := <-errc; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	fmt.Println(firstErr)

	// Ticker stops.
	tk := time.NewTicker(time.Millisecond)
	ticks := 0
	for range tk.C {
		ticks++
		if ticks == 3 {
			tk.Stop()
			break
		}
	}
	fmt.Println("ticks", ticks)
}
