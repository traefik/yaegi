package main

// reflect.MakeFunc with an interpreted function, reflect.StructOf with
// native field types, and the package path of unsafe.Pointer, through the
// VM reflect (GOROOT/test fixedbugs/issue30041.go, issue28748.go,
// issue30606.go, issue44830.go).

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

type ft func(int, string) (string, error)

func main() {
	var f ft
	f = reflect.MakeFunc(reflect.TypeOf(f), func(in []reflect.Value) []reflect.Value {
		s := strings.Repeat(in[1].String(), int(in[0].Int()))
		return []reflect.Value{reflect.ValueOf(s), reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())}
	}).Interface().(ft)
	s, err := f(3, "ab")
	fmt.Println(s, err)

	add := reflect.MakeFunc(reflect.TypeOf(func(int, int) int { return 0 }), func(in []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(int(in[0].Int() + in[1].Int()))}
	}).Interface().(func(int, int) int)
	fmt.Println(add(2, 3))

	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		bad := reflect.MakeFunc(reflect.TypeOf(func() error { return nil }), func([]reflect.Value) []reflect.Value {
			var x [1]reflect.Value
			return x[:]
		}).Interface().(func() error)
		bad()
	}()

	st := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.StructOf([]reflect.StructField{
			{Name: "B", Type: reflect.ArrayOf(3, reflect.SliceOf(reflect.TypeOf(uint64(0))))},
		})},
		{Name: "C", Type: reflect.TypeOf(""), Tag: `json:"c"`},
	})
	v := reflect.New(st).Elem()
	v.Field(1).SetString("x")
	fmt.Println(st, st.Field(1).Tag.Get("json"), v.Field(0).Field(0).Len(), v.Field(1))

	t := reflect.TypeOf(unsafe.Pointer(nil))
	fmt.Println(t.PkgPath(), t.Name(), t.String(), t.Kind())
}
