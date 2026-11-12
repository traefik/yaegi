// reflect: StructOf, SliceOf, MapOf, PointerTo, FuncOf and ArrayOf with
// interpreted field types; the result goes through encoding/json and fmt.
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type Color int

func (c Color) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"color-%d"`, int(c))), nil }

type Point struct{ X, Y int }

func (p Point) String() string { return fmt.Sprintf("P(%d,%d)", p.X, p.Y) }

func main() {
	st := reflect.StructOf([]reflect.StructField{
		{Name: "Name", Type: reflect.TypeOf(""), Tag: `json:"name"`},
		{Name: "C", Type: reflect.TypeOf(Color(0)), Tag: `json:"c"`},
		{Name: "P", Type: reflect.TypeOf(Point{}), Tag: `json:"p"`},
		{Name: "PS", Type: reflect.SliceOf(reflect.TypeOf(Point{})), Tag: `json:"ps"`},
	})
	fmt.Println(st.NumField(), st.Field(1).Type, st.Field(2).Type.Name())
	v := reflect.New(st).Elem()
	v.Field(0).SetString("dyn")
	v.Field(1).Set(reflect.ValueOf(Color(3)))
	v.Field(2).Set(reflect.ValueOf(Point{1, 2}))
	v.Field(3).Set(reflect.ValueOf([]Point{{3, 4}}))
	b, err := json.Marshal(v.Interface())
	fmt.Println(string(b), err)
	fmt.Printf("%v\n%+v\n", v.Interface(), v.Interface())

	nv := reflect.New(st)
	err = json.Unmarshal([]byte(`{"name":"back","p":{"X":5,"Y":6},"ps":[{"X":7}]}`), nv.Interface())
	fmt.Println(err, nv.Elem().Field(0), nv.Elem().Field(2).Interface(), nv.Elem().Field(3).Len())

	mt := reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(Point{}))
	m := reflect.MakeMap(mt)
	m.SetMapIndex(reflect.ValueOf("k"), reflect.ValueOf(Point{8, 9}))
	fmt.Println(mt, m.Interface())
	pt := reflect.PointerTo(reflect.TypeOf(Point{}))
	fmt.Println(pt, pt.Elem().NumMethod(), pt.NumMethod())
	at := reflect.ArrayOf(2, reflect.TypeOf(Color(0)))
	av := reflect.New(at).Elem()
	av.Index(1).Set(reflect.ValueOf(Color(1)))
	b, err = json.Marshal(av.Interface())
	fmt.Println(at, string(b), err)
	ft := reflect.FuncOf([]reflect.Type{reflect.TypeOf(Point{})}, []reflect.Type{reflect.TypeOf(Color(0))}, false)
	fn := reflect.MakeFunc(ft, func(args []reflect.Value) []reflect.Value {
		p := args[0].Interface().(Point)
		return []reflect.Value{reflect.ValueOf(Color(p.X + p.Y))}
	})
	fmt.Println(ft, fn.Call([]reflect.Value{reflect.ValueOf(Point{2, 3})})[0].Interface())
	f := fn.Interface().(func(Point) Color)
	fmt.Println(int(f(Point{10, 1})))
}
