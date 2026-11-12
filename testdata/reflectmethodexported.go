package main

// reflect.Method.IsExported and StructField.IsExported (expr-lang/expr).

import (
	"fmt"
	"reflect"
)

type S struct{}

func (S) Hello() string { return "hi" }
func (S) hidden()       {}

func main() {
	t := reflect.TypeOf(S{})
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		fmt.Println(m.Name, m.IsExported())
	}
	f := reflect.TypeOf(struct{ A, b int }{}).Field(1)
	fmt.Println(f.Name, f.IsExported())
}
