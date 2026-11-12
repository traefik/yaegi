// A write through reflect into &s[i] of a []U, U a byte type of the program
// with methods, reaches the slice: the twin value of the byte keeps its
// address when it goes to the reflect of the program.
package main

import (
	"fmt"
	"reflect"
)

type P byte

func (p *P) Set(x int) { *p = P(x) }

type V byte

func (v V) String() string { return fmt.Sprintf("V%d", byte(v)) }

func main() {
	s := []P{1, 2, 3}
	e := reflect.ValueOf(&s[0]).Elem()
	fmt.Println(e.CanSet(), e.CanAddr(), e.Addr().Interface() == any(&s[0]))
	e.SetUint(9)
	fmt.Println(s, e.Uint())
	w := reflect.ValueOf(s).Index(1).Addr()
	w.Elem().SetUint(8)
	w.Elem().Set(reflect.ValueOf(P(7)))
	fmt.Println(s)
	s[2] = 5
	fmt.Println(reflect.ValueOf(&s[2]).Elem().Uint())

	vs := []V{1, 2, 3}
	a := reflect.ValueOf(vs).Index(2).Addr()
	a.Elem().SetUint(6)
	_, ok := a.Interface().(*V)
	fmt.Println(vs, ok, a.NumMethod())
}
