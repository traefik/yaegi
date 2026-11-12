package main

import (
	"fmt"
	"unsafe"
)

// Comparisons: blank struct fields are ignored by == and by map keys, and
// pointers to zero-size values are all equal (runtime.zerobase in gc).

type T struct {
	_, _ int
	a    int
	_    string
}

func mk(blank, a int) T {
	// blank fields can only be set through unsafe
	v := struct {
		x, y, a int
		s       string
	}{blank, blank, a, fmt.Sprint(blank)}
	return *(*T)(unsafe.Pointer(&v))
}

var zp, zq = new([0]int), new([0]int)
var zs = new(struct{})

func main() {
	t1, t2 := mk(1, 2), mk(3, 2)
	fmt.Println(t1 == t2, mk(1, 2) == mk(1, 3))
	var i1, i2 any = t1, t2
	fmt.Println(i1 == i2)
	m := map[T]int{t1: 1}
	m[t2]++
	fmt.Println(len(m), m[t1])

	// (gc folds some of these comparisons at compile time: only the
	// ones that it does not fold are printed)
	fmt.Println(zp == zq)
	var arr [4][0]byte
	fmt.Println(&arr[1] == &arr[3])
	ys := make([]struct{}, 3)
	fmt.Println(&ys[0] == &ys[2])
	zm := map[*struct{}]int{}
	zm[new(struct{})]++
	zm[&ys[1]]++
	fmt.Println(len(zm))
	var np *struct{}
	fmt.Println(np == nil, np == zs)

	// pointers inside struct keys, interfaces and arrays: zero-size and
	// array pointers keep their identity rules
	type pk struct {
		z *[0]int
		a *[2]int
	}
	xa := [2]int{1, 2}
	km := map[pk]int{}
	km[pk{zp, &xa}]++
	km[pk{zq, (*[2]int)(xa[:])}]++
	fmt.Println(len(km), pk{zp, &xa} == pk{zq, (*[2]int)(xa[:])})
	var ia, ib any = &xa, (*[2]int)(xa[:])
	fmt.Println(ia == ib, [2]*[2]int{&xa, nil} == [2]*[2]int{(*[2]int)(xa[:]), nil})
	type node struct{ next *node }
	l := &node{&node{&node{}}}
	n := 0
	for p := l; p != nil; p = p.next {
		n++
	}
	fmt.Println(n, l.next != l, l.next.next.next == nil)
}
