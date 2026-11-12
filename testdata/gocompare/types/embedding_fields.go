// Struct embedding: promoted fields at several depths, shadowing by a
// shallower field, embedded pointers (also nil ones, recovered), embedded
// non-struct named types, and %+v of embedded structs.
package main

import "fmt"

type Base struct {
	ID   int
	Name string
}

type Mid struct {
	Base
	Name  string // shadows Base.Name
	Level int
}

type Top struct {
	*Mid
	Extra string
}

type Count int

type Wrapper struct {
	Count
	Label string
}

type A struct{ V int }
type B struct{ V int }

type Both struct {
	A
	B
	W int
}

type Inner struct{ X int }
type Outer struct {
	Inner
	Y int
}

func main() {
	m := Mid{Base: Base{ID: 1, Name: "base"}, Name: "mid", Level: 2}
	fmt.Println(m.ID, m.Name, m.Base.Name, m.Level)
	m.ID = 10
	m.Base.ID++
	fmt.Println(m.Base.ID)
	fmt.Printf("%v\n%+v\n", m, m)

	t := Top{Mid: &m, Extra: "e"}
	fmt.Println(t.ID, t.Name, t.Base.Name, t.Mid.Level, t.Extra)
	t.Level = 5
	fmt.Println(m.Level) // through the pointer
	t.Base.Name = "changed"
	fmt.Println(m.Base.Name)

	var nt Top
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(nt.ID)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		nt.Level = 3
	}()
	fmt.Println(nt.Mid == nil, nt.Extra == "")

	w := Wrapper{Count: 4, Label: "l"}
	w.Count++
	fmt.Println(w.Count, w, int(w.Count)*2)

	// Ambiguous field V is not used; the explicit paths work.
	b := Both{A{1}, B{2}, 3}
	fmt.Println(b.A.V, b.B.V, b.W)
	fmt.Printf("%+v\n", b)

	o := Outer{Inner{1}, 2}
	o.X = 7
	fmt.Println(o, o.Inner, o.X)
	po := &o
	po.X++
	fmt.Println(o.X)

	// Embedded struct copies on assignment.
	o2 := o
	o2.X = 100
	fmt.Println(o.X, o2.X)
	in := o.Inner
	in.X = -1
	fmt.Println(o.X, in.X)

	// Slice of structs with embedding.
	items := []Mid{{Base: Base{ID: 1}}, {Base: Base{ID: 2}, Name: "n"}}
	for i := range items {
		items[i].ID *= 10
	}
	fmt.Println(items[0].ID, items[1].ID, items[1].Name)

	// Anonymous struct embedding a named type.
	an := struct {
		Base
		Z int
	}{Base{9, "an"}, 1}
	fmt.Println(an.ID, an.Name, an.Z)
	fmt.Printf("%+v\n", an)
}
