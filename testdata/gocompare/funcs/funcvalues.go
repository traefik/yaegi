// Function values: comparison with nil, zero values in structs, maps and
// slices of funcs, method values (bound receiver copied at evaluation time)
// and method expressions, method values used in closures and goroutines, and
// funcs as map values and interface values.
package main

import (
	"fmt"
	"sort"
	"strings"
)

type counter struct{ n int }

func (c counter) Value() int { return c.n }
func (c *counter) Inc()      { c.n++ }
func (c *counter) Add(k int) int {
	c.n += k
	return c.n
}

type op func(int, int) int

func (o op) apply(a, b int) int { return o(a, b) }

type shape interface{ Area() float64 }
type sq struct{ s float64 }

func (q sq) Area() float64 { return q.s * q.s }

func main() {
	var f func()
	fmt.Println("nil func:", f == nil)
	f = func() {}
	fmt.Println("set func:", f != nil)
	type holder struct {
		cb func(string) string
	}
	var h holder
	fmt.Println("struct field nil:", h.cb == nil)
	h.cb = strings.ToUpper
	fmt.Println(h.cb("abc"))
	fs := make([]func() int, 3)
	fmt.Println("slice of nil funcs:", fs[0] == nil, fs[2] == nil)
	m := map[string]func(int, int) int{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Print(k, "=", m[k](7, 3), " ")
	}
	fmt.Println()
	fmt.Println("missing op nil:", m["div"] == nil)

	c := counter{n: 1}
	val := c.Value // receiver copied now
	inc := c.Inc   // &c bound
	c.n = 10
	fmt.Println("value method:", val(), "current:", c.Value())
	inc()
	inc()
	fmt.Println("after inc:", c.n)
	add := (&c).Add
	fmt.Println(add(5), c.n)

	ve := counter.Value
	pe := (*counter).Inc
	pv := (*counter).Value
	pe(&c)
	fmt.Println(ve(c), pv(&c))

	cp := &counter{}
	var incs []func()
	for i := 0; i < 3; i++ {
		incs = append(incs, cp.Inc)
	}
	for _, g := range incs {
		g()
	}
	fmt.Println("ptr bound:", cp.n)

	cs := []counter{{1}, {2}, {3}}
	var vals []func() int
	for _, x := range cs {
		vals = append(vals, x.Value)
	}
	cs[0].n = 100
	for _, v := range vals {
		fmt.Print(v(), " ")
	}
	fmt.Println()

	wrap := func(g func()) func() { return func() { g(); g() } }
	c2 := &counter{}
	wrap(c2.Inc)()
	fmt.Println("wrapped:", c2.n)

	done := make(chan int)
	c3 := &counter{}
	for i := 0; i < 5; i++ {
		go func(add func(int) int) {
			done <- 1
			_ = add
		}(c3.Add)
	}
	for i := 0; i < 5; i++ {
		<-done
	}
	addFn := c3.Add
	for i := 0; i < 5; i++ {
		addFn(i)
	}
	fmt.Println("c3:", c3.n)

	var o op = func(a, b int) int { return a<<1 | b }
	fmt.Println(o.apply(3, 1), op.apply(o, 4, 0))
	var s shape = sq{3}
	area := s.Area
	s = sq{10}
	fmt.Println(area(), s.Area())
	ifaceMethod := shape.Area
	fmt.Println(ifaceMethod(sq{2}))

	var any interface{} = strings.Repeat
	if r, ok := any.(func(string, int) string); ok {
		fmt.Println(r("ab", 3))
	}
	fmt.Printf("%T\n", any)
	var nilFn func(int) int
	var iface interface{} = nilFn
	fmt.Println("iface holding nil func is nil:", iface == nil)
	fp := &f
	fmt.Println("ptr to func non nil:", *fp != nil)
}
