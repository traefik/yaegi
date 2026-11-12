// Type assertions: comma-ok and panicking forms to concrete types, to
// interfaces, on nil interfaces, with named and unnamed types, and the
// exact messages of the recovered runtime errors.
package main

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
)

type MyInt int
type Stringer interface{ String() string }

func (m MyInt) String() string { return fmt.Sprintf("MyInt(%d)", int(m)) }

type Point struct{ X, Y int }

func try(name string, f func()) {
	defer func() {
		e := recover()
		if e == nil {
			return
		}
		_, isRT := e.(runtime.Error)
		fmt.Printf("%s: %v (runtime.Error=%v)\n", name, e, isRT)
	}()
	f()
}

func main() {
	var a any = 42
	n, ok := a.(int)
	fmt.Println(n, ok)
	s, ok := a.(string)
	fmt.Printf("%q %v\n", s, ok)
	m, ok := a.(MyInt)
	fmt.Println(m, ok)
	_, ok = a.(Stringer)
	fmt.Println(ok)

	a = MyInt(7)
	st, ok := a.(Stringer)
	fmt.Println(st, ok, st.String())
	fs, ok := a.(fmt.Stringer)
	fmt.Println(fs, ok)
	i, ok := a.(int)
	fmt.Println(i, ok)

	a = Point{1, 2}
	p := a.(Point)
	fmt.Println(p.X, p.Y)
	pp, ok := a.(*Point)
	fmt.Println(pp == nil, ok)

	var nilA any
	_, ok = nilA.(int)
	fmt.Println(ok)
	_, ok = nilA.(any)
	fmt.Println(ok)

	try("int to string", func() { _ = any(1).(string) })
	try("string to int", func() { _ = any("x").(int) })
	try("named", func() { _ = any(MyInt(1)).(int) })
	try("to named", func() { _ = any(1).(MyInt) })
	try("struct", func() { _ = any(Point{}).(*Point) })
	try("nil to int", func() { _ = nilA.(int) })
	try("nil to iface", func() { _ = nilA.(Stringer) })
	try("missing method", func() { _ = any(1).(Stringer) })
	try("missing method fmt", func() { _ = any(Point{}).(fmt.Stringer) })
	try("io.Reader", func() { _ = any(strings.NewReader("")).(io.Writer) })
	try("slice", func() { _ = any([]int{}).([]string) })
	try("map", func() { _ = any(map[string]int{}).(map[string]any) })
	try("func", func() { _ = any(func() {}).(func(int)) })
	try("error to Stringer", func() {
		var e error = errors.New("x")
		_ = e.(fmt.Stringer)
	})
	try("Stringer to concrete", func() {
		var s fmt.Stringer = MyInt(1)
		_ = s.(*MyInt)
	})
	try("anon struct", func() { _ = any(struct{ A int }{}).(struct{ B int }) })
	try("ok form", func() {
		v, ok := any(1).(string)
		fmt.Printf("ok form did not panic: %q %v\n", v, ok)
	})

	// Assertion to the same interface and between interfaces.
	var r io.Reader = strings.NewReader("abc")
	rs, ok := r.(io.ReadSeeker)
	fmt.Println(ok, rs != nil)
	_, ok = r.(io.Writer)
	fmt.Println(ok)
	buf := make([]byte, 2)
	nn, _ := r.(io.Reader).Read(buf)
	fmt.Println(nn, string(buf))
}
