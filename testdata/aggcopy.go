package main

import "fmt"

// copy and append copy struct and array elements by value: after
// copy(dst, src) or an append that moves the elements to a new array, a
// store into one slice does not change the other. Stores into the
// destination go into its memory, so pointers into it see them.

type S struct{ a, b int }

type nest struct {
	s  S
	xs [2]S
}

func main() {
	src := []S{{1, 2}}
	dst := make([]S, 1)
	copy(dst, src)
	dst[0] = S{9, 9}
	fmt.Println(src[0], dst[0])

	as := [][2]int{{1, 2}}
	ad := make([][2]int, 1)
	copy(ad, as)
	ad[0] = [2]int{9, 9}
	fmt.Println(as[0], ad[0])

	a := make([]S, 1, 1)
	b := append(a, S{})
	a[0] = S{7, 7}
	fmt.Println(a[0], b[0])

	// append grows the array: the old and the new array do not share
	g := []S{{1, 1}}
	h := append(g, S{2, 2}, S{3, 3})
	g[0].a = 50
	h[0].b = 60
	fmt.Println(g, h)

	// nested structs and arrays, by copy and by append
	ns := []nest{{S{1, 1}, [2]S{{2, 2}, {3, 3}}}}
	nd := make([]nest, 1)
	copy(nd, ns)
	nd[0].xs[1] = S{9, 9}
	nd[0].s.a = 8
	ng := append(ns, nest{})
	ng[0].xs[0].a = 7
	fmt.Println(ns, nd, ng[0])

	// copy in a deferred call
	func() {
		dd := make([]S, 1)
		defer func() {
			dd[0].a = 70
			fmt.Println(src[0], dd[0])
		}()
		defer copy(dd, src)
	}()

	// overlap: copy inside the same slice
	o := []S{{1, 1}, {2, 2}, {3, 3}}
	copy(o[1:], o)
	o[0].a = 10
	o[1].a = 20
	fmt.Println(o)
	o = []S{{1, 1}, {2, 2}, {3, 3}}
	copy(o, o[1:])
	o[1].a = 30
	fmt.Println(o)

	// delete with append: overlap in place
	d := []S{{1, 1}, {2, 2}, {3, 3}}
	d = append(d[:0], d[1:]...)
	d[0].a = 40
	fmt.Println(d, d[:3][2])

	// pointer into the destination sees copy and in-place append
	x := make([]S, 1, 2)
	p := &x[0]
	copy(x, []S{{5, 5}})
	fmt.Println(*p)
	q := &x[:2][1]
	x = append(x, S{6, 6})
	fmt.Println(*q, q.a)
	pa := &x[:2][1].a
	x = append(x[:1], S{8, 8})
	fmt.Println(*pa)
}
