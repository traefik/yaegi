// Locals whose address goes to calls: those that cannot outlive the call
// keep one cell per frame in the VM (vm/escape.go). Each run of the
// declaration must still give a zero value, and the locals whose address
// is kept must stay distinct objects.
package main

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
)

type Op struct {
	Kind int
	Arg  string
}

type Rule struct {
	ID   int
	Ops  [3]Op
	Tags []string
	Next *Rule
	M    map[string]int
}

func match(op *Op, s string) bool { return op.Kind == len(s)%3 || strings.Contains(s, op.Arg) }

func score(r *Rule, s string) int {
	n := 0
	for j := range r.Ops {
		if match(&r.Ops[j], s) {
			n += r.ID
		}
	}
	return n
}

// fill writes through the pointer: still a local of the caller.
func fill(r *Rule, i int) {
	r.ID = i
	r.Ops[i%3] = Op{Kind: i % 3, Arg: fmt.Sprint(i)}
}

var kept []*Rule

func keep(r *Rule) { kept = append(kept, r) }

func ret(r *Rule) *Rule { return r }

func viaSlice(b []int) int {
	s := 0
	for _, x := range b {
		s += x
	}
	return s
}

func setGrid(g *[3][2]int, i int) {
	for x := range g {
		for y := range g[x] {
			g[x][y] += i + x*2 + y
		}
	}
}

func sumGrid(g *[3][2]int) int {
	s := 0
	for x := range g {
		for y := range g[x] {
			s += g[x][y]
		}
	}
	return s
}

func setBig(a *[64]int, i int) {
	for j := range a {
		a[j] += i * j
	}
}

func setCookie(c *http.Cookie, i int) {
	c.Name += fmt.Sprint("n", i)
	c.Value += "v"
	c.MaxAge += i
	c.HttpOnly = c.HttpOnly != (i%2 == 0)
}

func rec(r *Rule, n int) int {
	if n == 0 {
		return r.ID
	}
	var x Rule
	x.ID = r.ID + n
	return rec(&x, n-1) + r.ID
}

func main() {
	rules := []Rule{{ID: 1, Ops: [3]Op{{0, "a"}, {1, "b"}, {2, "c"}}}, {ID: 2, Ops: [3]Op{{1, "x"}}}, {ID: 3}}
	total := 0
	for _, r := range rules {
		total += score(&r, "abc")
	}
	fmt.Println("score", total)

	// A declaration in a loop starts at zero at each iteration, even when
	// the previous iteration wrote all its fields.
	for i := 0; i < 4; i++ {
		var r Rule
		fmt.Println("zero", r.ID, r.Ops, r.Tags == nil, r.Next == nil, r.M == nil)
		fill(&r, i+1)
		r.Tags = append(r.Tags, "t")
		r.Next = &rules[0]
		r.M = map[string]int{"k": i}
		fmt.Println("filled", r.ID, r.Ops[(i+1)%3], len(r.Tags), r.Next.ID, r.M["k"])
	}

	// An array local, sliced and passed to a function that only reads it.
	for i := 0; i < 3; i++ {
		var a [4]int
		fmt.Println("array zero", a)
		for j := range a {
			a[j] = i + j
		}
		fmt.Println("array sum", viaSlice(a[:]))
	}

	// Kept: each iteration has its own object.
	for i := 0; i < 3; i++ {
		var r Rule
		r.ID = i
		keep(&r)
	}
	for _, r := range kept {
		fmt.Print(r.ID, " ")
	}
	fmt.Println()

	// Returned, then kept in a slice.
	var rs []*Rule
	for i := 0; i < 3; i++ {
		var r Rule
		r.ID = 10 + i
		rs = append(rs, ret(&r))
	}
	fmt.Println("returned", rs[0].ID, rs[1].ID, rs[2].ID)

	// Captured by a closure: per-iteration variables (Go 1.22).
	var fs []func() int
	for i := 0; i < 3; i++ {
		var r Rule
		r.ID = 20 + i
		fs = append(fs, func() int { return score(&r, "") + r.ID })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()

	// A pointer of the previous iteration, kept in a variable of the loop
	// (a phi).
	var prev *Rule
	for i := 0; i < 3; i++ {
		var r Rule
		r.ID = 30 + i
		if prev != nil {
			fmt.Println("prev", prev.ID, "cur", r.ID)
		}
		prev = &r
	}

	// A nested array local: the reset in place writes into the inner
	// arrays the local already has.
	for i := 0; i < 3; i++ {
		var g [3][2]int
		fmt.Println("grid zero", g, sumGrid(&g))
		setGrid(&g, i+1)
		fmt.Println("grid", g, sumGrid(&g))
	}

	// A big flat array local: the reset in place clears it.
	for i := 0; i < 3; i++ {
		var a [64]int
		fmt.Println("big zero", a[0], a[1], a[63], viaSlice(a[:]))
		setBig(&a, i+1)
		fmt.Println("big", a[0], a[1], a[63], viaSlice(a[:]))
	}

	// A local of a named struct of a native package.
	for i := 0; i < 3; i++ {
		var c http.Cookie
		fmt.Printf("cookie zero %q %q %d %v\n", c.Name, c.Value, c.MaxAge, c.HttpOnly)
		setCookie(&c, i)
		fmt.Printf("cookie %q %q %d %v\n", c.Name, c.Value, c.MaxAge, c.HttpOnly)
	}

	// A GC after the last use of the local, while a pointer to one of its
	// fields is still used.
	for i := 0; i < 3; i++ {
		var r Rule
		fill(&r, i+1)
		p := &r.Ops[(i+1)%3]
		runtime.GC()
		fmt.Println("field after GC", p.Kind, p.Arg, match(p, "x1"))
	}

	// Recursion: each frame has its own local.
	var r0 Rule
	r0.ID = 1
	fmt.Println("rec", rec(&r0, 5))

	// Through an interface and fmt: escapes.
	var ss []fmt.Stringer
	for i := 0; i < 2; i++ {
		var b strings.Builder
		b.WriteString(fmt.Sprint("b", i))
		ss = append(ss, &b)
	}
	fmt.Println(ss[0].String(), ss[1].String())
}
