// Goroutines and closures that capture loop variables: each iteration has
// its own variable (Go 1.22 semantics) for three-clause loops, range over
// ints, slices and maps; go statements with arguments evaluated at the go
// statement; and method values started as goroutines.
package main

import (
	"fmt"
	"sort"
	"sync"
)

type acc struct {
	mu  sync.Mutex
	sum int
}

func (a *acc) add(n int, wg *sync.WaitGroup) {
	defer wg.Done()
	a.mu.Lock()
	a.sum += n
	a.mu.Unlock()
}

type val int

func (v val) report(out chan<- int) { out <- int(v) * 2 }

func main() {
	var wg sync.WaitGroup
	got := make([]int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = i
		}()
	}
	wg.Wait()
	fmt.Println(got)

	// Range over int.
	sq := make([]int, 8)
	for i := range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); sq[i] = i * i }()
	}
	wg.Wait()
	fmt.Println(sq)

	// Range over a slice: value variable per iteration.
	names := []string{"x", "y", "z"}
	out := make([]string, len(names))
	for i, n := range names {
		wg.Add(1)
		go func() { defer wg.Done(); out[i] = n + n }()
	}
	wg.Wait()
	fmt.Println(out)

	// Range over a map, results collected and sorted.
	m := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4}
	res := make(chan string, len(m))
	for k, v := range m {
		go func() { res <- fmt.Sprint(k, "=", v*10) }()
	}
	var rs []string
	for range m {
		rs = append(rs, <-res)
	}
	sort.Strings(rs)
	fmt.Println(rs)

	// Arguments are evaluated at the go statement.
	x := 1
	ch := make(chan int)
	go func(v int) { ch <- v }(x)
	r := <-ch
	x = 100
	fmt.Println("arg", r, x)

	// Closures captured before the goroutines start.
	var fs []func() int
	for i := 0; i < 5; i++ {
		fs = append(fs, func() int { return i * 100 })
	}
	vals := make([]int, len(fs))
	for i, f := range fs {
		wg.Add(1)
		go func() { defer wg.Done(); vals[i] = f() }()
	}
	wg.Wait()
	fmt.Println(vals)

	// A post statement that steps by 3: each goroutine sees its own value.
	seen := make([]int, 0)
	var mu sync.Mutex
	for i := 0; i < 20; i += 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			seen = append(seen, i)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Ints(seen)
	fmt.Println(seen)

	// Method values as goroutines.
	a := &acc{}
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go a.add(i, &wg)
	}
	wg.Wait()
	fmt.Println("acc", a.sum)

	rep := make(chan int, 3)
	for _, v := range []val{1, 2, 3} {
		go v.report(rep)
	}
	t := 0
	for i := 0; i < 3; i++ {
		t += <-rep
	}
	fmt.Println("reports", t)

	// A goroutine started with a builtin-like call through a func value.
	f := fmt.Sprintf
	sc := make(chan string)
	go func() { sc <- f("%03d", 7) }()
	fmt.Println(<-sc)
}
