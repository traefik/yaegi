// A composite literal stored into memory, *p = T{...} or var x = T{...}
// for an x whose address is taken: the VM writes its fields into the
// memory that p points to (vm/aggv.go, OpStoreAgg). The fields are read
// before the memory changes, pointers into the memory see the new
// values, and the fields not named by the literal become zero.
package main

import (
	"fmt"
	"testing"
)

type pair struct{ a, b int }

type item struct {
	name string
	n    int
	ok   bool
	p    *pair
}

type outer struct {
	in  pair // a struct field: the VM builds the value, then copies it
	tag string
}

func sum(p *[3]int) int { return p[0] + p[1] + p[2] }

func swap(p *pair) { *p = pair{a: p.b, b: p.a} }

var global pair

func loop(n int) (t int) {
	for i := 0; i < n; i++ {
		var y = [3]int{i, i + 1, i + 2}
		t += sum(&y)
	}
	return t
}

var sink int

func main() {
	p := &pair{1, 2}
	q := &p.a
	swap(p)
	fmt.Println(*p, *q)

	*p = pair{b: 7}
	fmt.Println(*p, *q)

	it := &item{name: "x", n: 3, ok: true, p: p}
	n := &it.n
	*it = item{name: "y", p: &global}
	fmt.Println(it.name, it.n, *n, it.ok, it.p == &global)

	o := &outer{in: pair{1, 2}, tag: "t"}
	in := &o.in
	*o = outer{in: pair{o.in.b, o.in.a}}
	fmt.Println(*o, *in)

	arr := []pair{{1, 1}, {2, 2}}
	for i := range arr {
		arr[i] = pair{arr[i].b * 10, i}
	}
	fmt.Println(arr)

	fmt.Println(loop(4))

	// gc builds the literal on the stack: no allocation per round.
	fmt.Println("a new array per round:", testing.AllocsPerRun(20, func() { sink = loop(100) }) >= 50)
}
