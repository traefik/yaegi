// reflect writes and comparisons on program types: Value.Set and the typed
// setters on addressable values (Elem of a pointer, slice elements, map
// SetMapIndex), CanSet/CanAddr, the panics for unaddressable or unexported
// values recovered, reflect.New, Append, MakeSlice, MakeMap and DeepEqual.
package main

import (
	"fmt"
	"reflect"
	"sort"
)

type Config struct {
	Host    string
	Port    int
	Debug   bool
	Ratio   float64
	Limit   uint16
	Tags    []string
	Env     map[string]string
	Sub     *Config
	private int
}

type Node struct {
	V    int
	Kids []*Node
}

func try(name string, f func()) {
	defer func() {
		if e := recover(); e != nil {
			fmt.Printf("%s: panic: %v\n", name, e)
		}
	}()
	f()
	fmt.Printf("%s: ok\n", name)
}

func main() {
	var c Config
	v := reflect.ValueOf(&c).Elem()
	fmt.Println(v.CanSet(), v.CanAddr(), v.Field(0).CanSet(), v.Field(8).CanSet(), v.Field(8).CanAddr())
	v.FieldByName("Host").SetString("localhost")
	v.Field(1).SetInt(8080)
	v.Field(2).SetBool(true)
	v.Field(3).SetFloat(0.25)
	v.Field(4).SetUint(65535)
	v.Field(5).Set(reflect.ValueOf([]string{"a", "b"}))
	v.Field(6).Set(reflect.MakeMap(v.Field(6).Type()))
	v.Field(6).SetMapIndex(reflect.ValueOf("K"), reflect.ValueOf("V"))
	v.Field(7).Set(reflect.New(v.Type()))
	v.Field(7).Elem().Field(0).SetString("sub")
	cc := c
	cc.Sub = nil // no address in the output
	fmt.Printf("%+v\n", cc)
	fmt.Println(c.Sub.Host, c.Env)

	// Overflow checks and kind checks.
	fmt.Println(v.Field(4).OverflowUint(70000), v.Field(1).OverflowInt(1<<20))

	try("set on non-addressable", func() { reflect.ValueOf(c).Field(0).SetString("x") })
	try("set unexported", func() { v.Field(8).SetInt(1) })
	try("wrong type Set", func() { v.Field(1).Set(reflect.ValueOf("s")) })
	try("Interface of unexported", func() { _ = v.Field(8).Interface() })
	try("good set", func() { v.Field(1).Set(reflect.ValueOf(9)) })
	fmt.Println(c.Port)

	// Slices: Index is addressable, Append returns a new value.
	s := []int{1, 2, 3}
	sv := reflect.ValueOf(s)
	sv.Index(1).SetInt(20)
	fmt.Println(s, sv.Index(0).CanSet())
	sv = reflect.Append(sv, reflect.ValueOf(4), reflect.ValueOf(5))
	fmt.Println(sv.Len(), sv.Interface(), s)
	ms := reflect.MakeSlice(reflect.TypeOf([]string{}), 2, 5)
	ms.Index(0).SetString("x")
	fmt.Println(ms.Len(), ms.Cap(), ms.Interface())
	arr := [3]int{}
	av := reflect.ValueOf(&arr).Elem()
	av.Index(2).SetInt(7)
	fmt.Println(arr, reflect.ValueOf(arr).Index(0).CanSet())

	// Maps.
	m := map[string]int{"a": 1}
	mv := reflect.ValueOf(m)
	mv.SetMapIndex(reflect.ValueOf("b"), reflect.ValueOf(2))
	mv.SetMapIndex(reflect.ValueOf("a"), reflect.Value{}) // delete
	keys := mv.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	fmt.Println(m, len(keys), keys[0], mv.MapIndex(reflect.ValueOf("b")), mv.MapIndex(reflect.ValueOf("z")).IsValid())

	// reflect.New and Indirect.
	pn := reflect.New(reflect.TypeOf(Node{}))
	pn.Elem().Field(0).SetInt(3)
	n := pn.Interface().(*Node)
	fmt.Println(n.V, reflect.Indirect(pn).Field(0).Int(), reflect.Indirect(reflect.ValueOf(5)).Int())

	// Swapping through reflect.
	x, y := 1, 2
	xv, yv := reflect.ValueOf(&x).Elem(), reflect.ValueOf(&y).Elem()
	tmp := xv.Interface()
	xv.Set(yv)
	yv.Set(reflect.ValueOf(tmp))
	fmt.Println(x, y)

	// DeepEqual on program types.
	a := &Node{1, []*Node{{2, nil}, {3, []*Node{{4, nil}}}}}
	b := &Node{1, []*Node{{2, nil}, {3, []*Node{{4, nil}}}}}
	fmt.Println(reflect.DeepEqual(a, b), a == b)
	b.Kids[1].Kids[0].V = 5
	fmt.Println(reflect.DeepEqual(a, b))
	fmt.Println(reflect.DeepEqual(Config{Tags: []string{}}, Config{}), reflect.DeepEqual(Config{}, Config{}))
	fmt.Println(reflect.DeepEqual(map[string][]int{"a": {1}}, map[string][]int{"a": {1}}))
	fmt.Println(reflect.DeepEqual([]any{1, "a"}, []any{1, "a"}), reflect.DeepEqual(any(1), any(int64(1))))
	fmt.Println(reflect.DeepEqual(nil, nil), reflect.DeepEqual((*Node)(nil), nil), reflect.DeepEqual(c, c))
	var f1 func()
	fmt.Println(reflect.DeepEqual(f1, f1), reflect.DeepEqual(func() {}, func() {}))
	cyc1 := &Node{V: 1}
	cyc1.Kids = []*Node{cyc1}
	cyc2 := &Node{V: 1}
	cyc2.Kids = []*Node{cyc2}
	fmt.Println(reflect.DeepEqual(cyc1, cyc2))
}
