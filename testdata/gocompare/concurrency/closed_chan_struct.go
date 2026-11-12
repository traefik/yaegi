// Receive from a closed channel of a struct, array or other composite type.
package main

import "fmt"

type pair struct {
	a int
	b string
}

type inner struct {
	p pair
	x [2]int
}

func main() {
	c := make(chan pair)
	close(c)
	v, ok := <-c
	fmt.Println("a", v, ok)
	fmt.Println("b", <-c)
	v = <-c
	v.a = 3
	fmt.Println("c", v)

	ca := make(chan [3]int, 1)
	ca <- [3]int{1, 2, 3}
	close(ca)
	a, ok := <-ca
	fmt.Println("d", a, ok)
	a, ok = <-ca
	fmt.Println("e", a, ok)
	a[1] = 7
	fmt.Println("f", a)

	ci := make(chan inner)
	close(ci)
	w, ok := <-ci
	w.p.b = "x"
	w.x[0] = 1
	fmt.Println("g", w, ok)

	cp := make(chan *pair)
	close(cp)
	p, ok := <-cp
	fmt.Println("h", p == nil, ok)

	cb := make(chan pair, 2)
	cb <- pair{1, "one"}
	close(cb)
	for x := range cb {
		fmt.Println("i", x)
	}

	select {
	case v, ok := <-c:
		fmt.Println("j", v, ok)
	}
	cs := make(chan struct{})
	close(cs)
	s, ok := <-cs
	fmt.Println("k", s, ok)
}
