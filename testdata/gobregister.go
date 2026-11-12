package main

// gob.Register and gob.RegisterName with interpreted types (as in
// gorilla/sessions): a value decoded into an interface must come back with
// its interpreted type, as a value or a pointer, also when it is nested in
// a struct, a slice or a map, and the duplicate checks of gob must panic
// like in Go.

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

type User struct {
	Username      string
	Authenticated bool
}

type Point struct{ X, Y int }

type Named struct{ N string }

type Box struct {
	Name  string
	Items []interface{}
	Meta  map[string]interface{}
	Any   interface{}
}

type WithMethod struct{ V int }

func (w WithMethod) Double() int { return 2 * w.V }

func roundTrip(in interface{}) interface{} {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&in); err != nil {
		return "encode: " + err.Error()
	}
	var out interface{}
	if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
		return "decode: " + err.Error()
	}
	return out
}

func main() {
	gob.Register(User{})
	gob.Register(&Point{})
	gob.RegisterName("custom.Named", Named{})
	gob.Register(Box{})
	gob.Register(WithMethod{})
	gob.Register(User{}) // the same type twice: allowed

	// the case of the issue: a session map of gorilla/sessions
	in := map[interface{}]interface{}{"user": &User{Username: "bob", Authenticated: true}}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		panic(err)
	}
	out := map[interface{}]interface{}{}
	if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
		panic(err)
	}
	u, ok := out["user"].(User)
	fmt.Printf("%T %v %+v\n", out["user"], ok, u)
	fmt.Printf("%v %+v %#v\n", out["user"], out["user"], out["user"])

	for _, v := range []interface{}{
		User{"al", false},
		Point{1, 2},
		&Point{3, 4},
		Named{"n"},
		WithMethod{21},
		Box{Name: "b", Items: []interface{}{User{"x", true}, &Point{5, 6}, 7}, Meta: map[string]interface{}{"n": Named{"m"}}, Any: User{"y", false}},
	} {
		r := roundTrip(v)
		fmt.Printf("%T\n", r)
		switch x := r.(type) {
		case User:
			fmt.Println("User", x.Username)
		case *Point:
			fmt.Println("*Point", x.X+x.Y)
		case Named:
			fmt.Println("Named", x.N)
		case WithMethod:
			fmt.Println("WithMethod", x.Double())
		case Box:
			u, ok := x.Items[0].(User)
			p, ok2 := x.Items[1].(*Point)
			n, ok3 := x.Meta["n"].(Named)
			a, ok4 := x.Any.(User)
			fmt.Println("Box", u.Username, ok, p.X, ok2, x.Items[2], n.N, ok3, a.Username, ok4)
		default:
			fmt.Println("unexpected", x)
		}
	}

	// an unnamed struct of the same shape as a registered type is not
	// registered
	fmt.Println(roundTrip(struct{ A int }{1}))

	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		gob.RegisterName("custom.Named", User{})
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		gob.RegisterName("other.User", User{})
	}()
}
