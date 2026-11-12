// A reflect.Type and a reflect.StructField of the program stored in fields
// of native structs (json.UnsupportedTypeError, json.UnmarshalFieldError):
// the native field gets a native reflect.Type with the names of the
// program's type, and the program reads its own type back.
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type Ch chan int

type Outer struct {
	A int
	b string
	B map[string]int
}

func main() {
	defer func() { fmt.Println("recovered:", recover()) }()
	err := &json.UnsupportedTypeError{Type: reflect.TypeOf(Ch(nil))}
	fmt.Println(err)
	fmt.Println(err.Type == reflect.TypeOf(Ch(nil)), err.Type.Kind(), err.Type)
	var nilErr json.UnsupportedTypeError
	fmt.Println(nilErr.Type == nil)
	fe := &json.UnmarshalFieldError{Key: "k", Type: reflect.TypeOf(Outer{}), Field: reflect.TypeOf(Outer{}).Field(1)}
	fmt.Println(fe.Error())
	fmt.Println(fe.Field.Name, fe.Field.Type)
	// fields of a type of the program keep that type
	it := reflect.TypeOf(Inner{})
	for i := range it.NumField() {
		f := it.Field(i)
		fe := &json.UnmarshalFieldError{Key: "k", Type: it, Field: f}
		fmt.Println(fe.Field.Name, fe.Field.Type, fe.Field.Type == f.Type)
	}
}

type In struct{ X int }

type Inner struct {
	In  In
	P   *In
	S   []In
	Out Outer
}
