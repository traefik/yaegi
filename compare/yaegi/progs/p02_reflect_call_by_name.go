package main

import (
	"fmt"
	"reflect"
	"strings"
)

type Handlers struct{ prefix string }

func (h Handlers) Upper(s string) string    { return h.prefix + strings.ToUpper(s) }
func (h Handlers) Lower(s string) string    { return h.prefix + strings.ToLower(s) }
func (h *Handlers) SetPrefix(p string)      { h.prefix = p }
func (h Handlers) Sum(a, b int) (int, error) { return a + b, nil }

func main() {
	h := &Handlers{">"}
	v := reflect.ValueOf(h)
	fmt.Println(v.NumMethod(), v.Elem().NumMethod())
	for i := 0; i < v.Type().NumMethod(); i++ {
		fmt.Print(v.Type().Method(i).Name, " ")
	}
	fmt.Println()
	v.MethodByName("SetPrefix").Call([]reflect.Value{reflect.ValueOf("#")})
	out := v.MethodByName("Upper").Call([]reflect.Value{reflect.ValueOf("abc")})
	fmt.Println(out[0].String())
	r := v.MethodByName("Sum").Call([]reflect.Value{reflect.ValueOf(2), reflect.ValueOf(3)})
	fmt.Println(r[0].Int(), r[1].IsNil())
	m, ok := reflect.TypeOf(h).MethodByName("Lower")
	fmt.Println(ok, m.Func.Call([]reflect.Value{v, reflect.ValueOf("XY")})[0])
}
