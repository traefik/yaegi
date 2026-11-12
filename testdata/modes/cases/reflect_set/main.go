// reflect: Value.Set into interfaces and fields, New, MakeSlice, MakeMap,
// Append, SetMapIndex, Convert, Zero, Indirect with interpreted types.
package main

import (
	"fmt"
	"reflect"
)

type Shape interface{ Area() int }

type Sq struct{ S int }

func (s Sq) Area() int { return s.S * s.S }

type Rect struct{ W, H int }

func (r *Rect) Area() int { return r.W * r.H }

type Conf struct {
	Name   string
	Port   int
	Shape  Shape
	Any    any
	List   []Sq
	ByName map[string]Shape
	Ptr    *Sq
	Inner  struct{ X int }
	hidden int
}

type Celsius float64

// area does not panic on a nil interface: a wrong result is printed, not
// turned into a crash of the program.
func area(s Shape) int {
	if s == nil {
		return -1
	}
	return s.Area()
}

func main() {
	var c Conf
	v := reflect.ValueOf(&c).Elem()
	v.FieldByName("Name").SetString("srv")
	v.Field(1).SetInt(8080)
	v.FieldByName("Shape").Set(reflect.ValueOf(Sq{3}))
	fmt.Println(c.Name, c.Port, c.Shape.Area())
	v.FieldByName("Shape").Set(reflect.ValueOf(&Rect{2, 5}))
	fmt.Println(c.Shape.Area())
	v.FieldByName("Any").Set(reflect.ValueOf(Rect{1, 1}))
	fmt.Printf("%T\n", c.Any)

	l := reflect.MakeSlice(reflect.TypeOf([]Sq{}), 0, 2)
	l = reflect.Append(l, reflect.ValueOf(Sq{1}), reflect.ValueOf(Sq{2}))
	v.FieldByName("List").Set(l)
	fmt.Println(c.List, len(c.List))

	m := reflect.MakeMap(reflect.TypeOf(map[string]Shape{}))
	m.SetMapIndex(reflect.ValueOf("sq"), reflect.ValueOf(Sq{4}))
	m.SetMapIndex(reflect.ValueOf("re"), reflect.ValueOf(&Rect{3, 3}))
	v.FieldByName("ByName").Set(m)
	fmt.Println(area(c.ByName["sq"]), area(c.ByName["re"]))

	p := reflect.New(reflect.TypeOf(Sq{}))
	p.Elem().Field(0).SetInt(6)
	v.FieldByName("Ptr").Set(p)
	fmt.Println(c.Ptr != nil && c.Ptr.Area() == 36, reflect.Indirect(p).Interface())

	v.FieldByName("Inner").Field(0).SetInt(11)
	fmt.Println(c.Inner.X, v.FieldByName("hidden").CanSet(), v.FieldByName("Name").CanSet())

	// An interface variable set through reflect.
	var s Shape
	sv := reflect.ValueOf(&s).Elem()
	sv.Set(reflect.ValueOf(Sq{7}))
	fmt.Println(s.Area(), sv.Elem().Type())
	sv.Set(reflect.Zero(sv.Type()))
	fmt.Println(s == nil)

	// Convert and named numeric types.
	cv := reflect.ValueOf(21.5).Convert(reflect.TypeOf(Celsius(0)))
	fmt.Printf("%T %v\n", cv.Interface(), cv.Float())
	var x any = Celsius(3)
	fmt.Println(reflect.ValueOf(x).Kind(), reflect.ValueOf(x).Type().Name())

	// Assignability errors.
	func() {
		defer func() { fmt.Println("recovered:", recover() != nil) }()
		v.FieldByName("Shape").Set(reflect.ValueOf(Rect{1, 2}))
	}()

	// A new value of an interpreted type, filled and asserted.
	nv := reflect.New(reflect.TypeOf(Conf{})).Interface().(*Conf)
	nv.Port = 1
	fmt.Println(nv.Port, reflect.DeepEqual(*nv, Conf{Port: 1}))

	// Swapper and Copy.
	arr := []Sq{{1}, {2}, {3}}
	reflect.Swapper(arr)(0, 2)
	fmt.Println(arr)
	dst := make([]Sq, 2)
	fmt.Println(reflect.Copy(reflect.ValueOf(dst), reflect.ValueOf(arr)), dst)
}
