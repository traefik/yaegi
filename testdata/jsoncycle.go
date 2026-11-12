package main

// json.Marshal of pointers that point back to themselves: encoding/json
// returns an UnsupportedValueError ("encountered a cycle via *main.N").
// In -aot mode, the AOT copy of json never found the cycle (the key of its
// check, v.Interface(), was a new box at each call) and the host died with
// a fatal Go stack overflow (vm.PtrKey). In default mode, native json only
// sees the native mirror of the interpreted types, so the type in the
// message is an unnamed struct: a known divergence (the reflect wall).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type N struct{ Next *N }

type A struct {
	Name string
	B    *B
}

type B struct{ A *A }

type Box struct{ V any }

type W struct {
	Name string
	Next *W
}

func (w *W) String() string { return "W " + w.Name }

type E struct {
	Next *E
	M    map[string]*E
}

func show(name string, v any) {
	b, err := json.Marshal(v)
	var ue *json.UnsupportedValueError
	fmt.Println(name, string(b), errors.As(err, &ue))
	fmt.Println("  ", err)
}

func main() {
	n := &N{}
	n.Next = n
	show("self", n)
	show("self in a slice", []*N{n})
	show("self in a map", map[string]*N{"n": n})

	a := &A{Name: "a"}
	a.B = &B{A: a}
	show("two types", a)
	show("two types from B", a.B)

	bx := &Box{}
	bx.V = bx
	show("through any", bx)

	w := &W{Name: "w"}
	w.Next = w
	show("with a String method", w)

	e := &E{M: map[string]*E{}}
	e.M["e"] = e
	show("through a map field", e)

	_, err := json.MarshalIndent(n, "", "  ")
	fmt.Println("MarshalIndent:", err)
	var buf bytes.Buffer
	fmt.Println("Encoder:", json.NewEncoder(&buf).Encode(a), buf.Len())

	// The encoder can be used again after the error.
	show("no cycle", &N{Next: &N{}})

	// A long chain without a cycle passes the 1000 levels after which
	// json starts to check for cycles.
	var head *N
	for i := 0; i < 1500; i++ {
		head = &N{Next: head}
	}
	b, err := json.Marshal(head)
	fmt.Println("long chain:", len(b), err)

	// The same pointer twice, but not inside itself, is not a cycle.
	shared := &N{}
	var deep any = shared
	for i := 0; i < 1100; i++ {
		deep = &Box{V: deep}
	}
	b, err = json.Marshal([]any{deep, shared, deep})
	fmt.Println("shared pointer:", len(b), err)
}
