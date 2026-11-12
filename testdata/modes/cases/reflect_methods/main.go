// reflect: NumMethod, Method, MethodByName, Call, Type.Implements,
// AssignableTo, ConvertibleTo, Comparable, Name and String of
// interpreted types.
package main

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

type Greeter interface {
	Greet(string) string
}

type Bot struct {
	Name string
	n    int
}

func (b Bot) Greet(who string) string { return b.Name + " greets " + who }
func (b Bot) Sum(xs ...int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}
func (b *Bot) Rename(n string)            { b.Name = n }
func (b *Bot) Read(p []byte) (int, error) { return copy(p, b.Name), io.EOF }
func (b Bot) private()                    {}
func (b Bot) Pair() (string, int)         { return b.Name, len(b.Name) }

type MyInt int

func (m MyInt) Double() MyInt { return m * 2 }

type Alias = Bot

func main() {
	t := reflect.TypeOf(Bot{})
	pt := reflect.TypeOf(&Bot{})
	fmt.Println(t.Name(), t.String(), t.Kind(), t.PkgPath(), pt.String(), pt.Elem() == t)
	fmt.Println(t.NumMethod(), pt.NumMethod())
	for i := 0; i < pt.NumMethod(); i++ {
		m := pt.Method(i)
		fmt.Println(i, m.Name, m.Type, m.Index)
	}
	gi := reflect.TypeOf((*Greeter)(nil)).Elem()
	ri := reflect.TypeOf((*io.Reader)(nil)).Elem()
	fmt.Println(t.Implements(gi), pt.Implements(gi), t.Implements(ri), pt.Implements(ri))
	fmt.Println(t.AssignableTo(gi), reflect.TypeOf(MyInt(0)).ConvertibleTo(reflect.TypeOf(0)), reflect.TypeOf(Alias{}) == t, t.Comparable())

	b := &Bot{Name: "R2"}
	v := reflect.ValueOf(b)
	out := v.MethodByName("Greet").Call([]reflect.Value{reflect.ValueOf("you")})
	fmt.Println(out[0].String())
	out = v.MethodByName("Sum").Call([]reflect.Value{reflect.ValueOf(1), reflect.ValueOf(2), reflect.ValueOf(3)})
	fmt.Println(out[0].Int())
	v.MethodByName("Rename").Call([]reflect.Value{reflect.ValueOf("C3")})
	fmt.Println(b.Name)
	out = v.MethodByName("Pair").Call(nil)
	fmt.Println(out[0], out[1])
	m, ok := pt.MethodByName("Rename")
	fmt.Println(ok, m.Type.NumIn(), m.Type.In(0) == pt)
	m.Func.Call([]reflect.Value{v, reflect.ValueOf("D4")})
	fmt.Println(b.Name)
	_, ok = t.MethodByName("private")
	fmt.Println(ok, v.MethodByName("Nope").IsValid())

	f := v.Elem().Method(0).Interface().(func(string) string)
	fmt.Println(f("closure"))
	mv := reflect.ValueOf(MyInt(21)).MethodByName("Double").Call(nil)[0]
	fmt.Println(mv.Interface(), mv.Type())

	var g Greeter = Bot{Name: "via iface"}
	gv := reflect.ValueOf(&g).Elem()
	fmt.Println(gv.Kind(), gv.Elem().Type(), gv.NumMethod(), gv.Method(0).Call([]reflect.Value{reflect.ValueOf("x")})[0])

	sf, _ := t.FieldByName("n")
	fmt.Println(sf.Name, sf.IsExported(), sf.Type, t.NumField(), reflect.ValueOf(Bot{n: 5}).Field(1).Int())
	fmt.Println(strings.Contains(fmt.Sprint(reflect.TypeOf(Bot.Greet)), "main.Bot"))
}
