package main

import (
	"fmt"
	"reflect"
)

type Rule struct {
	Name string
	Tags []string
	M    map[string]*Rule
}

func main() {
	a := Rule{"a", []string{"x"}, map[string]*Rule{"k": {Name: "b"}}}
	b := Rule{"a", []string{"x"}, map[string]*Rule{"k": {Name: "b"}}}
	c := Rule{"a", []string{"y"}, nil}
	fmt.Println(reflect.DeepEqual(a, b), reflect.DeepEqual(a, c), reflect.DeepEqual(&a, &b))
	fmt.Println(reflect.DeepEqual([]any{a, 1}, []any{b, 1}))
}
