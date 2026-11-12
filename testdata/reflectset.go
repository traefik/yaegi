// The checks of the reflect setters: a Set* on a value
// that cannot be set panics with the message of gc, and recover gets that
// message. Before the fix, the VM reflect panicked with a value that recover
// saw as nil, and a field of a native struct given to reflect.ValueOf could
// be set.
package main

import (
	"fmt"
	"net/url"
	"reflect"
	"runtime"
	"strings"
)

type T struct {
	Name string
	n    int
	M    map[string]int
}

func try(name string, f func()) {
	defer func() {
		r := recover()
		_, isRT := r.(runtime.Error)
		fmt.Printf("%s -> %v (runtime.Error: %v)\n", name, r, isRT)
	}()
	f()
}

func main() {
	// Values that cannot be set: CanSet is false and every setter panics.
	c := T{Name: "orig"}
	v := reflect.ValueOf(c).Field(0)
	fmt.Println(v.CanSet(), v.CanAddr())
	try("SetString", func() { v.SetString("x") })
	fmt.Println(c.Name)
	try("SetInt", func() { reflect.ValueOf(3).SetInt(4) })
	try("Set", func() { reflect.ValueOf(3).Set(reflect.ValueOf(4)) })
	try("SetBool", func() { reflect.ValueOf(true).SetBool(false) })
	try("SetFloat", func() { reflect.ValueOf(1.5).SetFloat(2) })
	try("SetUint", func() { reflect.ValueOf(uint(1)).SetUint(2) })
	try("SetLen", func() { reflect.ValueOf([]int{1, 2}).SetLen(1) })
	try("SetBytes", func() { reflect.ValueOf([]byte{1}).SetBytes(nil) })
	try("SetZero", func() { reflect.ValueOf(3).SetZero() })
	try("Grow", func() { reflect.ValueOf([]int{}).Grow(3) })
	try("Addr", func() { reflect.ValueOf(3).Addr() })
	try("Zero", func() { reflect.Zero(reflect.TypeOf(0)).SetInt(1) })
	try("zero Value", func() { reflect.Value{}.SetInt(1) })

	// Through a pointer: settable, except the unexported fields.
	p := &T{Name: "a"}
	e := reflect.ValueOf(p).Elem()
	fmt.Println(e.CanSet(), e.CanAddr(), e.Field(0).CanSet(), e.Field(1).CanSet(), e.Field(1).CanAddr())
	try("unexported SetInt", func() { e.Field(1).SetInt(3) })
	try("unexported Set", func() { e.Field(1).Set(reflect.ValueOf(3)) })
	try("exported SetString", func() { e.Field(0).SetString("b") })
	fmt.Println(p.Name, p.n)
	fmt.Println(e.Field(1).Addr().CanSet())
	try("Set from unexported", func() {
		var x int
		reflect.ValueOf(&x).Elem().Set(reflect.ValueOf(T{n: 3}).Field(1))
	})
	try("New", func() { reflect.New(reflect.TypeOf(0)).Elem().SetInt(1) })

	// Elem of an interface, map values, slice and array elements.
	var i any = 5
	iv := reflect.ValueOf(&i).Elem()
	fmt.Println(iv.CanSet(), iv.Elem().CanSet(), iv.Elem().CanAddr())
	try("interface Elem", func() { iv.Elem().SetInt(9) })
	fmt.Println(i)
	m := map[string]int{"a": 1}
	mv := reflect.ValueOf(m).MapIndex(reflect.ValueOf("a"))
	fmt.Println(mv.CanSet(), mv.CanAddr())
	try("map value", func() { mv.SetInt(9) })
	sv := reflect.ValueOf([]int{1, 2}).Index(0)
	fmt.Println(sv.CanSet(), sv.CanAddr())
	try("slice element", func() { sv.SetInt(9) })
	av := reflect.ValueOf([2]int{1, 2}).Index(0)
	fmt.Println(av.CanSet(), av.CanAddr())
	try("array element", func() { av.SetInt(9) })

	// SetMapIndex: the map need not be addressable, but it may not come
	// from an unexported field, nor the element.
	try("SetMapIndex nil map", func() {
		reflect.ValueOf(map[string]int(nil)).SetMapIndex(reflect.ValueOf("a"), reflect.ValueOf(1))
	})
	try("SetMapIndex", func() { reflect.ValueOf(m).SetMapIndex(reflect.ValueOf("b"), reflect.ValueOf(2)) })
	fmt.Println(len(m))
	q := &T{M: map[string]int{}}
	try("SetMapIndex unexported elem", func() {
		reflect.ValueOf(q).Elem().Field(2).SetMapIndex(reflect.ValueOf("k"), reflect.ValueOf(T{n: 1}).Field(1))
	})
	fmt.Println(len(q.M))
	try("Elem of unexported pointer", func() {
		type U struct{ p *int }
		x := 1
		reflect.ValueOf(U{&x}).Field(0).Elem().SetInt(2)
	})

	// Native (host) structs.
	h := reflect.ValueOf(url.URL{})
	fmt.Println(h.CanAddr(), h.Field(0).CanSet(), h.Field(0).CanAddr())
	try("host field", func() { h.Field(0).SetString("x") })
	u := &url.URL{}
	try("host field through a pointer", func() { reflect.ValueOf(u).Elem().Field(0).SetString("http") })
	fmt.Println(u.Scheme)
	var hi any = url.URL{Host: "h"}
	try("host value in an interface", func() { reflect.ValueOf(&hi).Elem().Elem().Field(0).SetString("x") })
	try("host unexported field", func() {
		var b strings.Builder
		reflect.ValueOf(&b).Elem().Field(1).SetLen(0)
	})

	// Other panics of the VM reflect: recover gets the message.
	try("Elem of int", func() { reflect.ValueOf(1).Elem() })
	try("Type of zero Value", func() { reflect.Value{}.Type() })
	try("Interface of zero Value", func() { reflect.Value{}.Interface() })
	try("Interface of unexported", func() { reflect.ValueOf(T{n: 1}).Field(1).Interface() })
	try("Type.Elem of int", func() { reflect.TypeOf(1).Elem() })
	try("Implements nil", func() { reflect.TypeOf(1).Implements(nil) })
	try("Implements int", func() { reflect.TypeOf(1).Implements(reflect.TypeOf(1)) })
}
