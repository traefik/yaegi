// The reflect iterators of Go 1.26 (Type.Fields, Type.Methods, Value.Fields,
// Value.Methods): the yield of the program went to the twin as a
// func(reflect.StructField) bool, and the call panicked ("reflect: Call
// using func(reflect.StructField) bool as type func(vm.StructField) bool").
package main

import (
	"fmt"
	"reflect"
	"time"
)

type S struct {
	A int
	B string `json:"b"`
	c bool
}

func (S) Hello() string   { return "hi" }
func (s S) Add(n int) int { return s.A + n }
func (s *S) Set(v int)    { s.A = v }

func main() {
	s := S{A: 1, B: "x"}
	t := reflect.TypeOf(s)
	for f := range t.Fields() {
		fmt.Println("field", f.Name, f.Type, f.Tag, f.Index, f.IsExported())
	}
	for m := range t.Methods() {
		fmt.Println("method", m.Name, m.Type, m.Index)
	}
	for m := range reflect.TypeOf(&s).Methods() {
		fmt.Println("ptr method", m.Name, m.Type)
	}
	for f, v := range reflect.ValueOf(s).Fields() {
		fmt.Println("vfield", f.Name, v)
	}
	for m, v := range reflect.ValueOf(s).Methods() {
		fmt.Println("vmethod", m.Name, v.Type())
		if m.Name == "Add" {
			fmt.Println(v.Call([]reflect.Value{reflect.ValueOf(41)})[0])
		}
	}
	for f := range t.Fields() {
		if f.Name == "B" {
			break
		}
		fmt.Println("before B", f.Name)
	}
	n := 0
	for f := range reflect.TypeOf(time.Time{}).Fields() {
		n++
		_ = f
	}
	fmt.Println("time fields", n)
	for m := range reflect.TypeOf(time.Duration(0)).Methods() {
		if m.Name == "String" {
			fmt.Println("dur", m.Type)
		}
	}
	ft := reflect.TypeOf(func(int, string) error { return nil })
	for in := range ft.Ins() {
		fmt.Println("in", in)
	}
	for out := range ft.Outs() {
		fmt.Println("out", out)
	}
}
