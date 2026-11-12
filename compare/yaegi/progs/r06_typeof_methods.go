package main

import (
	"fmt"
	"reflect"
)

type Svc struct{ N int }

func (s Svc) Hello(name string) string { return fmt.Sprintf("hello %s %d", name, s.N) }
func (s *Svc) Inc()                    { s.N++ }
func (s Svc) priv()                    {}

func main() {
	t := reflect.TypeOf(Svc{})
	fmt.Println(t.Name(), t.PkgPath(), t.Kind(), t.NumMethod(), reflect.TypeOf(&Svc{}).NumMethod())
	fmt.Println(t.String(), reflect.TypeOf(&Svc{}).String())
	m, ok := t.MethodByName("Hello")
	fmt.Println(ok, m.Name, m.Type)
	out := reflect.ValueOf(Svc{7}).MethodByName("Hello").Call([]reflect.Value{reflect.ValueOf("bob")})
	fmt.Println(out[0])
	s := &Svc{1}
	reflect.ValueOf(s).MethodByName("Inc").Call(nil)
	fmt.Println(s.N)
	fmt.Println(t.Implements(reflect.TypeOf((*fmt.Stringer)(nil)).Elem()))
}
