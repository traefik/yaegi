// Recursion: direct, mutual, deep (a few thousand frames), recursion through
// closures, function values and methods, and recursive data structures.
package main

import "fmt"

func isEven(n int) bool {
	if n == 0 {
		return true
	}
	return isOdd(n - 1)
}

func isOdd(n int) bool {
	if n == 0 {
		return false
	}
	return isEven(n - 1)
}

func ackermann(m, n int) int {
	switch {
	case m == 0:
		return n + 1
	case n == 0:
		return ackermann(m-1, 1)
	default:
		return ackermann(m-1, ackermann(m, n-1))
	}
}

func depth(n int) int {
	if n == 0 {
		return 0
	}
	var pad [4]int64
	pad[n%4] = int64(n)
	return 1 + depth(n-1) + int(pad[n%4]-int64(n))
}

func hanoi(n int, from, to, via string, moves *[]string) {
	if n == 0 {
		return
	}
	hanoi(n-1, from, via, to, moves)
	*moves = append(*moves, from+to)
	hanoi(n-1, via, to, from, moves)
}

func perms(s []int) [][]int {
	if len(s) <= 1 {
		return [][]int{append([]int(nil), s...)}
	}
	var out [][]int
	for i := range s {
		rest := append(append([]int(nil), s[:i]...), s[i+1:]...)
		for _, p := range perms(rest) {
			out = append(out, append([]int{s[i]}, p...))
		}
	}
	return out
}

type node struct {
	val      int
	children []*node
}

func (n *node) sum() int {
	s := n.val
	for _, c := range n.children {
		s += c.sum()
	}
	return s
}

func build(d int) *node {
	n := &node{val: d}
	if d > 0 {
		for i := 0; i < d; i++ {
			n.children = append(n.children, build(d-1))
		}
	}
	return n
}

func gcd(a, b int64) int64 {
	if b == 0 {
		return a
	}
	return gcd(b, a%b)
}

func sumDigits(n int) (s int) {
	if n < 10 {
		return n
	}
	defer func() { s += n % 10 }()
	return sumDigits(n / 10)
}

func main() {
	fmt.Println(isEven(10), isOdd(7), isEven(3001), isOdd(2000))
	fmt.Println(ackermann(2, 3), ackermann(3, 3))
	fmt.Println(depth(5000))
	var moves []string
	hanoi(4, "A", "C", "B", &moves)
	fmt.Println(len(moves), moves)
	fmt.Println(perms([]int{1, 2, 3}))
	fmt.Println(len(perms([]int{1, 2, 3, 4, 5})))
	fmt.Println(build(4).sum())
	fmt.Println(gcd(1071, 462), gcd(1<<40, 1<<20*3))
	fmt.Println(sumDigits(987654321))
	var fib func(int) int64
	fib = func(n int) int64 {
		if n < 2 {
			return int64(n)
		}
		return fib(n-1) + fib(n-2)
	}
	fmt.Println(fib(20))
	type fn func(fn, int) int
	self := func(f fn, n int) int {
		if n == 0 {
			return 1
		}
		return n * f(f, n-1)
	}
	fmt.Println(self(self, 10))
	var walk func(n int, acc []int) []int
	walk = func(n int, acc []int) []int {
		if n == 0 {
			return acc
		}
		return walk(n/2, append(acc, n%2))
	}
	fmt.Println(walk(37, nil))
	// Recursion that panics at the bottom and recovers at the top.
	var dive func(int)
	dive = func(n int) {
		if n == 0 {
			panic("bottom")
		}
		dive(n - 1)
	}
	func() {
		defer func() { fmt.Println("recovered", recover()) }()
		dive(1000)
	}()
}
