// slices editing: Index, IndexFunc, Contains, ContainsFunc, Insert, Delete,
// DeleteFunc, Replace, Compact, CompactFunc, Reverse, Equal, EqualFunc,
// Compare, Clone, Grow, Clip, Concat, Repeat, and their panics.
package main

import (
	"fmt"
	"slices"
	"strings"
)

func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	f()
}

func main() {
	s := []string{"a", "b", "c", "b"}
	fmt.Println(slices.Index(s, "b"), slices.Index(s, "z"), slices.Contains(s, "c"), slices.Contains(s, "C"))
	fmt.Println(slices.IndexFunc(s, func(x string) bool { return x > "a" }), slices.ContainsFunc(s, func(x string) bool { return x == "" }))

	n := []int{1, 2, 3}
	n = slices.Insert(n, 0, -1, 0)
	fmt.Println(n)
	n = slices.Insert(n, len(n), 4)
	fmt.Println(n)
	n = slices.Insert(n, 3, 10, 11, 12)
	fmt.Println(n)
	n = slices.Insert(n, 2)
	fmt.Println(n)
	try("insert", func() { slices.Insert(n, 100, 1) })

	n = slices.Delete(n, 1, 3)
	fmt.Println(n)
	d := []int{0, 1, 2, 3, 4, 5}
	tail := d[4:6]
	d2 := slices.Delete(d, 1, 3)
	fmt.Println(d2, len(d2), d[:6], tail)
	try("delete", func() { slices.Delete([]int{1, 2}, 1, 5) })
	try("delete2", func() { slices.Delete([]int{1, 2}, 2, 1) })

	e := []int{1, 2, 3, 4, 5, 6, 7, 8}
	e = slices.DeleteFunc(e, func(x int) bool { return x%3 == 0 })
	fmt.Println(e, len(e))

	r := []string{"a", "b", "c", "d"}
	r = slices.Replace(r, 1, 3, "X", "Y", "Z")
	fmt.Println(r)
	r = slices.Replace(r, 0, 2)
	fmt.Println(r)

	c := []int{1, 1, 2, 2, 2, 3, 1, 1, 4}
	c = slices.Compact(c)
	fmt.Println(c, len(c))
	w := []string{"Go", "go", "GO", "rust", "Rust", "go"}
	w = slices.CompactFunc(w, strings.EqualFold)
	fmt.Println(w)

	rv := []int{1, 2, 3, 4, 5}
	slices.Reverse(rv)
	fmt.Println(rv)
	slices.Reverse(rv[1:3])
	fmt.Println(rv)
	slices.Reverse([]int{})

	fmt.Println(slices.Equal([]int{1, 2}, []int{1, 2}), slices.Equal([]int{}, nil), slices.Equal([]int{1}, []int{1, 2}))
	fmt.Println(slices.EqualFunc([]int{1, 2}, []string{"1", "2"}, func(a int, b string) bool { return fmt.Sprint(a) == b }))
	fmt.Println(slices.Compare([]int{1, 2}, []int{1, 3}), slices.Compare([]int{1, 2}, []int{1}), slices.Compare([]int{}, []int{}))

	orig := []int{1, 2, 3}
	cl := slices.Clone(orig)
	cl[0] = 100
	fmt.Println(orig, cl, slices.Clone([]int(nil)) == nil, slices.Clone([]int{}) == nil)

	g := []int{1, 2}
	g = slices.Grow(g, 10)
	fmt.Println(g, len(g), cap(g) >= 12)
	try("grow", func() { slices.Grow(g, -1) })
	cp := slices.Clip(g)
	fmt.Println(len(cp), cap(cp))
	mk := make([]int, 2, 10)
	fmt.Println(cap(slices.Clip(mk)), cap(slices.Grow(mk, 8)))

	cc := slices.Concat([]int{1}, nil, []int{2, 3}, []int{})
	fmt.Println(cc, len(cc))
	fmt.Println(slices.Concat[[]int]() == nil)
	rp := slices.Repeat([]string{"x", "y"}, 3)
	fmt.Println(rp, len(rp))
	fmt.Println(slices.Repeat([]int{1}, 0), len(slices.Repeat([]int{}, 5)))
	try("repeat", func() { slices.Repeat([]int{1}, -1) })
}
