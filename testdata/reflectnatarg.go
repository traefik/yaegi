package main

// A program that imports reflect passes its reflect.Type to a native
// function, which takes a real reflect.Type, and gets a real reflect.Value
// back (testing/quick.Value).

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing/quick"
)

type point struct{ X, Y int8 }

func main() {
	r := rand.New(rand.NewSource(1))
	v, ok := quick.Value(reflect.TypeOf(0), r)
	fmt.Println(ok, v.Kind(), v.Type(), v.Type() == reflect.TypeOf(0))
	s, ok := quick.Value(reflect.TypeOf(""), r)
	fmt.Println(ok, s.Kind(), s.Len() >= 0)
	p, ok := quick.Value(reflect.TypeOf([]int8{}), r)
	fmt.Println(ok, p.Kind(), p.Type())
	n, _ := v.Interface().(int)
	fmt.Println(n == v.Interface())
	pt, ok := quick.Value(reflect.TypeOf(point{}), r)
	fmt.Println(ok, pt.Kind(), pt.NumField())
}
