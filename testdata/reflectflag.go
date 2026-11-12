// The flag field of reflect.Value read and written with unsafe, as
// github.com/davecgh/go-spew does in bypass.go (pulled in by client-go): the
// init checks the read-only and addressable bits, and the printer clears the
// read-only bits to call Interface on unexported fields.
package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

type flag uintptr

var flagValOffset = func() uintptr {
	field, ok := reflect.TypeOf(reflect.Value{}).FieldByName("flag")
	if !ok {
		panic("reflect.Value has no flag field")
	}
	return field.Offset
}()

func flagField(v *reflect.Value) *flag {
	return (*flag)(unsafe.Pointer(uintptr(unsafe.Pointer(v)) + flagValOffset))
}

var flagRO, flagAddr flag

func probe() {
	field, ok := reflect.TypeOf(reflect.Value{}).FieldByName("flag")
	fmt.Println("field:", ok, field.Name, field.Type.Kind(), field.Type.Kind() == reflect.TypeOf(flag(0)).Kind())
	type t0 int
	var t struct {
		A t0
		t0
		a t0
	}
	vA := reflect.ValueOf(t).FieldByName("A")
	va := reflect.ValueOf(t).FieldByName("a")
	vt0 := reflect.ValueOf(t).FieldByName("t0")
	flagPublic := *flagField(&vA)
	flagWithRO := *flagField(&va) | *flagField(&vt0)
	flagRO = flagPublic ^ flagWithRO
	vPtrA := reflect.ValueOf(&t).Elem().FieldByName("A")
	flagAddr = *flagField(&vA) ^ *flagField(&vPtrA)
	fmt.Println("ro:", flagRO, "addr:", flagAddr)
}

func unsafeReflectValue(v reflect.Value) reflect.Value {
	if !v.IsValid() || (v.CanInterface() && v.CanAddr()) {
		return v
	}
	p := flagField(&v)
	*p &^= flagRO
	*p |= flagAddr
	return v
}

type inner struct{ n int }

func (i inner) String() string { return fmt.Sprintf("inner(%d)", i.n) }

type outer struct {
	Name   string
	secret string
	in     inner
	list   []int
	m      map[string]int
}

func main() {
	probe()
	o := outer{Name: "x", secret: "hidden", in: inner{7}, list: []int{1, 2}, m: map[string]int{"k": 3}}
	v := reflect.ValueOf(o)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		before := f.CanInterface()
		u := unsafeReflectValue(f)
		fmt.Printf("%s: %v -> %v %v %v\n", v.Type().Field(i).Name, before, u.CanInterface(), u.CanAddr(), u.Interface())
		if s, ok := u.Interface().(fmt.Stringer); ok {
			fmt.Println("  Stringer:", s.String())
		}
	}
	// Through a pointer: the fields are addressable, Addr works.
	pv := reflect.ValueOf(&o).Elem().Field(2)
	u := unsafeReflectValue(pv)
	fmt.Println(u.CanAddr(), u.Addr().Interface().(*inner).n)
	// The original value is not changed by the bypass of a copy.
	fmt.Println(v.Field(1).CanInterface(), o.secret)
}
