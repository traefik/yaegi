// append(s, x1, ..., xk) compiled without the varargs array of go/ssa
// (vm/appendv.go): the elements come from registers. Each case checks the
// semantics of append that the shortcut must keep: writes into spare
// capacity, a new array on growth, copies of struct and array elements,
// every element representation (Go []byte, cells, zero-size elements,
// slices in native memory).
package main

import (
	"fmt"
	"strings"
)

type P struct {
	X int
	Y []int
	A [2]int
}

type Q struct {
	P
	Name string
}

func bytes() {
	var b []byte
	for i := 0; i < 20; i++ {
		b = append(b, byte('a'+i%26))
	}
	fmt.Println(string(b), len(b), cap(b))
	b = append(b, '!', '?')
	fmt.Println(string(b), len(b))
	c := []byte("ab")
	c = append(c, 'c')
	fmt.Println(string(c), len(c))
	e := []byte{}
	e = append(e, 1, 2, 3)
	fmt.Println(e, len(e))
	// Spare capacity is shared, as in Go.
	base := make([]byte, 2, 8)
	b1 := append(base, 7)
	b2 := append(base, 9)
	fmt.Println(b1[2], b2[2], len(base), cap(base))
	// A string built from the slice does not change with later appends.
	s := string(b1)
	b1 = append(b1, 'x')
	fmt.Println(s, string(b1))
}

func ints() {
	var s []int
	for i := 0; i < 5; i++ {
		s = append(s, i*i)
	}
	fmt.Println(s, len(s))
	s = append(s, 10, 20, 30)
	fmt.Println(s, len(s))
	a := make([]int, 2, 10)
	x := append(a, 7)
	y := append(a, 8)
	fmt.Println(x[2], y[2]) // the same spare element, as in Go
	z := append(s[:1], 99)  // writes into s
	fmt.Println(z, s[1])
	var u []uint16
	u = append(u, 65535, 1)
	fmt.Println(u)
	var f []float64
	f = append(f, 1.5, 2.5)
	fmt.Println(f)
	var ss []string
	ss = append(ss, "a", "b")
	ss = append(ss, "c")
	fmt.Println(ss, len(ss))
	var xs []any
	xs = append(xs, 1, "two", nil, 3.0)
	fmt.Println(xs...)
	var bs []bool
	bs = append(bs, true, false)
	fmt.Println(bs)
}

func structs() {
	p := P{X: 1, Y: []int{1}, A: [2]int{1, 2}}
	var ps []P
	ps = append(ps, p)
	p.X = 9
	p.A[0] = 9
	p.Y[0] = 9 // the slice is shared, as in Go
	fmt.Println(ps[0].X, ps[0].A, ps[0].Y, p.X, p.A, p.Y)
	ps = append(ps, p, P{X: 3})
	ps[1].X = 8
	fmt.Println(len(ps), ps[0].X, ps[1].X, ps[2].X, p.X)
	// Appending to spare capacity copies the struct into the array.
	buf := make([]P, 0, 4)
	q := P{X: 5}
	buf = append(buf, q)
	q.X = 6
	fmt.Println(buf[0].X, q.X)
	// Growth gives every element its own copy.
	grown := append(buf, q, q, q, q)
	grown[0].X = 50
	fmt.Println(buf[0].X, grown[0].X, len(grown))
	// Embedded structs and arrays.
	qq := Q{P: P{X: 1}, Name: "n"}
	var qs []Q
	qs = append(qs, qq)
	qq.P.X = 2
	qq.Name = "m"
	fmt.Println(qs[0].P.X, qs[0].Name, qq.P.X, qq.Name)
	var arrs [][2]int
	arr := [2]int{1, 2}
	arrs = append(arrs, arr)
	arr[0] = 3
	fmt.Println(arrs, arr)
	// Pointers to structs share the struct.
	var pps []*P
	pp := &P{X: 1}
	pps = append(pps, pp)
	pp.X = 2
	fmt.Println(pps[0].X)
	// Slices of slices share the inner array.
	inner := []int{1, 2}
	var outer [][]int
	outer = append(outer, inner)
	inner[0] = 7
	fmt.Println(outer)
}

func zeroSize() {
	var z []struct{}
	z = append(z, struct{}{}, struct{}{})
	z = append(z, struct{}{})
	fmt.Println(len(z))
	var e [][0]int
	e = append(e, [0]int{})
	fmt.Println(len(e))
}

func native() {
	parts := strings.Split("a,b", ",")
	parts = append(parts, "c")
	parts = append(parts, "d", "e")
	fmt.Println(parts, len(parts))
	fields := strings.Fields(" x y ")
	fields = append(fields[:1], "z")
	fmt.Println(fields)
}

func inLoop() {
	// The same append site, many times, with growth: the result is what
	// Go gives.
	var s []int
	sum := 0
	for i := 0; i < 1000; i++ {
		s = append(s, i)
		sum += s[i]
	}
	fmt.Println(len(s), sum, s[999])
	var b []byte
	for i := 0; i < 300; i++ {
		b = append(b, byte(i))
	}
	fmt.Println(len(b), b[255], b[299])
}

// allOrNothing: several elements go in one append. When they do not all
// fit in the spare capacity, Go allocates a new array and writes nothing
// into the old one (GOROOT test fixedbugs/issue79909.go); when they fit,
// they go into the shared array.
func allOrNothing() {
	s := []int64{1, 2, 3, 4} // cap 4
	t := s[3:3]              // len 0, cap 1
	t = append(t, 9, 9)      // two do not fit: a new array
	fmt.Println(s, t, cap(t) >= 2)
	u := s[3:3]
	u = append(u, 9) // one fits: s[3] changes
	fmt.Println(s, u)
	b := make([]byte, 4, 4)
	b2 := b[3:3]
	b2 = append(b2, 9, 9)
	fmt.Println(b, b2)
	c := make([]byte, 4, 6)
	c2 := c[3:3] // cap 3
	c2 = append(c2, 9, 9)
	fmt.Println(c, c2, len(c2), cap(c2))
	ps := []P{{X: 1}, {X: 2}}
	q := ps[1:1] // cap 1
	q = append(q, P{X: 8}, P{X: 9})
	fmt.Println(ps[1].X, q[0].X, q[1].X)
	r := ps[1:1]
	r = append(r, P{X: 7})
	fmt.Println(ps[1].X, r[0].X)
	// The range of the GOROOT test: the loop sees the original array.
	var w []int64
	for i := int64(1); i <= 4; i++ {
		w = append(w, i)
	}
	sum := int64(0)
	for idx, v := range w {
		sum = sum*10 + v
		if idx == 0 {
			w = w[3:3]
			w = append(w, 9, 9)
		}
	}
	fmt.Println(sum)
	// Five elements: the temporary slice path.
	var five []int
	five = append(five, 1, 2, 3, 4, 5)
	base := make([]int, 2, 4)
	more := append(base, 1, 2, 3, 4, 5)
	base[0] = 9
	fmt.Println(five, more, len(more))
}

func main() {
	bytes()
	ints()
	structs()
	zeroSize()
	native()
	inLoop()
	allOrNothing()
}
