// Function types with methods (http.HandlerFunc style), interfaces
// satisfied by func types, func values compared with nil, closures as
// fields, middleware chains, and funcs in maps and slices.
package main

import (
	"fmt"
	"sort"
	"strings"
)

type Handler interface {
	Serve(req string) string
}

type HandlerFunc func(string) string

func (f HandlerFunc) Serve(req string) string { return f(req) }

type Middleware func(Handler) Handler

func chain(h Handler, ms ...Middleware) Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

func tag(name string) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(r string) string {
			return name + "(" + next.Serve(r) + ")"
		})
	}
}

type Op func(a, b int) int

func (o Op) Apply(a, b int) int { return o(a, b) }
func (o Op) Twice() Op          { return func(a, b int) int { return o(o(a, b), b) } }

type Pred func(int) bool

func (p Pred) And(q Pred) Pred { return func(x int) bool { return p(x) && q(x) } }
func (p Pred) Not() Pred       { return func(x int) bool { return !p(x) } }

type LessFunc func(a, b string) bool

type bySlice struct {
	s    []string
	less LessFunc
}

func (b bySlice) Len() int           { return len(b.s) }
func (b bySlice) Less(i, j int) bool { return b.less(b.s[i], b.s[j]) }
func (b bySlice) Swap(i, j int)      { b.s[i], b.s[j] = b.s[j], b.s[i] }

type Visitor func(depth int) Visitor

func main() {
	h := chain(HandlerFunc(strings.ToUpper), tag("a"), tag("b"))
	fmt.Println(h.Serve("req"))
	var hi Handler = HandlerFunc(func(s string) string { return s + s })
	fmt.Println(hi.Serve("x"))
	_, ok := hi.(HandlerFunc)
	fmt.Println(ok)
	fmt.Printf("%T\n", hi)

	add := Op(func(a, b int) int { return a + b })
	mul := Op(func(a, b int) int { return a * b })
	fmt.Println(add.Apply(2, 3), mul.Apply(2, 3), add.Twice()(1, 10), mul.Twice().Twice()(1, 2))
	ops := map[string]Op{"add": add, "mul": mul, "sub": func(a, b int) int { return a - b }}
	names := []string{}
	for k := range ops {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Print(k, "=", ops[k](7, 3), " ")
	}
	fmt.Println()

	even := Pred(func(x int) bool { return x%2 == 0 })
	big := Pred(func(x int) bool { return x > 5 })
	sel := even.And(big.Not())
	var got []int
	for i := 0; i < 12; i++ {
		if sel(i) {
			got = append(got, i)
		}
	}
	fmt.Println(got)

	s := []string{"ccc", "a", "bb", "dddd"}
	sort.Sort(bySlice{s, func(a, b string) bool { return len(a) > len(b) }})
	fmt.Println(s)

	var nf Op
	fmt.Println(nf == nil, add != nil)
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		nf.Apply(1, 2)
	}()

	var v Visitor
	count := 0
	v = func(d int) Visitor {
		count++
		if d >= 3 {
			return nil
		}
		return v
	}
	for cur, d := v, 0; cur != nil; d++ {
		cur = cur(d)
	}
	fmt.Println(count)

	// Closures capture variables, not values (one variable per iteration).
	var fs []func() int
	for i := 0; i < 3; i++ {
		fs = append(fs, func() int { return i * 10 })
	}
	x := 1
	inc := func() int { x++; return x }
	fs = append(fs, inc, inc)
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println(x)

	// Function types as struct fields.
	type Btn struct {
		Label   string
		OnClick func() string
	}
	btns := []Btn{{"ok", func() string { return "clicked ok" }}, {"none", nil}}
	for _, b := range btns {
		if b.OnClick != nil {
			fmt.Println(b.OnClick())
		} else {
			fmt.Println(b.Label, "has no handler")
		}
	}
}
