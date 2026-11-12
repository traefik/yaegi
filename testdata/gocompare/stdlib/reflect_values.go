// reflect on program types: Type and Kind of each kind, struct fields with
// StructTag.Get and Lookup, embedded fields and promoted methods,
// DeepEqual of nested program values, Value.Interface round trips and
// setting values through pointers.
package main

import (
	"fmt"
	"reflect"
)

type Inner struct {
	A []int
	M map[string]*Inner
}

type Embedded struct{ E int }

func (Embedded) Promoted() string { return "promoted" }

type Outer struct {
	Embedded
	Name   string `json:"name,omitempty" xml:"n" custom:""`
	Count  int    `json:"count"`
	In     Inner
	InP    *Inner
	Any    any
	Fn     func(int) int
	Ch     chan string
	Arr    [3]byte
	hidden bool
}

func (o Outer) Value() string { return "value:" + o.Name }
func (o *Outer) Ptr() string  { return "ptr:" + o.Name }

type MyInt int

func (m MyInt) Double() MyInt { return m * 2 }

type Shape interface{ Area() float64 }

func main() {
	var o Outer
	t := reflect.TypeOf(o)
	fmt.Println(t, t.Name(), t.Kind(), t.NumField(), t.NumMethod(), reflect.PointerTo(t).NumMethod(), t.PkgPath())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		v, ok := f.Tag.Lookup("json")
		c, cok := f.Tag.Lookup("custom")
		fmt.Printf("%d %s %v anon=%v exported=%v tag=%q json=%q/%v xml=%q custom=%q/%v index=%v\n", i, f.Name, f.Type, f.Anonymous, f.IsExported(), f.Tag, v, ok, f.Tag.Get("xml"), c, cok, f.Index)
	}
	for i := 0; i < reflect.PointerTo(t).NumMethod(); i++ {
		m := reflect.PointerTo(t).Method(i)
		fmt.Println("method", i, m.Name, m.Type)
	}
	sf, ok := t.FieldByName("E")
	fmt.Println(sf.Name, sf.Index, ok)
	_, ok = t.FieldByName("Nope")
	fmt.Println(ok)
	for _, v := range []any{MyInt(3), &o, []Outer{}, map[MyInt]Inner{}, [2]Shape{}, make(chan<- MyInt), func(MyInt, ...string) (bool, error) { return false, nil }, struct{ X int }{}, (*Shape)(nil)} {
		rt := reflect.TypeOf(v)
		fmt.Printf("%v kind=%v name=%q str=%s", rt, rt.Kind(), rt.Name(), rt.String())
		switch rt.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan:
			fmt.Print(" elem=", rt.Elem())
		case reflect.Map:
			fmt.Print(" key=", rt.Key(), " elem=", rt.Elem())
		case reflect.Func:
			fmt.Print(" in=", rt.NumIn(), " out=", rt.NumOut(), " variadic=", rt.IsVariadic(), " in1=", rt.In(1))
		}
		fmt.Println()
	}
	shapeT := reflect.TypeOf((*Shape)(nil)).Elem()
	fmt.Println(shapeT, shapeT.Kind(), shapeT.NumMethod(), reflect.TypeOf(MyInt(0)).Implements(shapeT), reflect.TypeOf(MyInt(0)).ConvertibleTo(reflect.TypeOf(0)), reflect.TypeOf(MyInt(0)).AssignableTo(reflect.TypeOf(0)))
	fmt.Println(reflect.TypeOf(o).Comparable(), reflect.TypeOf(Embedded{}).Comparable(), reflect.TypeOf(int64(0)).Size(), reflect.TypeOf([3]byte{}).Size(), reflect.TypeOf(Embedded{}).NumField())

	// DeepEqual.
	a := Outer{Name: "x", In: Inner{A: []int{1, 2}, M: map[string]*Inner{"k": {A: []int{3}}}}, InP: &Inner{}, Any: []any{1, "s", MyInt(2)}}
	b := Outer{Name: "x", In: Inner{A: []int{1, 2}, M: map[string]*Inner{"k": {A: []int{3}}}}, InP: &Inner{}, Any: []any{1, "s", MyInt(2)}}
	fmt.Println(reflect.DeepEqual(a, b))
	b.In.M["k"].A[0] = 4
	fmt.Println(reflect.DeepEqual(a, b))
	b.In.M["k"].A[0] = 3
	b.Any = []any{1, "s", 2}
	fmt.Println(reflect.DeepEqual(a, b))
	fmt.Println(reflect.DeepEqual([]int{}, []int(nil)), reflect.DeepEqual(map[int]int{}, map[int]int(nil)), reflect.DeepEqual(MyInt(1), 1), reflect.DeepEqual(nil, nil), reflect.DeepEqual(&Inner{}, &Inner{}))
	fn := func() {}
	fmt.Println(reflect.DeepEqual(fn, fn), reflect.DeepEqual(Outer{}, Outer{}))
	cyc := &Inner{M: map[string]*Inner{}}
	cyc.M["self"] = cyc
	cyc2 := &Inner{M: map[string]*Inner{}}
	cyc2.M["self"] = cyc2
	fmt.Println(reflect.DeepEqual(cyc, cyc2))

	// Values: Set through a pointer, Interface round trips.
	v := reflect.ValueOf(&o).Elem()
	v.FieldByName("Name").SetString("set")
	v.Field(2).SetInt(42)
	v.FieldByName("E").SetInt(7)
	v.FieldByName("Arr").Index(1).SetUint(200)
	v.FieldByName("Any").Set(reflect.ValueOf(MyInt(5)))
	v.FieldByName("InP").Set(reflect.New(reflect.TypeOf(Inner{})))
	v.FieldByName("InP").Elem().Field(0).Set(reflect.ValueOf([]int{9}))
	v.FieldByName("Fn").Set(reflect.ValueOf(func(i int) int { return i + 1 }))
	fmt.Println(o.Name, o.Count, o.E, o.Arr, o.Any, o.InP.A, o.Fn(1))
	hv := v.FieldByName("hidden")
	fmt.Println(hv.CanSet(), hv.CanInterface(), hv.Bool(), v.FieldByName("Name").CanSet(), reflect.ValueOf(o).Field(1).CanSet())
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		hv.SetBool(true)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		reflect.ValueOf(o).Field(1).SetString("x")
	}()
	back := v.Interface().(Outer)
	fmt.Println(back.Name, back.Value(), v.Addr().Interface().(*Outer).Ptr())
	mi := reflect.ValueOf(MyInt(21))
	fmt.Println(mi.Int(), mi.Kind(), mi.Type(), mi.Interface().(MyInt).Double(), mi.Convert(reflect.TypeOf(int64(0))).Interface())
	anyv := reflect.ValueOf(&o.Any).Elem()
	fmt.Println(anyv.Kind(), anyv.Elem().Type(), anyv.Elem().Int(), anyv.IsNil(), reflect.ValueOf(o.Ch).IsNil(), reflect.ValueOf(o.InP).IsNil())
	fmt.Println(reflect.ValueOf(Outer{}).IsZero(), reflect.ValueOf(o).IsZero(), reflect.Zero(t).Interface().(Outer).Count, reflect.Indirect(reflect.ValueOf(&o)).Type())
	var nilShape Shape
	fmt.Println(reflect.ValueOf(nilShape).IsValid(), reflect.ValueOf(nilShape).Kind())
	v.FieldByName("In").SetZero()
	fmt.Println(o.In.A == nil, v.NumField(), v.FieldByIndex([]int{0, 0}).Int())
}
