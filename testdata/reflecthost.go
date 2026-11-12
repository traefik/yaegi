package main

// reflect on host objects (native values) in a program that imports reflect:
// reads and writes go to the host object, and native reflect keeps its
// checks (Set of an unexported field panics). Run in the default mode and
// with -restricted (the import restrictions of plugins) by TestTestdata.

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panics:", r)
		}
	}()
	f()
}

type holder struct {
	Req  *http.Request
	req  *http.Request
	Hdr  http.Header
	Data []int
}

func main() {
	r, _ := http.NewRequest("GET", "/x", nil)

	// Elem, NumField, FieldByName of a host pointer.
	v := reflect.ValueOf(r).Elem()
	fmt.Println(v.Kind(), v.Type(), v.NumField() > 10, v.Type() == reflect.TypeOf(http.Request{}))
	fmt.Println(v.FieldByName("Method").String(), v.FieldByName("Method").CanSet())

	// Set of an exported field reaches the host object.
	v.FieldByName("Method").SetString("POST")
	reflect.ValueOf(r).Elem().FieldByName("Host").Set(reflect.ValueOf("example.com"))
	fmt.Println(r.Method, r.Host)

	// FieldByIndex of a host struct: the right field, for reads and writes.
	for i := 0; i < 4; i++ {
		fmt.Println(v.FieldByIndex([]int{i}).Type(), v.Type().FieldByIndex([]int{i}).Name)
	}
	f, err := v.FieldByIndexErr([]int{2})
	fmt.Println(f.String(), err)
	v.FieldByIndex([]int{2}).SetString("HTTP/1.0")
	fmt.Println(r.Proto, r.Method)

	// Set of an unexported field of a host object panics.
	ctx := v.FieldByName("ctx")
	fmt.Println(ctx.CanSet(), ctx.CanInterface())
	try("unexported Set", func() { ctx.Set(reflect.Zero(ctx.Type())) })

	// The error of fmt.Errorf and a *bytes.Buffer.
	var e error = fmt.Errorf("boom %d", 1)
	ev := reflect.ValueOf(e).Elem()
	fmt.Println(ev.Kind(), ev.NumField(), ev.Field(0))
	b := &bytes.Buffer{}
	bv := reflect.ValueOf(b).Elem()
	fmt.Println(bv.NumField(), bv.Type(), bv.Field(0).CanSet())
	try("unexported SetBytes", func() { bv.Field(0).SetBytes([]byte("x")) })
	fmt.Println(b.Len())

	// Host objects in fields of an interpreted struct.
	h := &holder{Req: r, req: r, Hdr: http.Header{"A": {"1"}, "B": {"2"}}, Data: []int{1, 2}}
	hv := reflect.ValueOf(h).Elem()
	hv.Field(0).Elem().FieldByName("Method").SetString("PUT")
	fmt.Println(r.Method)
	m := hv.Field(1).Elem().FieldByName("Method")
	fmt.Println(m.String(), m.CanSet())
	try("Set through an unexported field", func() { m.SetString("DELETE") })
	fmt.Println(r.Method)
	var keys []string
	it := hv.Field(2).MapRange()
	for it.Next() {
		keys = append(keys, it.Key().String()+"="+it.Value().Index(0).String())
	}
	sort.Strings(keys)
	fmt.Println(keys)

	// Pointer of native values.
	s := []int{1, 2, 3}
	fmt.Println(reflect.ValueOf(s).Pointer() != 0, reflect.ValueOf(r).Pointer() != 0)
	hdr := r.Header
	fmt.Println(reflect.ValueOf(hdr).Pointer() == reflect.ValueOf(r.Header).Pointer())
	b.WriteString("hello")
	fmt.Println(bv.Field(0).Pointer() != 0, bv.Field(0).Len())

	// Pointer of an empty slice is 0 only for a nil slice.
	var nilS []int
	fmt.Println(reflect.ValueOf(make([]int, 0)).Pointer() != 0, reflect.ValueOf(nilS).Pointer() == 0)
	fmt.Println(reflect.ValueOf(strings.Split("a,b", ",")[:0:0]).Pointer() != 0)
	fmt.Println(reflect.ValueOf(r.Header["Nope"]).Pointer() == 0, hv.Field(3).Slice3(0, 0, 0).Pointer() != 0)
}
