// Closures and loop variables: Go 1.22 per-iteration variables in every
// loop form, captures by reference, closures that outlive their frame, and
// goroutines that capture loop variables.
package main

import (
	"fmt"
	"sort"
	"sync"
)

type pair struct{ k, v string }

func main() {
	// 3-clause loop: each iteration has its own i, also when the body
	// changes i (the change is copied to the next iteration).
	var fs []func() int
	for i := 0; i < 6; i++ {
		if i == 2 {
			i++ // skip 3
		}
		fs = append(fs, func() int { return i })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()

	// The closure changes its own copy: the next iteration starts from the
	// changed value (the copy is made at the end of the body).
	var incs []func()
	for i := 0; i < 3; i++ {
		incs = append(incs, func() { i += 10 })
		incs[len(incs)-1]()
		fmt.Print(i, " ")
	}
	fmt.Println()

	// Pointers to the loop variable differ per iteration.
	var ptrs []*int
	for i := range 3 {
		ptrs = append(ptrs, &i)
	}
	fmt.Println(*ptrs[0], *ptrs[1], *ptrs[2], ptrs[0] != ptrs[1])

	// range over slice, map, string, channel, int: key and value captured.
	var out []string
	var gs []func() string
	for i, s := range []string{"a", "b", "c"} {
		gs = append(gs, func() string { return fmt.Sprint(i, s) })
	}
	for i, r := range "hé!" {
		gs = append(gs, func() string { return fmt.Sprint(i, string(r)) })
	}
	ch := make(chan int, 3)
	ch <- 7
	ch <- 8
	ch <- 9
	close(ch)
	for v := range ch {
		gs = append(gs, func() string { return fmt.Sprint("ch", v) })
	}
	for _, g := range gs {
		out = append(out, g())
	}
	fmt.Println(out)

	m := map[string]string{"x": "1", "y": "2", "z": "3"}
	var ps []func() pair
	for k, v := range m {
		ps = append(ps, func() pair { return pair{k, v} })
	}
	var got []string
	for _, p := range ps {
		q := p()
		got = append(got, q.k+"="+q.v)
	}
	sort.Strings(got)
	fmt.Println(got)

	// Goroutines that capture the loop variable.
	var mu sync.Mutex
	var wg sync.WaitGroup
	seen := make([]int, 0, 10)
	for i := 0; i < 10; i++ {
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

	// A variable declared outside the loop is shared by all closures.
	shared := 0
	var adds []func()
	for j := 0; j < 4; j++ {
		adds = append(adds, func() { shared += j })
	}
	for _, a := range adds {
		a()
	}
	fmt.Println(shared)

	// Nested closures capture variables of several frames.
	mk := func(base int) func(int) func() int {
		return func(step int) func() int {
			n := base
			return func() int { n += step; return n }
		}
	}
	c1 := mk(100)(5)
	c2 := mk(0)(1)
	fmt.Println(c1(), c1(), c2(), c1(), c2())

	// Recursive closure.
	var fib func(int) int
	fib = func(n int) int {
		if n < 2 {
			return n
		}
		return fib(n-1) + fib(n-2)
	}
	fmt.Println(fib(20))

	// Captured named results changed by a deferred closure.
	fmt.Println(namedResult())

	// Labeled continue keeps per-iteration variables.
	var lfs []func() string
outer:
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if j > i {
				continue outer
			}
			lfs = append(lfs, func() string { return fmt.Sprint(i, j) })
		}
	}
	for _, f := range lfs {
		fmt.Print(f(), ";")
	}
	fmt.Println()
}

func namedResult() (n int, s string) {
	defer func() { n *= 2; s += "!" }()
	n, s = 21, "ok"
	return n, s
}
