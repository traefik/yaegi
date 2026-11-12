// Phi coalescing (vm/phicoalesce.go): the values that flow into the phis
// of a loop header share the registers of the phis when nothing reads
// the phi after them. Each case checks a shape the rule must keep right:
// the parallel copy of the Fibonacci loop, a phi read after its next
// value is computed, a loop with a conditional back edge, nested loops,
// phis of strings, floats, bools, structs and slices, and a phi whose
// value comes from a call.
package main

import "fmt"

type P struct {
	X int
	Y string
}

func sum(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i ^ (i >> 3)
	}
	return s
}

func fib(n int) int {
	a, b := 0, 1
	for i := 0; i < n; i++ {
		a, b = b, a+b // the parallel copy: b's new value reads a's old one
	}
	return a
}

func readAfter(n int) (int, int) {
	s, t := 0, 0
	for i := 0; i < n; i++ {
		s2 := s + i
		t += s * 2 // reads s after s2 is computed: s2 must not take s's register
		s = s2
	}
	return s, t
}

func condBack(n int) (int, int) {
	i, s := 0, 0
loop:
	i++
	s += i
	if i < n {
		goto loop // a conditional back edge: s is read after the loop
	}
	return i, s
}

func nested(n, m int) int {
	t := 0
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			t += i*m + j
		}
		t -= i
	}
	return t
}

func kinds(n int) (string, float64, bool, P, []int) {
	s := ""
	f := 0.5
	b := false
	p := P{}
	var xs []int
	for i := 0; i < n; i++ {
		s += string(rune('a' + i%26))
		f = f*1.5 + float64(i)
		b = !b
		p = P{X: p.X + i, Y: s}
		xs = append(xs, p.X)
	}
	return s, f, b, p, xs
}

func calls(n int) int {
	t := 1
	for i := 0; i < n; i++ {
		t = step(t, i)
	}
	return t
}

func step(t, i int) int { return (t*31 + i) % 1000003 }

func swap3(n int) (int, int, int) {
	a, b, c := 1, 2, 3
	for i := 0; i < n; i++ {
		a, b, c = b, c, a+b+c
	}
	return a, b, c
}

func selfUse(n int) int {
	x := 1
	for i := 0; i < n; i++ {
		x = x*2 + x // x read twice by its own next value
		if x > 1000000 {
			x -= 1000000
		}
	}
	return x
}

func rangeLoops(xs []int) (int, int) {
	s, k := 0, 0
	for i, x := range xs {
		s += x * i
		k = i
	}
	m := map[int]int{1: 1, 2: 2, 3: 3}
	t := 0
	for key, v := range m {
		t += key * v
	}
	return s + t, k
}

func earlyExit(n int) int {
	s := 0
	for i := 0; ; i++ {
		s += i
		if s > n {
			return s - i // s and i used after the back edge
		}
	}
}

func main() {
	fmt.Println(sum(1000), fib(40), nested(10, 7), calls(100), selfUse(50))
	fmt.Println(readAfter(10))
	fmt.Println(condBack(10))
	fmt.Println(kinds(7))
	fmt.Println(swap3(20))
	fmt.Println(rangeLoops([]int{3, 1, 4, 1, 5}))
	fmt.Println(earlyExit(100))
}
