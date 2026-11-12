package main

// reflect called by the interpreted program on its own types, in the
// default mode: the methods of interpreted types, reflect.MakeFunc, native
// values of unexported types, and reflect values printed by native fmt.
// Native reflect cannot see interpreted methods, so these calls go to the
// VM reflect (GOROOT/test reflectmethod*.go, issue27695*.go, bug461.go).

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type M int

func (m M) String() string        { return fmt.Sprintf("M(%d)", int(m)) }
func (m M) Hello(s string) string { return "hello " + s + " from " + m.String() }

type T struct{ n int }

func (t *T) Inc(d int) int { t.n += d; return t.n }

type S struct {
	A int `json:"a"`
	B string
	c bool
}

var hidden struct {
	int
}

func main() {
	// Methods of interpreted types.
	t := reflect.TypeOf(M(1))
	fmt.Println(t.NumMethod(), t.Method(0).Name, t.Method(1).Name, t.Name(), t.String())
	fmt.Println(t.Method(0).Func.Interface().(func(M, string) string)(M(2), "a"))
	m, ok := t.MethodByName("Hello")
	fmt.Println(ok, m.Func.Call([]reflect.Value{reflect.ValueOf(M(3)), reflect.ValueOf("b")})[0])
	h := reflect.Type.MethodByName
	_, ok = h(t, "Nope")
	fmt.Println(ok)
	f := reflect.ValueOf(M(4)).MethodByName("Hello").Interface().(func(string) string)
	fmt.Println(f("c"))
	p := &T{}
	inc := reflect.ValueOf(p).Method(0).Interface().(func(int) int)
	inc(2)
	fmt.Println(inc(3), p.n)

	// reflect.MakeFunc.
	double := reflect.MakeFunc(reflect.TypeOf(func(int) int { return 0 }), func(a []reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(int(a[0].Int() * 2))}
	}).Interface().(func(int) int)
	fmt.Println(double(21))

	// Struct fields: offsets, unexported embedded field.
	st := reflect.TypeOf(S{})
	for i := 0; i < st.NumField(); i++ {
		sf := st.Field(i)
		fmt.Println(sf.Name, sf.Type, sf.Tag.Get("json"), sf.PkgPath, sf.Index, sf.Offset, sf.IsExported())
	}
	fmt.Println(st.Size(), reflect.ValueOf(&hidden).Elem().Field(0).CanSet())

	// Native values whose type is not known to the program.
	e := errors.New("boom")
	et := reflect.TypeOf(e)
	fmt.Println(et, et.Kind(), et.Elem().Kind(), et.Implements(reflect.TypeOf((*error)(nil)).Elem()))
	fmt.Println(reflect.ValueOf(e).MethodByName("Error").Call(nil)[0])
	sb := &strings.Builder{}
	reflect.ValueOf(sb).MethodByName("WriteString").Call([]reflect.Value{reflect.ValueOf("zz")})
	fmt.Println(sb.String())

	// reflect values printed by native fmt.
	fmt.Println(reflect.ValueOf(3), reflect.ValueOf(M(5)), reflect.ValueOf(S{1, "x", true}), reflect.Value{})
	fmt.Printf("%v %d %s %v\n", reflect.ValueOf(4), reflect.ValueOf(5), reflect.ValueOf("s"), reflect.Zero(st))
	fmt.Println(reflect.TypeOf(1), reflect.TypeOf(M(1)), reflect.TypeOf(S{}))
}
