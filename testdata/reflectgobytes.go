// The reflect of the program on a Go []byte (bytes.go): Index, Elem of a
// *byte, Addr, the setters and reflect.Swapper write the bytes, as in gc.
package main

import (
	"fmt"
	"reflect"
	"sort"
)

func main() {
	b := []byte("hello")
	v := reflect.ValueOf(b)
	e := v.Index(1)
	fmt.Println(e.Kind(), e.Uint(), e.CanSet(), e.CanAddr())
	e.SetUint(7)
	fmt.Println(b)
	v.Index(0).Set(reflect.ValueOf(byte(9)))
	fmt.Println(b)
	p := v.Index(2).Addr().Interface().(*byte)
	*p = 42
	fmt.Println(b)
	sw := reflect.Swapper(b)
	sw(0, 4)
	fmt.Println(b)
	s := []byte("dcba")
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	fmt.Println(string(s))
	rp := reflect.ValueOf(&b).Elem().Index(3)
	rp.SetUint(1)
	fmt.Println(b, reflect.ValueOf(b).Index(3).Interface())

	// copies of the value: a package variable, a struct field, an
	// interface, a parameter
	last = v.Index(4)
	h := holder{v: v.Index(0)}
	var x any = v.Index(1)
	last.SetUint('!')
	h.v.SetUint('H')
	set(x.(reflect.Value), 'E')
	fmt.Println(string(b), last.Uint(), h.v.Uint(), x.(reflect.Value).Uint())
}

var last reflect.Value

type holder struct{ v reflect.Value }

func set(v reflect.Value, c byte) { v.SetUint(uint64(c)) }
