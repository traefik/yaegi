// Locals whose address goes to a recursion (vm/escape.go): when the
// recursive functions only pass the pointer down and read or write
// through it, the local keeps one cell in its frame, as gc keeps it on the
// stack. When a function of the cycle keeps the pointer, even one that the
// cycle only reaches through another function, each run of the
// declaration must still give a distinct object.
package main

import (
	"fmt"
	"testing"
)

type node struct {
	sum  int
	hits [4]int
}

// walk and step call each other, and only write through p.
func walk(p *node, n int) {
	if n == 0 {
		return
	}
	p.sum += n
	step(p, n-1)
}

func step(p *node, n int) {
	p.hits[n%4]++
	walk(p, n)
}

// self is a direct recursion that only reads p.
func self(p *[3]int, n int) int {
	if n == 0 {
		return p[0] + p[1] + p[2]
	}
	return self(p, n-1) + p[n%3]
}

var kept []*node

// down, up and late form cycles; late keeps the pointer at the end.
func down(p *node, n int) {
	if n > 0 {
		up(p, n-1)
	}
}

func up(p *node, n int) {
	down(p, n)
	late(p, n)
}

func late(p *node, n int) {
	if n == 0 {
		kept = append(kept, p)
	}
}

func sums(rounds int) (t int) {
	for i := 0; i < rounds; i++ {
		var x node
		walk(&x, 6)
		var a [3]int
		a[i%3] = i
		t += x.sum + x.hits[1] + self(&a, 5)
	}
	return t
}

var sink int

func main() {
	fmt.Println("sums", sums(3), sums(10))

	for i := 0; i < 3; i++ {
		var x node
		x.sum = i
		down(&x, 3)
	}
	for _, p := range kept {
		fmt.Print(p.sum, " ")
	}
	fmt.Println(len(kept))

	// gc keeps x and a on the stack: no allocation per round.
	allocs := testing.AllocsPerRun(20, func() { sink = sums(100) })
	fmt.Println("a new object per round:", allocs >= 50)
}
