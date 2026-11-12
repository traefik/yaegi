package main

// Recursive named types that are not structs: type M map[string]M,
// type T []T, type A [1]*A, type F func() F, type C chan C, type P *P.
// reflect cannot build them: in default mode their native type
// (reflectType) never ended, and the first value that crossed to native
// code (fmt, json, reflect, sort) killed the host with a fatal Go stack
// overflow, which recover, Budget and MaxNest cannot stop. The native type
// now cuts the parts that refer back to the type to any (cutNamed in
// vm/native.go), and json.Unmarshal unrolls it along the data
// (vm/recursive.go).

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type M map[string]M

type T []T

type A [1]*A

type F func() F

// a variadic func: reflect.FuncOf needs a slice for the last parameter, so
// the cut gives func(...any) any (it gave func(any) any, and reflect.FuncOf
// panicked)
type V func(...V) V

type HV struct{ F V }

type C chan C

type P *P

type N map[string][]N // the cut goes through a slice

type K []map[string]K

type MP map[string]*MP // the cut goes through a pointer

// a struct that holds a recursive named type, and a named type that holds
// a recursive struct
type Doc struct {
	Name string `json:"name"`
	Tree M      `json:"tree"`
	List T      `json:"list,omitempty"`
}

type Node struct {
	V    int
	Kids Nodes
}

type Nodes []Node

type ByLen []T

func (b ByLen) Len() int           { return len(b) }
func (b ByLen) Less(i, j int) bool { return len(b[i]) < len(b[j]) }
func (b ByLen) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func show(name string, v any) {
	b, err := json.Marshal(v)
	fmt.Printf("%s: %v | %s %v\n", name, v, b, err)
}

func decode(name, data string, v any) {
	err := json.Unmarshal([]byte(data), v)
	var te *json.UnmarshalTypeError
	b, _ := json.Marshal(v)
	fmt.Printf("%s: err=%v typeErr=%v -> %s\n", name, err != nil, errors.As(err, &te), b)
}

