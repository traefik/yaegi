// Dynamic dispatch through interfaces: several types behind one interface,
// slices of interfaces, interfaces holding interfaces' dynamic values, the
// same method on value and pointer types, and fmt using String and Error.
package main

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

type Shape interface {
	Area() float64
	Perimeter() float64
}

type Rect struct{ W, H float64 }
type Circle struct{ R float64 }
type Square struct{ Rect }
type Tri struct{ A, B, C float64 }

// Each product is rounded with float64(...): once inlined, gc could fuse it
// with an addition of the caller into one FMA on some platforms.
func (r Rect) Area() float64         { return float64(r.W * r.H) }
func (r Rect) Perimeter() float64    { return float64(2 * (r.W + r.H)) }
func (c *Circle) Area() float64      { return float64(math.Pi * c.R * c.R) }
func (c *Circle) Perimeter() float64 { return float64(2 * math.Pi * c.R) }
func (t Tri) Area() float64 {
	s := (t.A + t.B + t.C) / 2
	return math.Sqrt(s * (s - t.A) * (s - t.B) * (s - t.C))
}
func (t Tri) Perimeter() float64 { return t.A + t.B + t.C }
func (t Tri) String() string     { return fmt.Sprintf("Tri(%g,%g,%g)", t.A, t.B, t.C) }

func NewSquare(s float64) Square { return Square{Rect{s, s}} }

type myErr struct{ code int }

func (e *myErr) Error() string { return fmt.Sprintf("code %d", e.code) }

type Visitor interface {
	Visit(n Node) string
}
type Node interface {
	Accept(v Visitor) string
}
type Num int
type Add struct{ L, R Node }

func (n Num) Accept(v Visitor) string { return v.Visit(n) }
func (a Add) Accept(v Visitor) string { return v.Visit(a) }

type printer struct{}

func (p printer) Visit(n Node) string {
	switch n := n.(type) {
	case Num:
		return fmt.Sprint(int(n))
	case Add:
		return "(" + n.L.Accept(p) + " + " + n.R.Accept(p) + ")"
	}
	return "?"
}

type evaluator struct{ depth *int }

func (e evaluator) Visit(n Node) string {
	*e.depth++
	switch n := n.(type) {
	case Num:
		return fmt.Sprint(int(n))
	case Add:
		var l, r int
		fmt.Sscan(n.L.Accept(e), &l)
		fmt.Sscan(n.R.Accept(e), &r)
		return fmt.Sprint(l + r)
	}
	return "?"
}

func main() {
	shapes := []Shape{Rect{3, 4}, &Circle{1}, NewSquare(2), Tri{3, 4, 5}}
	for _, s := range shapes {
		fmt.Printf("%T %.4f %.4f\n", s, s.Area(), s.Perimeter())
	}
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].Area() < shapes[j].Area() })
	for _, s := range shapes {
		fmt.Printf("%.3f ", s.Area())
	}
	fmt.Println()
	total := 0.0
	for _, s := range shapes {
		total += s.Area()
	}
	fmt.Printf("%.5f\n", total)

	fmt.Println(Tri{1, 1, 1})
	fmt.Printf("%v %s %d\n", Tri{2, 2, 2}, Tri{1, 2, 2}, Tri{1, 1, 1})

	var err error = &myErr{7}
	fmt.Println(err, err.Error())
	wrapped := fmt.Errorf("wrap: %w", err)
	fmt.Println(wrapped)
	var me *myErr
	fmt.Println(errors.As(wrapped, &me), me.code, errors.Is(wrapped, err), errors.Unwrap(wrapped) == err)

	expr := Add{Num(1), Add{Num(2), Num(3)}}
	fmt.Println(expr.Accept(printer{}))
	depth := 0
	fmt.Println(expr.Accept(evaluator{&depth}), depth)

	var sb strings.Builder
	var w fmt.Stringer = &sb
	sb.WriteString("built")
	fmt.Println(w.String(), w)
}
