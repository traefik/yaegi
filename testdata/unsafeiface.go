// The two words of an interface read and written through unsafe, as real
// libraries do (docs/UNSAFE.md): protobuf (protoreflect.ifaceHeader,
// impl.pointerOfIface), zerolog (isNilValue) and reflect2 (eface, and its
// cache of types keyed by the type word).
package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

// ifaceHeader is the type of protobuf's internal/protolazy and
// reflect/protoreflect (value_unsafe_go121.go).
type ifaceHeader struct {
	_    [0]any
	Type unsafe.Pointer
	Data unsafe.Pointer
}

// eface is the type of modern-go/reflect2 (reflect2.go).
type eface struct {
	rtype unsafe.Pointer
	data  unsafe.Pointer
}

func unpackEFace(obj any) *eface { return (*eface)(unsafe.Pointer(&obj)) }

func packEFace(rtype, data unsafe.Pointer) any {
	var i any
	e := (*eface)(unsafe.Pointer(&i))
	e.rtype = rtype
	e.data = data
	return i
}

// isNilValue is zerolog's check of a nil pointer in an interface
// (zerolog's internal/json/types.go).
func isNilValue(i any) bool {
	return (*[2]uintptr)(unsafe.Pointer(&i))[1] == 0
}

type T struct{ A, B int }

type Stringer interface{ String() string }

func (t *T) String() string { return fmt.Sprint("T", t.A, t.B) }

func main() {
	// type words: equal for the same dynamic type, as in gc
	var x, y, z any = &T{1, 2}, &T{3, 4}, "s"
	hx := (*ifaceHeader)(unsafe.Pointer(&x))
	hy := (*ifaceHeader)(unsafe.Pointer(&y))
	hz := (*ifaceHeader)(unsafe.Pointer(&z))
	fmt.Println(hx.Type == hy.Type, hx.Type == hz.Type, hx.Data == unsafe.Pointer(x.(*T)))

	// the data word of a pointer is the pointer itself
	p := (*T)(hx.Data)
	p.A = 10
	fmt.Println(x.(*T).A)

	// an interface built again from its words (protobuf's getIface)
	var w any
	hw := (*ifaceHeader)(unsafe.Pointer(&w))
	hw.Type, hw.Data = hy.Type, hy.Data
	fmt.Println(w.(*T).B, w == y)
	fmt.Println(packEFace(unpackEFace(z).rtype, unpackEFace(z).data))

	// zerolog: a nil pointer in an interface
	var np *T
	fmt.Println(isNilValue(np), isNilValue(x), isNilValue(nil))

	// a non-empty interface: the words, then an interface of the same type
	var s Stringer = &T{5, 6}
	ws := (*[2]unsafe.Pointer)(unsafe.Pointer(&s))
	var s2 Stringer
	ws2 := (*[2]unsafe.Pointer)(unsafe.Pointer(&s2))
	ws2[0], ws2[1] = ws[0], ws[1]
	fmt.Println(s2.String())

	// reflect2: the data word of a reflect.Type is the type word of the
	// values of that type (its cache uses both)
	rt := reflect.TypeOf(x)
	fmt.Println(unpackEFace(rt).data == unpackEFace(x).rtype)
	fmt.Println(unpackEFace(reflect.TypeOf(0)).data == unpackEFace(1).rtype, unpackEFace(reflect.TypeOf(0)).data == unpackEFace(z).rtype)
	back := packEFace(unpackEFace(rt).rtype, unpackEFace(rt).data).(reflect.Type)
	fmt.Println(back == rt, back)

	// reflect2: a struct in an interface, its fields by offset from the
	// data word (UnsafeStructField.UnsafeGet on PtrOf(v))
	type rec struct {
		A int
		B []string
		C string
	}
	var rv any = rec{A: 1, B: []string{"x"}, C: "c"}
	data := unpackEFace(rv).data
	fb, _ := reflect.TypeOf(rec{}).FieldByName("B")
	fc, _ := reflect.TypeOf(rec{}).FieldByName("C")
	fmt.Println(*(*int)(data), *(*[]string)(unsafe.Pointer(uintptr(data) + fb.Offset)), *(*string)(unsafe.Add(data, fc.Offset)))
}
