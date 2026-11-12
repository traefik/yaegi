// complex values inside composites (struct fields, slices, arrays, maps,
// interfaces) with fmt (%v, %+v, %#v) and reflect (Complex, SetComplex), in
// default mode where fmt goes to its AOT copy for interpreted types.
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type S struct {
	C  complex128
	D  complex64
	Cs []complex128
	M  map[string]complex64
	A  [2]complex128
	I  any
}

func main() {
	c := complex(1, 2)
	s := S{C: 3 + 4i, D: 5 - 6i, Cs: []complex128{1i, 2}, M: map[string]complex64{"a": 7i}, A: [2]complex128{8, 9i}, I: complex64(1 + 1i)}
	fmt.Printf("%+v\n", s)
	fmt.Printf("%v %v %v\n", s.Cs, s.M, s.A)
	fmt.Println(s.C, c, s.I)
	fmt.Printf("%#v\n", s.Cs)
	v := reflect.ValueOf(s)
	fmt.Println(v.Field(0).Complex(), v.Field(1).Complex(), v.Field(2).Index(0).Complex())
	fmt.Println(fmt.Sprint([]any{c, s.D}))
	_, err := json.Marshal(s)
	fmt.Println(err)
	ch := make(chan complex128, 1)
	ch <- c
	fmt.Println(<-ch)
	var arr [3]complex64
	arr[1] = 2i
	fmt.Println(arr, real(arr[1]), imag(arr[1]))
	type T struct{ X, Y complex128 }
	m := map[T]int{{1, 2i}: 3}
	fmt.Println(m)
	rv := reflect.ValueOf(&s).Elem()
	rv.Field(0).SetComplex(1.5i)
	rv.Field(1).SetComplex(complex(1.0/3, 0))
	rv.Field(2).Index(1).SetComplex(-2)
	fmt.Printf("%+v %v\n", s, rv.Field(1).Complex() == complex128(complex64(complex(1.0/3, 0))))
	fmt.Printf("%v %.2f %+v\n", []any{s.A, s.M}, s.C, struct{ Z complex64 }{3})
}
