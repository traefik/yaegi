// Composite literals built in registers and read-only locals aliased to
// their stored value (vm/aggv.go). Each case checks what the shortcut
// must keep: the zero value of fields that the literal does not write,
// copies of struct and array fields, nested literals, literals after a
// phi, locals that are written (not aliased), receivers and parameters
// read through an alias, arrays indexed with a variable, and a literal
// whose value is used twice.
package main

import "fmt"

type P struct {
	X int
	Y string
	A [2]int
}

type Q struct {
	P
	N  int
	Ps []P
}

type R struct {
	Q Q
	M map[string]int
	_ int
	Z P
}

type shape interface{ area() int }

type sq struct{ s int }

func (q sq) area() int { return q.s * q.s }

type rect struct {
	w, h int
	tag  [3]byte
}

func (r rect) area() int        { return r.w * r.h }
func (r rect) tagAt(i int) byte { return r.tag[i] }
func (r rect) grow() rect       { r.w++; r.h++; return r }
func (r rect) self() rect       { return r }

func literals(i int) {
	p := P{X: i, Y: "y"}
	fmt.Println(p.X, p.Y, p.A)
	a := [3]int{1, i, 3}
	fmt.Println(a, len(a))
	q := Q{P: P{X: i, A: [2]int{i, i + 1}}, N: i}
	fmt.Println(q.X, q.A, q.N, q.Ps == nil)
	r := R{Q: q, M: map[string]int{"k": i}}
	fmt.Println(r.Q.N, r.M["k"], r.Z.A)
	// The literal copies its struct and array fields: later changes of
	// the sources do not show.
	inner := P{X: 1, A: [2]int{1, 1}}
	arr := [2]int{5, 6}
	q2 := Q{P: inner, N: 2}
	p2 := P{X: 7, A: arr}
	inner.X = 100
	inner.A[0] = 100
	arr[0] = 60
	fmt.Println(q2.P.X, q2.P.A, p2.A, inner.X, arr)
	// A literal used twice: both uses see the same value.
	p3 := P{X: i}
	s := []P{p3, p3}
	s[0].X = 9
	fmt.Println(p3.X, s[0].X, s[1].X)
	// Partial literal: the other fields are zero.
	p4 := P{Y: "only"}
	fmt.Println(p4.X, p4.Y, p4.A)
	// A literal of a struct with a blank field.
	r2 := R{Z: P{X: 3}}
	fmt.Println(r2.Z.X, r2.M == nil)
}

func phiLiteral(i int, b bool) P {
	y := "f"
	if b {
		y = "t"
	}
	return P{X: i, Y: y}
}

func assigned(i int) P {
	p := P{X: i}
	p.Y = "b"
	p.A[1] = i
	return p
}

func mutate(p P) (int, string) {
	p.X = 2
	p.Y += "!"
	return p.X, p.Y
}

func readOnly(p P) int { return p.X + p.A[0] + p.A[1] + len(p.Y) }

func readIndex(p P, i int) int { return p.A[i] }

func readNested(q Q, i int) (int, int, string) { return q.P.A[i], q.N, q.Y }

func passOn(p P) int {
	q := p
	return readOnly(q) + readOnly(p)
}

func addrOf(p P) *P { return &p }

func recurse(p P, n int) int {
	if n == 0 {
		return p.X
	}
	p.X++
	return recurse(p, n-1)
}

func closureOver(p P) func() int {
	return func() int { return p.X }
}

func spills(i int) {
	p := P{X: i, Y: "ab", A: [2]int{10, 20}}
	fmt.Println(mutate(p))
	fmt.Println(p.X, p.Y) // the caller's value does not change
	fmt.Println(readOnly(p), readIndex(p, 1), passOn(p))
	q := Q{P: p, N: 4}
	fmt.Println(readNested(q, 0))
	ap := addrOf(p)
	ap.X = 55
	fmt.Println(ap.X, p.X)
	fmt.Println(recurse(p, 3), p.X)
	f := closureOver(p)
	p.X = 77
	fmt.Println(f(), p.X)
	func() {
		defer func() { fmt.Println("recovered:", recover() != nil) }()
		_ = readIndex(p, 5)
	}()
}

func receivers() {
	shapes := []shape{sq{2}, rect{w: 3, h: 4, tag: [3]byte{'a', 'b', 'c'}}, sq{5}}
	total := 0
	for i := 0; i < 1000; i++ {
		for _, s := range shapes {
			total += s.area()
		}
	}
	fmt.Println(total)
	r := rect{w: 1, h: 2, tag: [3]byte{'x', 'y', 'z'}}
	fmt.Println(r.tagAt(2), r.grow().area(), r.area(), r.self() == r)
	var s shape = r
	fmt.Println(s.area(), s.(rect).tag[0])
}

func main() {
	literals(3)
	fmt.Println(phiLiteral(1, true), phiLiteral(2, false))
	fmt.Println(assigned(4))
	spills(5)
	receivers()
}
