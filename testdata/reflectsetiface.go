package main

// reflect stores into interfaces: Value.Set and SetMapIndex check that the
// value is assignable to the type of the place, and box it when the place
// is an interface, as gc's Value.assignTo does. The VM reflect stored a
// value given to SetMapIndex of a map of interfaces without its type (a
// later read found nil), and Set of a Rect into an interface that only
// *Rect implements did not panic. (From the reflect battery of the modes
// challenge, case reflect_set.)

import (
	"fmt"
	"reflect"
)

type Shape interface{ Area() int }

type Sq struct{ S int }

func (s Sq) Area() int { return s.S * s.S }

type Rect struct{ W, H int }

func (r *Rect) Area() int { return r.W * r.H }

type Key interface{ K() string }

type SK string

func (s SK) K() string { return string(s) }

type Conf struct {
	Name   string
	Shape  Shape
	Any    any
	ByName map[string]Shape
	ByKey  map[Key]int
}

// area does not panic on a nil interface: a wrong result is printed.
func area(s Shape) int {
	if s == nil {
		return -1
	}
	return s.Area()
}

func try(name string, f func()) {
	defer func() { fmt.Println(name, "recovered:", recover()) }()
	f()
}

func main() {
	var c Conf
	v := reflect.ValueOf(&c).Elem()

	m := reflect.MakeMap(reflect.TypeOf(map[string]Shape{}))
	m.SetMapIndex(reflect.ValueOf("sq"), reflect.ValueOf(Sq{4}))
	m.SetMapIndex(reflect.ValueOf("re"), reflect.ValueOf(&Rect{3, 3}))
	fmt.Println(m.Len(), m.MapIndex(reflect.ValueOf("sq")), m.MapIndex(reflect.ValueOf("sq")).Elem().Type())
	v.FieldByName("ByName").Set(m)
	fmt.Println(area(c.ByName["sq"]), area(c.ByName["re"]))

	// a map of the program, not made by reflect
	direct := map[string]Shape{}
	reflect.ValueOf(direct).SetMapIndex(reflect.ValueOf("x"), reflect.ValueOf(Sq{2}))
	fmt.Println(len(direct), area(direct["x"]))

	// an interface key
	km := reflect.MakeMap(reflect.TypeOf(map[Key]int{}))
	km.SetMapIndex(reflect.ValueOf(SK("a")), reflect.ValueOf(1))
	v.FieldByName("ByKey").Set(km)
	fmt.Println(c.ByKey[SK("a")], len(c.ByKey))

	// values that are not assignable panic with the message of gc
	try("Set", func() { v.FieldByName("Shape").Set(reflect.ValueOf(Rect{1, 2})) })
	try("SetMapIndex elem", func() { m.SetMapIndex(reflect.ValueOf("r"), reflect.ValueOf(Rect{1, 2})) })
	try("SetMapIndex key", func() { m.SetMapIndex(reflect.ValueOf(1), reflect.ValueOf(Sq{1})) })
	try("Set named", func() { v.FieldByName("Name").Set(reflect.ValueOf(SK("x"))) })
	fmt.Println(c.Shape == nil, m.Len(), c.Name == "")

	// assignable values are fine
	try("Set ok", func() {
		v.FieldByName("Shape").Set(reflect.ValueOf(&Rect{2, 5}))
		v.FieldByName("Any").Set(reflect.ValueOf(Rect{1, 1}))
		v.FieldByName("Name").Set(reflect.ValueOf("srv"))
	})
	fmt.Println(area(c.Shape), c.Any, c.Name)
	fmt.Printf("%T\n", c.Any)

	// an interface variable set through reflect
	var s Shape
	sv := reflect.ValueOf(&s).Elem()
	sv.Set(reflect.ValueOf(Sq{7}))
	fmt.Println(s.Area(), sv.Elem().Type())
	try("Set any to Shape", func() {
		var x any = Sq{1}
		sv.Set(reflect.ValueOf(&x).Elem())
	})
	sv.Set(reflect.Zero(sv.Type()))
	fmt.Println(s == nil)

	// a zero Value panics as in gc, not with a nil dereference of the VM
	try("Set zero", func() { sv.Set(reflect.Value{}) })
	try("SetMapIndex zero key", func() { m.SetMapIndex(reflect.Value{}, reflect.ValueOf(Sq{1})) })

	// SetIterKey and SetIterValue check as Set, with their own name
	it := reflect.ValueOf(map[SK]Sq{"k": {6}}).MapRange()
	it.Next()
	var k Key
	kv := reflect.ValueOf(&k).Elem()
	kv.SetIterKey(it)
	sv.SetIterValue(it)
	fmt.Println(k, s.Area())
	try("SetIterKey", func() { v.FieldByName("Name").SetIterKey(it) })
	try("SetIterValue", func() { v.FieldByName("ByKey").SetIterValue(it) })
}
