// reflect checks the kind, the types and the bounds before it acts, and
// panics with its own messages on misuse.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type T struct {
	Name  string
	Count int
}

type C struct{ S string }

func (C) Add(a, b int) int { return a + b }

func add(a, b int) int { return a + b }

func try(name string, f func()) {
	defer func() { fmt.Println(name, "recovered:", recover()) }()
	f()
}

func main() {
	var o T
	v := reflect.ValueOf(&o).Elem()
	try("a1", func() { v.Field(1).SetString("x") })
	fmt.Println(o.Count)
	try("a2", func() { reflect.ValueOf(&o.Count).Elem().SetString("x") })
	try("a3", func() { n := 0; reflect.ValueOf(&n).Elem().SetString("x") })
	try("a4", func() { v.Field(0).SetInt(1) })

	try("b1", func() {
		fmt.Println(reflect.ValueOf(add).Call([]reflect.Value{reflect.ValueOf("x"), reflect.ValueOf(1)}))
	})
	try("b2", func() {
		fmt.Println(reflect.ValueOf(C{}).MethodByName("Add").Call([]reflect.Value{reflect.ValueOf("x"), reflect.ValueOf(1)}))
	})
	try("b3", func() {
		fmt.Println(reflect.ValueOf(fmt.Sprint).Call([]reflect.Value{reflect.ValueOf(1)}))
		fmt.Println(reflect.ValueOf(fmt.Sprintf).Call([]reflect.Value{reflect.ValueOf(1)}))
	})
	try("b4", func() {
		bad := reflect.MakeFunc(reflect.TypeOf(func() int { return 0 }), func([]reflect.Value) []reflect.Value {
			return []reflect.Value{reflect.ValueOf("str")}
		})
		fmt.Println(bad.Call(nil))
	})
	try("b5", func() {
		var f func() int
		reflect.ValueOf(&f).Elem().Set(reflect.MakeFunc(reflect.TypeOf(f), func([]reflect.Value) []reflect.Value {
			return []reflect.Value{reflect.ValueOf("str")}
		}))
		fmt.Println(f())
	})

	try("c1", func() {
		m := reflect.MakeMap(reflect.TypeOf(0))
		fmt.Println("after MakeMap", m.Kind())
	})
	try("c2", func() {
		a := reflect.ValueOf([]int{1}).Convert(reflect.TypeOf([3]int{}))
		fmt.Println("after Convert", a)
	})
	try("c3", func() {
		reflect.ValueOf(1).Len()
		fmt.Println("after Len")
	})
	try("c4", func() {
		ch := make(chan int)
		cv := reflect.ValueOf(ch)
		cv.Close()
		cv.Close()
	})

	try("d1", func() { reflect.ValueOf((*C)(nil)).Elem().Field(0) })
	try("d2", func() { reflect.ValueOf(1).NumField() })
	try("d3", func() { reflect.ValueOf([]int{1}).Index(3) })
	try("d4", func() { reflect.ValueOf([2]int{}).Index(2) })
	try("d5", func() { reflect.ValueOf("ab").Index(2) })

	// correct uses still work
	try("e1", func() {
		fmt.Println(reflect.ValueOf(strings.Repeat).Call([]reflect.Value{reflect.ValueOf("ab"), reflect.ValueOf(2)})[0])
		fmt.Println(reflect.ValueOf(add).Call([]reflect.Value{reflect.ValueOf(1), reflect.ValueOf(2)})[0])
		fmt.Println(reflect.ValueOf(fmt.Sprintf).Call([]reflect.Value{reflect.ValueOf("%d-%s"), reflect.ValueOf(1), reflect.ValueOf("x")})[0])
		fmt.Println(reflect.ValueOf(name).Call([]reflect.Value{reflect.ValueOf(myStr("p"))})[0])
		fmt.Println(reflect.ValueOf(join).Call([]reflect.Value{reflect.ValueOf(myStr("a")), reflect.ValueOf(myStr("b"))})[0])
		fmt.Println(reflect.ValueOf(join).CallSlice([]reflect.Value{reflect.ValueOf([]myStr{"c", "d"})})[0])
	})
	try("e2", func() {
		f := reflect.MakeFunc(reflect.TypeOf(func(int) myStr { return "" }), func(in []reflect.Value) []reflect.Value {
			return []reflect.Value{reflect.ValueOf(myStr(fmt.Sprint(in[0].Int())))}
		})
		fmt.Println(f.Call([]reflect.Value{reflect.ValueOf(7)})[0], f.Interface().(func(int) myStr)(8))
		var g func() any
		reflect.ValueOf(&g).Elem().Set(reflect.MakeFunc(reflect.TypeOf(g), func([]reflect.Value) []reflect.Value {
			return []reflect.Value{reflect.ValueOf("in any")}
		}))
		fmt.Println(g())
	})
	try("e3", func() {
		fmt.Println(reflect.ValueOf(myStr("s")).Convert(reflect.TypeOf("")).Interface())
		fmt.Println(reflect.ValueOf("t").Convert(reflect.TypeOf(myStr(""))).Interface())
		fmt.Println(reflect.ValueOf([]int{1, 2, 3, 4}).Convert(reflect.TypeOf([3]int{})))
		fmt.Println(reflect.ValueOf(&[2]int{}).Len(), reflect.ValueOf([2]int{5, 6}).Index(1), reflect.ValueOf("xy").Index(1))
		var p T
		pv := reflect.ValueOf(&p).Elem()
		pv.Field(0).SetString("n")
		pv.Field(1).SetInt(3)
		fmt.Println(p, pv.NumField(), reflect.MakeMap(reflect.TypeOf(map[string]int{})).Len())
	})

	// a type of the program given where a native type is expected, and the
	// other way
	try("y1", func() { fmt.Println(reflect.ValueOf(strings.ToUpper).Call([]reflect.Value{reflect.ValueOf(C{})})) })
	try("y2", func() {
		fmt.Println(reflect.ValueOf(strings.Repeat).Call([]reflect.Value{reflect.ValueOf(myStr("a")), reflect.ValueOf(2)}))
	})
	try("y3", func() { fmt.Println(reflect.ValueOf(name).Call([]reflect.Value{reflect.ValueOf(3)})) })
	try("y4", func() { fmt.Println(reflect.ValueOf(name).Call([]reflect.Value{reflect.ValueOf("ok")})) })
	try("y5", func() { fmt.Println(reflect.ValueOf(C{}).Convert(reflect.TypeOf(""))) })
	try("y6", func() { fmt.Println(reflect.ValueOf(myStr("q")).Convert(reflect.TypeOf(0))) })
	try("y7", func() {
		fmt.Println(reflect.ValueOf(myStr("q")).Convert(reflect.TypeOf("")), reflect.ValueOf("r").Convert(reflect.TypeOf(myStr(""))))
	})
	try("y8", func() { fmt.Println(reflect.ValueOf(&o).Len()) })
	try("y9", func() { fmt.Println(reflect.ValueOf([]int{1}).Convert(reflect.TypeOf(&[2]int{}))) })
	try("y10", func() {
		fmt.Println(reflect.ValueOf(add).CallSlice([]reflect.Value{reflect.ValueOf(1), reflect.ValueOf(2)}))
	})
	try("y11", func() { fmt.Println(reflect.ValueOf(fmt.Sprint).CallSlice([]reflect.Value{})) })
	// a named type and an unnamed type with the same underlying type are
	// assignable, across the native boundary too
	try("z1", func() {
		fmt.Println(reflect.ValueOf(bytes.ToUpper).Call([]reflect.Value{reflect.ValueOf(bs("ab"))})[0])
	})
	try("z2", func() {
		fmt.Println(reflect.ValueOf(strings.Join).Call([]reflect.Value{reflect.ValueOf(strs{"a", "b"}), reflect.ValueOf("-")})[0])
	})
	try("z3", func() {
		fmt.Println(reflect.ValueOf(strings.Map).Call([]reflect.Value{reflect.ValueOf(runeFn(func(r rune) rune { return r + 1 })), reflect.ValueOf("ab")})[0])
	})
	try("z4", func() { fmt.Println(reflect.ValueOf(bsLen).Call([]reflect.Value{reflect.ValueOf([]byte("abc"))})[0]) })
	try("z5", func() {
		fmt.Println(reflect.ValueOf(bsLen).Call([]reflect.Value{reflect.ValueOf(json.RawMessage("ab"))})[0])
	})
	// a bidirectional channel is assignable to a named channel type with a
	// direction, and a named channel type to a channel with a direction
	try("z6", func() {
		fmt.Println(reflect.ValueOf(rcCap).Call([]reflect.Value{reflect.ValueOf(make(chan int, 4))})[0])
	})
	try("z7", func() {
		fmt.Println(reflect.ValueOf(rcCap).Call([]reflect.Value{reflect.MakeChan(reflect.TypeOf(make(chan int)), 3)})[0])
	})
	try("z8", func() {
		f := reflect.MakeFunc(reflect.TypeOf(func() rc { return nil }), func([]reflect.Value) []reflect.Value {
			return []reflect.Value{reflect.ValueOf(make(chan int, 2))}
		})
		fmt.Println(cap(f.Interface().(func() rc)()))
	})
	try("z9", func() {
		fmt.Println(reflect.ValueOf(sendCap).Call([]reflect.Value{reflect.ValueOf(sc(make(chan int, 5)))})[0])
	})
}

type bs []byte

type strs []string

type runeFn func(rune) rune

func bsLen(b bs) int { return len(b) }

type rc <-chan int

type sc chan int

func rcCap(c rc) int { return cap(c) }

func sendCap(c chan<- int) int { return cap(c) }

type myStr string

func name(s myStr) string { return "name:" + string(s) }

func join(s ...myStr) myStr {
	var r myStr
	for _, x := range s {
		r += x
	}
	return r
}
