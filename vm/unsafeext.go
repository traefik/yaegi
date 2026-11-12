package vm

import (
	"go/types"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

// dataExtents keeps the extent of the memory behind each pointer given by
// unsafe.SliceData and unsafe.StringData, for as long as that memory lives.
//
// The ring of regions (uregions) is a small cache. Protobuf keeps the data
// pointer and the length of a []byte or a string in its protoreflect.Value
// (valueOfBytes: unsafe.Pointer(unsafe.SliceData(v))) and makes the slice
// again much later (getBytes: unsafe.Slice((*byte)(v.ptr), v.num)). In
// between, a big init (cel-go builds many descriptors) records more than 64
// regions, the ring forgets the memory, and the VM refused a correct
// unsafe.Slice ("reaches outside the memory known to the VM").
//
// An entry holds the pointer by a weak pointer: it keeps no memory alive,
// and an entry whose memory was freed is never used (a new object at the
// same address has no entry). So the extent is always the one of a live Go
// array that the VM itself gave the pointer of: unsafe.Slice and
// unsafe.String still never reach outside a real Go array. Only memory in
// the Go heap is recorded (a weak pointer needs it); the bytes of a string
// constant of the host keep the old rules.
type dataExtents struct {
	m    sync.Map // uintptr (the pointer) -> *dataExtent
	adds atomic.Int64
}

// dataExtent is the memory from a pointer: n bytes (wb) or n cells of type
// t (we).
type dataExtent struct {
	wb weak.Pointer[byte]
	we weak.Pointer[Value]
	n  int
	t  types.Type
}

// addBytes records that the n bytes from p are part of one Go array. When
// the same live pointer was recorded with more bytes (a longer string with
// the same data), the longer extent is kept: both are inside the array.
func (x *dataExtents) addBytes(p *byte, n int) {
	if n <= 1 {
		return // a single byte is what the VM knows anyway
	}
	k := uintptr(unsafe.Pointer(p))
	if v, ok := x.m.Load(k); ok {
		if e := v.(*dataExtent); e.wb.Value() == p {
			if e.n >= n {
				return
			}
		} else if !inHeap(p) {
			return
		}
	} else if !inHeap(p) {
		return
	}
	x.store(k, &dataExtent{wb: weak.Make(p), n: n})
}

// addCells records that the n cells of type t from p are part of one Go
// array (the longer extent is kept, as in addBytes).
func (x *dataExtents) addCells(p *Value, n int, t types.Type) {
	if n <= 1 {
		return
	}
	k := uintptr(unsafe.Pointer(p))
	if v, ok := x.m.Load(k); ok {
		if e := v.(*dataExtent); e.we.Value() == p && e.t == t {
			if e.n >= n {
				return
			}
		} else if !inHeap(p) {
			return
		}
	} else if !inHeap(p) {
		return
	}
	x.store(k, &dataExtent{we: weak.Make(p), n: n, t: t})
}

func (x *dataExtents) store(k uintptr, e *dataExtent) {
	x.m.Store(k, e)
	if x.adds.Add(1)%4096 == 0 {
		// drop the entries of the memory that is gone
		x.m.Range(func(k, v any) bool {
			if e := v.(*dataExtent); e.wb.Value() == nil && e.we.Value() == nil {
				x.m.CompareAndDelete(k, v)
			}
			return true
		})
	}
}

// region returns the region recorded for the interpreted pointer p, nil
// when there is none or its memory is gone.
func (x *dataExtents) region(p Value) *uregion {
	var k uintptr
	switch q := p.R.(type) {
	case *byte:
		k = uintptr(unsafe.Pointer(q))
	case *Value:
		k = uintptr(unsafe.Pointer(q))
	default:
		return nil
	}
	v, ok := x.m.Load(k)
	if !ok {
		return nil
	}
	e := v.(*dataExtent)
	// One call of Value: the memory can be freed between two calls.
	if b := e.wb.Value(); b != nil {
		if q, ok := p.R.(*byte); ok && q == b {
			return &uregion{base: k, bytes: unsafe.Slice(b, e.n)}
		}
		return nil
	}
	if c := e.we.Value(); c != nil {
		if q, ok := p.R.(*Value); ok && q == c {
			return &uregion{base: k, elems: unsafe.Slice(c, e.n), t: e.t}
		}
	}
	return nil
}
