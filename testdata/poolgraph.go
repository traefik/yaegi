// Values that native containers only store and give back (sync.Pool.Put,
// sync.Map.Store, atomic.Value.Store) get an opaque handle (storeFuncs in
// vm/native.go): they come back with their identity, and the graph they
// reach (funcs, a map, a cycle) is unchanged. testdata/protobuf.go has the
// case that broke: protobuf's order.RangeFields puts into a pool a slice
// that reaches the message infos.
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

type pointer struct{ p unsafe.Pointer }

type coder struct {
	name string
	size func(p pointer, c *coder) int
}

type info struct {
	coders []*coder
	byNum  map[int32]*coder
	next   *info
}

type msg struct {
	info *info
	name string
}

// field is protobuf's messageField: a descriptor and a protoreflect.Value,
// which holds the message in an interface.
type field struct {
	fd any
	v  any
}

var pool = sync.Pool{New: func() any { return new([]field) }}

func sizeInt(p pointer, c *coder) int { return *(*int)(p.p) + len(c.name) }

func main() {
	in := &info{coders: []*coder{{"int", sizeInt}}}
	in.byNum = map[int32]*coder{1: in.coders[0]}
	in.next = in // a cycle
	ms := &msg{info: in, name: "m"}
	func() {
		p := pool.Get().(*[]field)
		fields := (*p)[:0]
		defer func() {
			*p = fields
			pool.Put(p)
		}()
		fields = append(fields, field{fd: 1, v: ms}, field{fd: 2, v: in.byNum})
	}()
	n := 40
	fmt.Println(in.coders[0].size(pointer{unsafe.Pointer(&n)}, in.coders[0]))
	fmt.Println(in.byNum[1].size(pointer{unsafe.Pointer(&n)}, in.byNum[1]), in.byNum[1] == in.coders[0])

	// the value comes back the same, when the pool keeps it
	q := pool.Get().(*[]field)
	fmt.Println(len(*q) == 0 || (*q)[0].v.(*msg) == ms)

	var m sync.Map
	m.Store("k", in)
	v, _ := m.Load("k")
	fmt.Println(v.(*info) == in, v.(*info).next == in)
	old, loaded := m.LoadOrStore("k", &info{})
	fmt.Println(old.(*info) == in, loaded)

	var a atomic.Value
	a.Store(in)
	fmt.Println(a.Load().(*info) == in, a.Swap(in).(*info) == in)
	in.coders[0].name = "integer"
	fmt.Println(a.Load().(*info).coders[0].size(pointer{unsafe.Pointer(&n)}, in.coders[0]))
}
