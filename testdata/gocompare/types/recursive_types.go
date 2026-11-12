// Recursive types: a linked list, a binary search tree with insert,
// in-order walk, height and delete, a tree of children slices, a map of
// recursive values, mutually recursive types and a recursive func type.
package main

import (
	"fmt"
	"strings"
)

type List struct {
	Val  int
	Next *List
}

func (l *List) Push(v int) *List { return &List{v, l} }
func (l *List) String() string {
	var b strings.Builder
	b.WriteString("[")
	for n := l; n != nil; n = n.Next {
		if n != l {
			b.WriteString(" ")
		}
		fmt.Fprint(&b, n.Val)
	}
	b.WriteString("]")
	return b.String()
}
func (l *List) Reverse() *List {
	var prev *List
	for l != nil {
		l, l.Next, prev = l.Next, prev, l
	}
	return prev
}
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return 1 + l.Next.Len()
}

type Tree struct {
	Left, Right *Tree
	Key         int
}

func (t *Tree) Insert(k int) *Tree {
	if t == nil {
		return &Tree{Key: k}
	}
	if k < t.Key {
		t.Left = t.Left.Insert(k)
	} else if k > t.Key {
		t.Right = t.Right.Insert(k)
	}
	return t
}

func (t *Tree) Walk(f func(int)) {
	if t == nil {
		return
	}
	t.Left.Walk(f)
	f(t.Key)
	t.Right.Walk(f)
}

func (t *Tree) Height() int {
	if t == nil {
		return 0
	}
	return 1 + max(t.Left.Height(), t.Right.Height())
}

func (t *Tree) Delete(k int) *Tree {
	if t == nil {
		return nil
	}
	switch {
	case k < t.Key:
		t.Left = t.Left.Delete(k)
	case k > t.Key:
		t.Right = t.Right.Delete(k)
	default:
		if t.Left == nil {
			return t.Right
		}
		if t.Right == nil {
			return t.Left
		}
		m := t.Right
		for m.Left != nil {
			m = m.Left
		}
		t.Key = m.Key
		t.Right = t.Right.Delete(m.Key)
	}
	return t
}

func (t *Tree) String() string {
	if t == nil {
		return "()"
	}
	return fmt.Sprintf("(%v %d %v)", t.Left, t.Key, t.Right)
}

type Dir struct {
	Name     string
	Children []Dir
}

func (d Dir) Print(indent int, out *strings.Builder) {
	out.WriteString(strings.Repeat("  ", indent) + d.Name + "\n")
	for _, c := range d.Children {
		c.Print(indent+1, out)
	}
}

func (d Dir) Count() int {
	n := 1
	for _, c := range d.Children {
		n += c.Count()
	}
	return n
}

type JSON map[string]JSON

type Even struct{ next *Odd }
type Odd struct{ next *Even }

type State func(int) (State, string)

func start(n int) (State, string) {
	if n%2 == 0 {
		return even, "start->even"
	}
	return odd, "start->odd"
}
func even(n int) (State, string) {
	if n > 3 {
		return nil, "even->end"
	}
	return odd, "even->odd"
}
func odd(n int) (State, string) { return even, "odd->even" }

func main() {
	var l *List
	for i := 1; i <= 5; i++ {
		l = l.Push(i)
	}
	fmt.Println(l, l.Len())
	l = l.Reverse()
	fmt.Println(l, l.Len(), (*List)(nil).Len())

	var t *Tree
	for _, k := range []int{50, 30, 70, 20, 40, 60, 80, 35, 45, 65, 30} {
		t = t.Insert(k)
	}
	var keys []int
	t.Walk(func(k int) { keys = append(keys, k) })
	fmt.Println(keys, t.Height())
	fmt.Println(t)
	t = t.Delete(30)
	t = t.Delete(50)
	t = t.Delete(99)
	fmt.Println(t, t.Height())

	root := Dir{"root", []Dir{
		{"a", []Dir{{"a1", nil}, {"a2", []Dir{{"a2x", nil}}}}},
		{"b", nil},
	}}
	var sb strings.Builder
	root.Print(0, &sb)
	fmt.Print(sb.String())
	fmt.Println(root.Count())

	j := JSON{"a": JSON{"b": JSON{}, "c": nil}, "d": nil}
	fmt.Println(j, len(j["a"]), j["a"]["b"] != nil, j["a"]["c"] == nil)

	e := &Even{}
	e.next = &Odd{next: e}
	fmt.Println(e.next.next == e, e.next.next.next.next == e)

	st, msg := start(0)
	for i := 1; st != nil; i++ {
		fmt.Print(msg, " ")
		st, msg = st(i)
	}
	fmt.Println(msg)
}
