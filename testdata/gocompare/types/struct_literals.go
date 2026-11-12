// Struct literals (keyed, positional, nested, pointer, partial), anonymous
// structs, zero values, %v and %+v of structs, and copies on assignment.
package main

import "fmt"

type Point struct {
	X, Y int
}

type Rect struct {
	Min, Max Point
	Name     string
	Tags     []string
}

type Mixed struct {
	B   bool
	I8  int8
	U16 uint16
	F   float64
	S   string
	P   *Point
	A   [3]int
	M   map[string]int
}

func main() {
	p1 := Point{1, 2}
	p2 := Point{X: 3}
	p3 := Point{Y: -4}
	var p4 Point
	fmt.Println(p1, p2, p3, p4)
	fmt.Printf("%v %+v %#v\n", p1, p2, p3)

	r := Rect{Min: Point{0, 0}, Max: Point{10, 20}, Name: "r1"}
	fmt.Printf("%v\n%+v\n", r, r)
	r.Tags = append(r.Tags, "a", "b")
	fmt.Println(r.Tags, len(r.Tags))

	pp := &Point{5, 6}
	pp.X += 10
	fmt.Println(*pp, pp.X, (*pp).Y)

	rects := []Rect{{Name: "a"}, {Min: Point{1, 1}, Name: "b"}}
	fmt.Println(len(rects), rects[1].Min.X, rects[0].Name)
	ptrs := []*Point{{1, 1}, {2, 2}}
	fmt.Println(*ptrs[0], *ptrs[1])
	mp := map[string]Point{"o": {}, "x": {1, 0}}
	fmt.Println(mp)
	arr := [2]Point{{7, 8}}
	fmt.Println(arr)

	var m Mixed
	fmt.Printf("%+v\n", m)
	fmt.Println(m.P == nil, m.M == nil, m.A)
	m2 := Mixed{B: true, I8: -5, U16: 65535, F: 1.5, S: "s", P: &Point{1, 2}, A: [3]int{1, 2, 3}, M: map[string]int{"k": 1}}
	fmt.Println(m2.B, m2.I8, m2.U16, m2.F, m2.S, *m2.P, m2.A, m2.M)

	// Anonymous structs.
	anon := struct {
		A int
		B string
	}{1, "x"}
	fmt.Printf("%v %+v %T\n", anon, anon, anon)
	people := []struct {
		Name string
		Age  int
	}{{"ann", 30}, {"bob", 25}}
	for _, pe := range people {
		fmt.Println(pe.Name, pe.Age)
	}
	var empty struct{}
	fmt.Printf("%v %+v %T\n", empty, empty, empty)
	nested := struct {
		In struct{ V int }
	}{}
	nested.In.V = 9
	fmt.Println(nested)

	// Assignment copies.
	a := p1
	a.X = 100
	fmt.Println(p1, a)
	r2 := r
	r2.Max.Y = 99
	r2.Tags[0] = "changed" // shared backing array
	fmt.Println(r.Max, r2.Max, r.Tags, r2.Tags)
	arr2 := arr
	arr2[0].X = -1
	fmt.Println(arr, arr2)

	// Struct values in maps are not addressable: copy, change, put back.
	v := mp["x"]
	v.Y = 50
	mp["x"] = v
	fmt.Println(mp["x"])

	// Fields of array elements and pointers.
	ps := make([]Point, 3)
	for i := range ps {
		ps[i].X = i
		ps[i].Y = i * i
	}
	fmt.Println(ps)
	for _, q := range ps {
		q.X = 1000 // copy
	}
	fmt.Println(ps)
}
