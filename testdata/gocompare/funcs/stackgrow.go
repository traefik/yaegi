// A context of the VM starts with a small register stack and grows it when a
// call needs more. This program grows it in many ways, with values that live
// across the growth: deep recursion, pointers to locals (&x), closures that
// capture locals, defer and recover, and callbacks of native code (sort.Slice,
// strings.Map) that recurse again. A register read from an old stack would
// print a wrong value.
package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type big struct {
	a, b, c, d, e, f, g, h int
	s                      string
}

// deep recurses n times; each frame keeps a pointer to its own local and a
// closure over it, and checks both after the deeper calls return.
func deep(n int) int {
	x := n * 3
	p := &x
	inc := func() { x++ }
	var arr [16]int
	for i := range arr {
		arr[i] = n + i
	}
	r := 0
	if n > 0 {
		r = deep(n - 1)
	}
	inc()
	if *p != n*3+1 || arr[15] != n+15 {
		panic(fmt.Sprint("bad frame at depth ", n))
	}
	return r + *p + arr[0]
}

// wide has many registers: a call to it grows a small stack at once.
func wide(n int) big {
	v := big{n, n + 1, n + 2, n + 3, n + 4, n + 5, n + 6, n + 7, fmt.Sprint(n)}
	if n > 0 {
		w := wide(n - 1)
		v.a += w.a
		v.s += "," + w.s
	}
	return v
}

// recurse panics at the bottom; a deferred call in each frame changes the
// named result after recover, so the defers run on grown stacks.
func recurse(n int) (r int) {
	defer func() {
		if e := recover(); e != nil {
			r = n * 1000
			return
		}
		r++
	}()
	if n == 0 {
		panic(errors.New("bottom"))
	}
	return recurse(n-1) + 1
}

func main() {
	fmt.Println(deep(500))
	w := wide(60)
	fmt.Println(w.a, len(w.s), w.s[:20])
	fmt.Println(recurse(300))

	// A native callback (sort.Slice calls less) that recurses: each call of
	// less runs on its own context, which grows from a small stack.
	xs := []int{5, 3, 9, 1, 7, 2, 8, 6, 4, 0}
	sort.Slice(xs, func(i, j int) bool {
		return deep(xs[i]+40)-deep(40) < deep(xs[j]+40)-deep(40)
	})
	fmt.Println(xs)

	// A callback of strings.Map, nested in a callback of sort.Slice.
	words := []string{"delta", "alpha", "charlie", "bravo"}
	sort.Slice(words, func(i, j int) bool {
		up := strings.Map(func(r rune) rune {
			if deep(50) < 0 {
				return r
			}
			return r - 'a' + 'A'
		}, words[i])
		return up < strings.ToUpper(words[j])
	})
	fmt.Println(words)

	// Goroutines also start with a small stack.
	done := make(chan int)
	for i := range 4 {
		go func() { done <- deep(200 + i) }()
	}
	sum := 0
	for range 4 {
		sum += <-done
	}
	fmt.Println(sum)
}
