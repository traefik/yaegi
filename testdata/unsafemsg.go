// The unsafe idioms of protobuf's internal/impl on the structs of a
// program (docs/UNSAFE.md): a message and its first field (messageState)
// have the same address, fields are reached with the offsets of
// reflect.StructField, a []*T is read as a []pointer, and the walk of a tree
// of messages comes back to the outer ones.
package main

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"unsafe"
)

type pointer struct{ p unsafe.Pointer }

func pointerOf(p unsafe.Pointer) pointer { return pointer{p: p} }

func (p pointer) IsNil() bool               { return p.p == nil }
func (p pointer) Apply(off uintptr) pointer { return pointer{p: unsafe.Pointer(uintptr(p.p) + off)} }
func (p pointer) Elem() pointer             { return pointer{p: *(*unsafe.Pointer)(p.p)} }
func (p pointer) PointerSlice() []pointer   { return *(*[]pointer)(p.p) }
func (p pointer) AppendPointerSlice(v pointer) {
	*(*[]pointer)(p.p) = append(*(*[]pointer)(p.p), v)
}
func (p pointer) AsValueOf(t reflect.Type) reflect.Value { return reflect.NewAt(t, p.p) }
func (p pointer) Float64() *float64                      { return (*float64)(p.p) }

func pointerOfValue(v reflect.Value) pointer { return pointer{p: unsafe.Pointer(v.Pointer())} }

type info struct{ name string }

// messageState is the first field of every message.
type messageState struct {
	_    [0]func()
	info *info
}

func stateOf(p unsafe.Pointer) *messageState { return (*messageState)(p) }

func (ms *messageState) load() *info {
	return (*info)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&ms.info))))
}

func (ms *messageState) store(i *info) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&ms.info)), unsafe.Pointer(i))
}

func (ms *messageState) pointer() pointer { return pointer{p: unsafe.Pointer(ms)} }

// The messages: a Value is a number or a list (a oneof), a List holds
// Values, a Struct maps names to Values.
type isKind interface{ isKind() }

type Value struct {
	state messageState
	size  int32
	Kind  isKind
}

type Number struct{ N float64 }
type ListKind struct{ L *List }

func (*Number) isKind()   {}
func (*ListKind) isKind() {}

type List struct {
	state  messageState
	Values []*Value
}

type Struct struct {
	state  messageState
	Fields map[string]*Value
	Tail   *Value
}

var valueInfo = &info{"Value"}

func (x *Value) reflectState() *messageState {
	ms := stateOf(unsafe.Pointer(x))
	if ms.load() == nil {
		ms.store(valueInfo)
	}
	return ms
}

func (x *Value) Reset() {
	*x = Value{}
	stateOf(unsafe.Pointer(x)).store(valueInfo)
}

func field(t any, name string) reflect.StructField {
	f, _ := reflect.TypeOf(t).FieldByName(name)
	return f
}

var (
	kindF   = field(Value{}, "Kind")
	numF    = field(Number{}, "N")
	valuesF = field(List{}, "Values")
	fieldsF = field(Struct{}, "Fields")
	tailF   = field(Struct{}, "Tail")
)

// sum walks a Value the way protobuf's size functions do.
func sum(p pointer) float64 {
	k := p.Apply(kindF.Offset).AsValueOf(kindF.Type).Elem()
	if k.IsNil() {
		return 0
	}
	k = k.Elem()
	q := pointerOfValue(k).Apply(0)
	switch k.Elem().Type() {
	case reflect.TypeOf(Number{}):
		return *q.Apply(numF.Offset).Float64()
	case reflect.TypeOf(ListKind{}):
		n := 0.0
		for _, e := range q.Elem().Apply(valuesF.Offset).PointerSlice() {
			n += sum(e)
		}
		return n
	}
	return -1
}

func sumStruct(s *Struct) float64 {
	p := pointerOf(unsafe.Pointer(s))
	n := 0.0
	iter := p.Apply(fieldsF.Offset).AsValueOf(fieldsF.Type).Elem().MapRange()
	for iter.Next() {
		n += sum(pointerOfValue(iter.Value()))
	}
	// back to the outer message after the walk
	if t := p.Apply(tailF.Offset); !t.Elem().IsNil() {
		n += sum(t.Elem())
	}
	return n
}

func num(f float64) *Value { return &Value{Kind: &Number{f}} }

func list(vs ...*Value) *Value { return &Value{Kind: &ListKind{&List{Values: vs}}} }

func main() {
	// the message and its first field
	x := &Value{Kind: &Number{1.5}}
	ms := x.reflectState()
	fmt.Println(ms.load().name, x.reflectState() == ms)
	back := reflect.NewAt(reflect.TypeOf(*x), ms.pointer().p).Interface().(*Value)
	fmt.Println(back == x)
	back.Reset()
	fmt.Println(x.Kind == nil, x.reflectState().load() == valueInfo, ms.load() == valueInfo)
	var y Value
	fmt.Println(y.reflectState().load().name, (&y).reflectState() == y.reflectState())

	// fields by offset: a pointer, a slice and a map seen as a pointer word
	s := &Struct{}
	p := pointerOf(unsafe.Pointer(s))
	fmt.Println(p.Apply(fieldsF.Offset).Elem().IsNil(), p.Apply(tailF.Offset).Elem().IsNil())
	s.Fields = map[string]*Value{}
	s.Tail = num(2)
	fmt.Println(p.Apply(fieldsF.Offset).Elem().IsNil(), p.Apply(tailF.Offset).Elem().IsNil())
	l := &List{}
	lp := pointerOf(unsafe.Pointer(l)).Apply(valuesF.Offset)
	fmt.Println(lp.Elem().IsNil(), len(lp.PointerSlice()))
	lp.AppendPointerSlice(pointerOf(unsafe.Pointer(num(3))))
	lp.AppendPointerSlice(pointerOf(unsafe.Pointer(num(4))))
	fmt.Println(lp.Elem().IsNil(), len(l.Values), sum(pointerOf(unsafe.Pointer(l.Values[1]))))

	// a tree of more than a hundred messages, walked through unsafe
	for i := 0; i < 20; i++ {
		var vs []*Value
		for j := 0; j < 5; j++ {
			vs = append(vs, num(float64(i*j)), list(num(1), list(num(0.5))))
		}
		s.Fields[fmt.Sprint("k", i)] = list(vs...)
	}
	fmt.Println(sumStruct(s))
	fmt.Println(sumStruct(s))
}
