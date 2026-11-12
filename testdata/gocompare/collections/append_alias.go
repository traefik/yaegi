// Append and aliasing: appends within a capacity fixed by make share the
// backing array, appends past it do not touch the old slice, and append of
// a slice to itself, of strings to byte slices and of nil works.
package main

import "fmt"

func main() {
	s := make([]int, 3, 10)
	t := append(s, 4)
	t[0] = 99
	fmt.Println(s, t, len(s), cap(s), len(t))
	u := append(s, 5)
	fmt.Println(t, u, t[3], u[3])
	s[1] = 7
	fmt.Println(s, t, u)

	// Full capacity: the next append copies.
	f := make([]int, 2, 2)
	g := append(f, 1)
	g[0] = 50
	fmt.Println(f, g)

	// Append of a slice to itself.
	a := []int{1, 2, 3}
	a = append(a, a...)
	fmt.Println(a, len(a))
	a = append(a[:2], a[4:]...)
	fmt.Println(a, len(a))

	// Append of a string to a byte slice.
	b := []byte("ab")
	b = append(b, "cdé"...)
	fmt.Println(b, string(b), len(b))

	// Append to nil, nothing appended.
	var n []string
	fmt.Println(n == nil, len(n))
	n2 := append(n)
	fmt.Println(n2 == nil)
	n3 := append(n, []string{}...)
	fmt.Println(n3 == nil)
	n = append(n, "x", "y")
	fmt.Println(n, n == nil)

	// Many appends keep the values.
	var big []int64
	for i := int64(0); i < 1000; i++ {
		big = append(big, i*i)
	}
	sum := int64(0)
	for _, v := range big {
		sum += v
	}
	fmt.Println(len(big), big[999], sum)

	// Appending structs and pointers.
	type P struct{ X, Y int }
	ps := make([]P, 0, 4)
	ps = append(ps, P{1, 2}, P{3, 4})
	q := ps[:1]
	q = append(q, P{9, 9})
	fmt.Println(ps, q)

	// Append in a function: the caller sees the change of elements, not len.
	h := make([]int, 2, 5)
	add := func(x []int) []int { x = append(x, 3); x[0] = 1; return x }
	r := add(h)
	fmt.Println(h, r, h[:3])

	// Append of a 3-index slice copies.
	base := []int{1, 2, 3, 4, 5}
	lim := base[1:3:3]
	lim = append(lim, 100)
	lim[0] = -2
	fmt.Println(base, lim)
	open := base[1:3]
	open = append(open, 200)
	fmt.Println(base, open)
}
