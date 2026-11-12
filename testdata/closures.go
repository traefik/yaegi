package main

import "fmt"

func counter() func() int {
	n := 0
	return func() int { n++; return n }
}

func main() {
	c := counter()
	fmt.Println(c(), c(), c())

	// Per-iteration loop variable (Go 1.22): each closure captures its own i.
	var fns []func() int
	for i := 0; i < 3; i++ {
		fns = append(fns, func() int { return i })
	}
	for _, f := range fns {
		fmt.Print(f(), " ")
	}
	fmt.Println()

	// Recursive closure.
	var fib func(int) int
	fib = func(n int) int {
		if n < 2 {
			return n
		}
		return fib(n-1) + fib(n-2)
	}
	fmt.Println(fib(10))

	// Closure mutating captured state across calls.
	acc := 0
	add := func(x int) { acc += x }
	for _, x := range []int{1, 2, 3, 4} {
		add(x)
	}
	fmt.Println(acc)
}
