// Closures: capture by reference, modification seen by the outer function and
// by other closures, closures returned and called later, counters and
// generators, closures over parameters and named results, recursive closures,
// and closures capturing structs, arrays and slices.
package main

import (
	"fmt"
	"strings"
)

func counter() (func() int, func()) {
	n := 0
	return func() int { n++; return n }, func() { n = 100 }
}

func adder(base int) func(int) int {
	return func(x int) int {
		base += x
		return base
	}
}

func compose(fs ...func(int) int) func(int) int {
	return func(x int) int {
		for _, f := range fs {
			x = f(x)
		}
		return x
	}
}

func fibGen() func() int64 {
	var a, b int64 = 0, 1
	return func() int64 {
		r := a
		a, b = b, a+b
		return r
	}
}

func named() (r int) {
	set := func(v int) { r = v }
	set(7)
	return
}

func memo(f func(int) int) func(int) int {
	cache := map[int]int{}
	calls := 0
	return func(n int) int {
		if v, ok := cache[n]; ok {
			return v
		}
		calls++
		v := f(n)
		cache[n] = v
		return v + calls*0
	}
}

func main() {
	next, reset := counter()
	fmt.Println(next(), next(), next())
	reset()
	fmt.Println(next())
	next2, _ := counter()
	fmt.Println(next2(), next())

	a := adder(10)
	fmt.Println(a(1), a(2), a(3))
	b := adder(0)
	fmt.Println(b(5), a(0))

	double := func(x int) int { return x * 2 }
	inc := func(x int) int { return x + 1 }
	fmt.Println(compose(double, inc)(5), compose(inc, double)(5), compose()(5))

	g := fibGen()
	var fibs []int64
	for i := 0; i < 20; i++ {
		fibs = append(fibs, g())
	}
	fmt.Println(fibs)

	fmt.Println(named())

	x := 1
	f := func() { x *= 3 }
	f()
	f()
	fmt.Println("x", x)
	x = 2
	f()
	fmt.Println("x", x)

	var fact func(int) int
	fact = func(n int) int {
		if n <= 1 {
			return 1
		}
		return n * fact(n-1)
	}
	fmt.Println(fact(10))

	type pt struct{ x, y int }
	p := pt{1, 2}
	movep := func(dx int) { p.x += dx }
	movep(5)
	fmt.Println(p)
	arr := [3]int{1, 2, 3}
	setArr := func() { arr[1] = 20 }
	cp := arr
	setArr()
	fmt.Println(arr, cp)
	sl := []int{1, 2, 3}
	app := func(v int) { sl = append(sl, v) }
	app(4)
	app(5)
	fmt.Println(sl)

	var sb strings.Builder
	write := func(s string) func() {
		return func() { sb.WriteString(s) }
	}
	ws := []func(){write("a"), write("b"), write("c")}
	for i := len(ws) - 1; i >= 0; i-- {
		ws[i]()
	}
	fmt.Println(sb.String())

	var slowCalls int
	fib := memo(func(n int) int { slowCalls++; return n * n })
	fmt.Println(fib(3), fib(3), fib(4), slowCalls)

	// Closure in a closure.
	outer := func() func() int {
		y := 0
		return func() int {
			inner := func() { y += 10 }
			inner()
			return y
		}
	}()
	fmt.Println(outer(), outer())

	// Immediately invoked.
	v := func(a, b int) int { return a*b + x }(3, 4)
	fmt.Println(v)

	// Closures sharing a captured variable through a struct.
	type acc struct {
		add func(int)
		get func() int
	}
	mk := func() acc {
		total := 0
		return acc{func(n int) { total += n }, func() int { return total }}
	}
	a1, a2 := mk(), mk()
	a1.add(3)
	a1.add(4)
	a2.add(100)
	fmt.Println(a1.get(), a2.get())

	// Capture of a parameter.
	mul := func(k int) func(int) int { return func(n int) int { k++; return n * k } }
	m := mul(2)
	fmt.Println(m(10), m(10), m(10))

	// Closure capturing an interface and a map.
	var it interface{} = "s"
	mp := map[string]int{}
	setIt := func() { it = 42; mp["k"]++ }
	setIt()
	setIt()
	fmt.Println(it, mp)
}
