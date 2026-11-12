// fmt: recursive types (trees, lists), cycles through pointers (the
// addresses are masked), a String method that recurses.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

var addr = regexp.MustCompile(`0x[0-9a-f]+`)

func mask(s string) string { return addr.ReplaceAllString(s, "PTR") }

type Tree struct {
	Val         int
	Left, Right *Tree
	Kids        []Tree
}

func (t *Tree) String() string {
	if t == nil {
		return "()"
	}
	return fmt.Sprintf("(%v %d %v)", t.Left, t.Val, t.Right)
}

type List struct {
	V    int
	Next *List
}

type Expr interface{ Eval() int }
type Num int
type Add struct{ L, R Expr }

func (n Num) Eval() int { return int(n) }
func (a Add) Eval() int { return a.L.Eval() + a.R.Eval() }

type Graph map[string][]string

type Self []Self

func main() {
	t := &Tree{Val: 2, Left: &Tree{Val: 1}, Right: &Tree{Val: 3, Right: &Tree{Val: 4}}}
	fmt.Println(t, t.Left)
	fmt.Println(mask(fmt.Sprintf("%v", *t)))
	fmt.Println(mask(fmt.Sprintf("%+v", Tree{Val: 1, Kids: []Tree{{Val: 2}, {Val: 3, Kids: []Tree{{Val: 4}}}}})))

	l := &List{V: 1}
	l.Next = &List{V: 2, Next: l} // a cycle
	fmt.Println(mask(fmt.Sprintf("%v %+v", l, *l.Next)))
	fmt.Println(mask(fmt.Sprintf("%#v", l)))

	var e Expr = Add{Num(1), Add{Num(2), Num(3)}}
	fmt.Println(e.Eval())
	fmt.Printf("%v\n%+v\n%#v\n", e, e, e)

	g := Graph{"a": {"b"}, "b": {"a", "c"}}
	fmt.Printf("%v %d\n", g, len(g))
	s := Self{Self{}, Self{Self{}}}
	fmt.Printf("%v %#v\n", s, s)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%v", []*Tree{nil, {Val: 9}})
	fmt.Println(sb.String())
}
