package main

import "fmt"

// Recursion deeper than the plugin depth limit (100000 frames) is legal in a
// program: gc grows goroutine stacks up to 1 GB.

type node struct{ next *node }

func build(n int) *node {
	if n == 0 {
		return nil
	}
	return &node{next: build(n - 1)}
}

func count(x *node) int {
	if x == nil {
		return 0
	}
	return count(x.next) + 1
}

func sum(n int, acc [4]int) int {
	if n == 0 {
		return acc[0] + acc[1] + acc[2] + acc[3]
	}
	acc[n%4]++
	return sum(n-1, acc)
}

func main() {
	fmt.Println(count(build(400000)))
	fmt.Println(sum(300000, [4]int{}))
	done := make(chan int)
	go func() { done <- count(build(250000)) }()
	fmt.Println(<-done)
}
