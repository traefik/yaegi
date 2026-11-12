// Package initialization: package-level variables are initialized in
// dependency order (not source order), then the init functions run in source
// order; several init functions in one file; variables initialized by calls
// with side effects, by closures and by multi-value calls.
package main

import "fmt"

var trace []string

func note(name string, v int) int {
	trace = append(trace, fmt.Sprintf("%s=%d", name, v))
	return v
}

var (
	a = note("a", b+c) // depends on b and c
	b = note("b", f())
	c = note("c", 3)
	d = note("d", 4)
)

func f() int { return note("f-reads-d", d) * 2 }

var e, g = pair()

func pair() (int, int) { return note("pair-e", 10), note("pair-g", h) }

var h = note("h", 7)

var table = map[string]int{"a": a, "b": b}

var double = func(x int) int { return x * 2 }

var applied = note("applied", double(c))

var initCount int

var x0 = note("x0", 0)

func init() {
	initCount++
	trace = append(trace, fmt.Sprint("init1 a=", a))
}

var x1 = note("x1", x0+1)

func init() {
	initCount++
	trace = append(trace, "init2")
	a = 1000
}

func init() {
	initCount++
	trace = append(trace, fmt.Sprint("init3 a=", a, " count=", initCount))
}

type registry map[string]func() string

var reg = registry{}

func register(name string, fn func() string) bool {
	reg[name] = fn
	trace = append(trace, "register "+name)
	return true
}

var _ = register("one", func() string { return "1" })
var _ = register("two", func() string { return "2" })

func main() {
	for _, t := range trace {
		fmt.Println(t)
	}
	fmt.Println(a, b, c, d, e, g, h, applied, x0, x1)
	fmt.Println(table, initCount, len(reg), reg["one"](), reg["two"]())
}
