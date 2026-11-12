package vm

import (
	"go/types"
	"reflect"
)

// zslice is a large slice whose element type has size zero ([]struct{},
// [][0]int...). gc never allocates memory for such elements, so
// make([]struct{}, maxInt) works. The VM stores one Value (24 bytes) per
// element of a []Value, which cannot hold so many elements. A zslice keeps
// only the length and the capacity: a []struct{} costs no memory, and Go
// itself checks the bounds of index, slice and append operations.
//
// Only make creates a zslice, and only when the capacity is at least
// zsliceMin: smaller slices keep the common []Value form. An array of at
// least zsliceMin zero-size elements is a zslice too (SSA turns make with a
// constant length into an array allocation). The operations
// supported on a zslice are len, cap, index (load and address), slicing,
// append, copy, range and the conversion to native values.
type zslice struct {
	s    []struct{}
	zero Value // zero value of the element type
	t    types.Type
}

// zsliceMin is the capacity from which make returns a zslice (a []Value of
// this size uses 24 MiB).
const zsliceMin = 1 << 20

// zeroSized reports whether values of t use no memory under gc.
func zeroSized(t types.Type) (zero bool) {
	defer func() {
		if recover() != nil { // type parameters have no size
			zero = false
		}
	}()
	return gcSizes.Sizeof(t) == 0
}

// elem returns a new copy of the zero element (index load, address).
func (z *zslice) elem() Value { return deepCopy(z.zero, z.t) }

// addr returns the address of z[i]: a new cell, since an element has no
// state.
func (z *zslice) addr(i int64) Value {
	_ = z.s[i]
	e := z.elem()
	return Value{R: &e}
}

// slice returns z[lo:hi:mx] (three is true) or z[lo:hi]. Go checks the
// bounds, with the gc messages.
func (z *zslice) slice(lo, hi, mx int, three bool) *zslice {
	if three {
		return &zslice{s: z.s[lo:hi:mx], zero: z.zero, t: z.t}
	}
	return &zslice{s: z.s[lo:hi], zero: z.zero, t: z.t}
}

// appendN returns z with n more elements. Go panics (len out of range) when
// the length overflows.
func (z *zslice) appendN(n int) *zslice {
	if n == 0 {
		return z
	}
	return &zslice{s: append(z.s, make([]struct{}, n)...), zero: z.zero, t: z.t}
}

// native converts z to the native slice type rt.
func (z *zslice) native(rt reflect.Type) reflect.Value {
	if rt.Elem().Size() != 0 {
		unsupported("native conversion of a large slice of zero-size %s", z.t)
	}
	return reflect.MakeSlice(rt, len(z.s), cap(z.s))
}

// seqLen returns the length of a source of append or copy.
func seqLen(v any) int {
	switch t := v.(type) {
	case []Value:
		return len(t)
	case []byte:
		return len(t)
	case string:
		return len(t)
	case *zslice:
		return len(t.s)
	}
	return 0
}
