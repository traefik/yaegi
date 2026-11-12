// container/ring: New, Len, Next, Prev, Move, Do, Link and Unlink, on rings
// of ints and strings, and the zero Ring.
package main

import (
	"container/ring"
	"fmt"
)

func dump(r *ring.Ring) []any {
	var out []any
	r.Do(func(v any) { out = append(out, v) })
	return out
}

func main() {
	r := ring.New(5)
	fmt.Println(r.Len())
	for i := range r.Len() {
		r.Value = i
		r = r.Next()
	}
	fmt.Println(dump(r), r.Value)
	fmt.Println(r.Move(2).Value, r.Move(-1).Value, r.Move(7).Value, r.Prev().Value)

	sum := 0
	r.Do(func(v any) { sum += v.(int) })
	fmt.Println(sum)

	// Unlink two elements after r.
	removed := r.Unlink(2)
	fmt.Println(dump(r), dump(removed), r.Len(), removed.Len())

	// Link another ring.
	s := ring.New(2)
	s.Value = "a"
	s.Next().Value = "b"
	rest := r.Link(s)
	fmt.Println(dump(r), rest.Value, r.Len())

	// Linking a ring with itself element: removes elements between.
	t := ring.New(4)
	for i := range 4 {
		t.Value = i * 10
		t = t.Next()
	}
	cut := t.Link(t.Move(2))
	fmt.Println(dump(t), dump(cut))

	// The zero Ring is a ring of one element.
	var z ring.Ring
	fmt.Println(z.Len(), z.Next() == &z, z.Prev() == &z, z.Value)
	var nr *ring.Ring
	fmt.Println(nr.Len(), ring.New(0) == nil, ring.New(-1) == nil)

	// Move by 0 and by a multiple of the length.
	u := ring.New(3)
	for i := range 3 {
		u.Value = string(rune('x' + i))
		u = u.Next()
	}
	fmt.Println(u.Move(0).Value, u.Move(3).Value, u.Move(-4).Value, dump(u))
	fmt.Println(u.Unlink(0) == nil, u.Unlink(3).Len(), dump(u))
}