func main() {
	m := M{"a": M{"b": nil, "c": M{}}}
	show("M", m)
	fmt.Println(fmt.Sprint(m), len(m["a"]), m["a"]["b"] == nil)
	fmt.Printf("%v %+v %d\n", m, m["a"], len(m))

	t := T{T{}, nil, T{T{T{}}}}
	show("T", t)

	a := A{&A{nil}}
	b, err := json.Marshal(a)
	fmt.Println("A", string(b), err)
	fmt.Println(a[0] != nil, a[0][0] == nil)

	var f F
	f = func() F { return f }
	fmt.Println("F", f()() != nil)
	_, err = json.Marshal(f)
	fmt.Println("F json error:", err != nil)

	ch := make(C, 1)
	ch <- ch
	fmt.Println("C", (<-ch) == ch)

	var p P
	p = &p
	fmt.Println("P", *p == p, **p == p)

	show("N", N{"x": {N{"y": nil}, nil}})
	show("K", K{{"a": K{}}, nil})
	mp := MP{"leaf": nil}
	mp["self"] = &mp
	fmt.Println("MP", len(*mp["self"]), mp["leaf"] == nil)
	show("Doc", Doc{Name: "d", Tree: M{"x": M{}}, List: T{nil}})
	show("Nodes", Nodes{{V: 1, Kids: Nodes{{V: 2}}}})

	// json.Unmarshal: valid data, and data of the wrong kind (go run
	// reports an UnmarshalTypeError and keeps the rest).
	var m2 M
	decode("M", `{"x":{"y":{}},"z":null}`, &m2)
	fmt.Println(m2["z"] == nil, len(m2["x"]["y"]))
	m3 := M{"keep": M{}}
	decode("M merge", `{"new":{"k":{}}}`, &m3)
	var m4 M
	decode("M bad", `{"x":1,"y":{"z":"s","w":{}}}`, &m4)
	var m5 M
	decode("M deep bad", `{"a":{"b":{"c":{"d":true}}}}`, &m5)
	var t2 T
	decode("T", `[[],[[]],null]`, &t2)
	fmt.Println(len(t2), t2[2] == nil)
	var t3 T
	decode("T bad", `[[],"s",[[1]]]`, &t3)
	var a2 A
	decode("A", `[[[null]]]`, &a2)
	fmt.Println(a2[0] != nil, a2[0][0] != nil, a2[0][0][0] == nil)
	var n2 N
	decode("N", `{"x":[{"y":[]},null]}`, &n2)
	var d Doc
	decode("Doc", `{"name":"d","tree":{"a":{}},"list":[[],[[]]]}`, &d)
	fmt.Println(d.Name, len(d.Tree), len(d.List))
	var d2 Doc
	decode("Doc bad", `{"name":"d","tree":{"a":5}}`, &d2)
	var nodes Nodes
	decode("Nodes", `[{"V":1,"Kids":[{"V":2,"Kids":[{"V":3}]}]}]`, &nodes)
	fmt.Println(nodes[0].Kids[0].Kids[0].V)

	// deep data: 300 levels
	deep := ""
	for i := 0; i < 300; i++ {
		deep += `{"k":`
	}
	deep += "{}"
	for i := 0; i < 300; i++ {
		deep += "}"
	}
	var md M
	fmt.Println("M 300 levels:", json.Unmarshal([]byte(deep), &md))
	// data of the wrong kind, deeper than the native types that yaegi v2
	// default mode can build: an error, never a crash
	bad := strings.Repeat(`{"k":`, 2000) + "1" + strings.Repeat("}", 2000)
	var mb M
	fmt.Println("M 2000 levels, bad leaf:", json.Unmarshal([]byte(bad), &mb) != nil)
	depth := 0
	for x := md; x != nil; x = x["k"] {
		depth++
	}
	fmt.Println("depth", depth)

	// reflect
	fmt.Println(reflect.DeepEqual(m, M{"a": M{"b": nil, "c": M{}}}), reflect.DeepEqual(m, m2))
	fmt.Println(reflect.DeepEqual(t, T{T{}, nil, T{T{T{}}}}), reflect.DeepEqual(a, A{&A{}}))
	fmt.Println(reflect.TypeOf(m), reflect.TypeOf(t), reflect.ValueOf(t).Len(), reflect.ValueOf(m).Len())

	// sort
	s := ByLen{T{T{}, T{}}, T{}, T{T{}}}
	sort.Sort(s)
	fmt.Println(s)
	sort.Slice(t, func(i, j int) bool { return len(t[i]) > len(t[j]) })
	fmt.Println(t)

	// a variadic recursive func that crosses to native code
	var vf V
	nv := 0
	vf = func(xs ...V) V { nv += len(xs); return vf }
	_, err = json.Marshal(HV{F: vf})
	fmt.Println("V json error:", err != nil)
	vout := reflect.ValueOf(vf).Call([]reflect.Value{reflect.ValueOf(vf), reflect.ValueOf(vf)})
	fmt.Println("V", len(vout), nv)
	vr := vout[0].Interface().(V)(vf)
	fmt.Println("V", vr != nil, nv)
	reflect.ValueOf(vf).CallSlice([]reflect.Value{reflect.ValueOf([]V{vf, vf, vf})})
	fmt.Println("V", nv, reflect.TypeOf(vf).IsVariadic(), reflect.TypeOf(vf).NumIn())

	// values that contain themselves
	cm := M{}
	cm["self"] = cm
	fmt.Println(len(cm["self"]["self"]), reflect.DeepEqual(cm, cm))
	_, err = json.Marshal(cm)
	var ue *json.UnsupportedValueError
	fmt.Println("cyclic M:", errors.As(err, &ue))
	decode("cyclic M", `{"x":{}}`, &cm)
	fmt.Println(len(cm), len(cm["self"]), cm["self"]["x"] != nil)
	ct := T{nil, T{}, T{T{}, T{}}}
	ct[0] = ct
	sort.Slice(ct, func(i, j int) bool { return len(ct[i]) < len(ct[j]) })
	fmt.Println(len(ct[0]), len(ct[1]), len(ct[2]), reflect.DeepEqual(ct, ct))
	_, err = json.Marshal(ct)
	fmt.Println("cyclic T:", err != nil)
	ca := A{}
	ca[0] = &ca
	fmt.Println(ca[0][0] == &ca, reflect.DeepEqual(ca, ca))
	_, err = json.Marshal(ca)
	fmt.Println("cyclic A:", errors.As(err, &ue))
}
