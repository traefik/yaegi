// copy with overlapping slices of word-sized elements (any, pointers,
// maps, funcs, chans): copyWords in vm/wordcell.go must copy backward
// when dst starts after src, and forward when it starts before, as Go
// does (memmove). Only the kinds of wordKind (pointers, maps, chans,
// funcs, interfaces, slices, unsafe.Pointer) go through copyWords.
// Strings are not one of them: the string case checks the plain copy.
package main

import "fmt"

func main() {
	// any
	a := []any{1, 2, 3, 4, 5, 6}
	copy(a[1:], a)
	fmt.Println(a)
	a = []any{1, 2, 3, 4, 5, 6}
	copy(a, a[1:])
	fmt.Println(a)
	a = []any{1, 2, 3, 4, 5, 6}
	copy(a[2:5], a[1:4])
	fmt.Println(a)

	// pointers
	xs := []int{10, 20, 30, 40, 50}
	p := []*int{&xs[0], &xs[1], &xs[2], &xs[3], &xs[4]}
	copy(p[1:], p)
	for _, q := range p {
		fmt.Print(*q, " ")
	}
	fmt.Println()
	p = []*int{&xs[0], &xs[1], &xs[2], &xs[3], &xs[4]}
	copy(p, p[2:])
	for _, q := range p {
		fmt.Print(*q, " ")
	}
	fmt.Println()

	// maps
	m := []map[string]int{{"a": 1}, {"b": 2}, {"c": 3}, {"d": 4}}
	copy(m[1:], m)
	fmt.Println(m)
	m = []map[string]int{{"a": 1}, {"b": 2}, {"c": 3}, {"d": 4}}
	copy(m, m[1:])
	fmt.Println(m)

	// funcs
	mk := func(i int) func() int { return func() int { return i } }
	f := []func() int{mk(1), mk(2), mk(3), mk(4), mk(5)}
	copy(f[1:], f)
	for _, g := range f {
		fmt.Print(g(), " ")
	}
	fmt.Println()
	f = []func() int{mk(1), mk(2), mk(3), mk(4), mk(5)}
	copy(f, f[3:])
	for _, g := range f {
		fmt.Print(g(), " ")
	}
	fmt.Println()

	// strings
	s := []string{"a", "b", "c", "d", "e", "f"}
	copy(s[1:], s)
	fmt.Println(s)
	s = []string{"a", "b", "c", "d", "e", "f"}
	copy(s, s[1:])
	fmt.Println(s)

	// chans
	cs := make([]chan int, 4)
	for i := range cs {
		cs[i] = make(chan int, 1)
		cs[i] <- i
	}
	copy(cs[1:], cs)
	for _, c := range cs[1:] {
		select {
		case v := <-c:
			fmt.Print(v, " ")
		default:
			fmt.Print("- ")
		}
	}
	fmt.Println()

	// the same backing array, through a second slice header, and the
	// count copy returns
	b := []any{"x", "y", "z", "w"}
	c := b[1:]
	n := copy(c, b)
	fmt.Println(n, b, c)
}
