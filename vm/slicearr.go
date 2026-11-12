package vm

import (
	"fmt"
	"go/types"
	"reflect"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// sliceToArrayPointer compiles (*[N]T)(s). The array pointer shares the
// backing array of s: it is a new cell that holds s[:N:N], so writes through
// the pointer and through the slice see each other. The conversion panics
// when len(s) < N. A nil slice gives a nil pointer. [N]T(s) is the same
// conversion followed by a load.
func (c *compiler) sliceToArrayPointer(v *ssa.SliceToArrayPointer) {
	arr := v.Type().Underlying().(*types.Pointer).Elem()
	n := arr.Underlying().(*types.Array).Len()
	x := c.reg(v.X)
	m := c.m
	c.emit(Instr{Op: OpFunc, A: c.reg(v), X: func(regs []Value) Value {
		var l int64
		switch s := regs[x].R.(type) {
		case nil:
		case []Value:
			if l = int64(len(s)); l >= n {
				if s == nil {
					return Value{}
				}
				return Value{R: &Value{R: s[:n:n]}}
			}
		case []byte:
			if l = int64(len(s)); l >= n {
				if s == nil {
					return Value{}
				}
				// a byte slice kept as []byte (bytes.go): a native pointer
				// into the same bytes
				return Value{R: reflect.NewAt(m.reflectType(arr), unsafe.Pointer(unsafe.SliceData(s)))}
			}
		case *zslice:
			// a large slice of zero-size elements: the elements have no
			// state, so the array does not need to share them
			if l = int64(len(s.s)); l >= n {
				if n >= zsliceMin {
					return Value{R: &Value{R: s.slice(0, int(n), int(n), true)}}
				}
				es := make([]Value, n)
				for i := range es {
					es[i] = s.elem()
				}
				return Value{R: &Value{R: es}}
			}
		case reflect.Value:
			if l = int64(s.Len()); l >= n {
				if s.IsNil() {
					return Value{}
				}
				return Value{R: s.Slice3(0, int(n), int(n)).Convert(m.reflectType(v.Type()))}
			}
		default:
			unsupported("slice to array pointer from %T", s)
		}
		if n == 0 {
			return Value{} // nil slice
		}
		panic(runtimeError(fmt.Sprintf("cannot convert slice with length %d to array or pointer to array with length %d", l, n)))
	}})
}

// zeroSizeAddr is the address of every zero-size variable, like
// runtime.zerobase in compiled Go.
var zeroSizeAddr = new(byte)

// ptrInfo describes the element type of a pointer type, for ptrKey.
type ptrInfo uint8

const (
	ptrZeroSize ptrInfo = 1 << iota // the element has size zero
	ptrArray                        // the element is an array
	ptrKnown                        // the info is computed
)

// ptrInfoOf computes the ptrInfo of t. Sizeof walks the type each time, and
// pointer == and map keys are hot paths, so the result is kept where it is
// used: in the compiled sites (ptrCmp, mapSite), in the RType of a dynamic
// type (keys of interface type) and in the Machine (Machine.ptrInfo). There
// is no global cache: it would keep the types of every loaded program alive.
func ptrInfoOf(t *types.Pointer) ptrInfo {
	i := ptrKnown
	if gcSizes.Sizeof(t.Elem()) == 0 {
		i |= ptrZeroSize
	}
	if _, ok := t.Elem().Underlying().(*types.Array); ok {
		i |= ptrArray
	}
	return i
}

// ptrInfo is ptrInfoOf(t), cached in the Machine (for == on pointers inside
// struct, array and interface values).
func (m *Machine) ptrInfo(t *types.Pointer) ptrInfo {
	if i, ok := m.ptrInfos.Load(t); ok {
		return i.(ptrInfo)
	}
	i := ptrInfoOf(t)
	m.ptrInfos.Store(t, i)
	return i
}

// ptrInfo is ptrInfoOf(t) for the dynamic type rt = t, computed once.
func (rt *RType) ptrInfo(t *types.Pointer) ptrInfo {
	if i := ptrInfo(rt.ptr.Load()); i != 0 {
		return i
	}
	i := ptrInfoOf(t)
	rt.ptr.Store(uint32(i))
	return i
}

// ptrCmp is the X of OpEqV and OpNeV for a pointer type: the type info is
// computed once, at compile time.
type ptrCmp struct {
	t    *types.Pointer
	info ptrInfo
}

func (p *ptrCmp) equal(a, b Value) bool { return ptrEqual(a, b, p.info, p.t) }

// eqSite returns the X of OpEqV and OpNeV for operands of type t.
func eqSite(t types.Type) any {
	if pt, ok := t.Underlying().(*types.Pointer); ok {
		return &ptrCmp{t: pt, info: ptrInfoOf(pt)}
	}
	return t
}

// ptrIsNil reports whether the pointer v is nil.
func ptrIsNil(v Value) bool {
	switch p := v.R.(type) {
	case nil:
		return true
	case reflect.Value:
		switch p.Kind() {
		case reflect.Pointer, reflect.UnsafePointer:
			return p.IsNil()
		}
	}
	return false
}

// ptrEqual is a == b for two pointers of type t. info is ptrInfoOf(t), or
// 0 when it is not computed yet.
func ptrEqual(a, b Value, info ptrInfo, t *types.Pointer) bool {
	if an, bn := ptrIsNil(a), ptrIsNil(b); an || bn {
		return an == bn // fast path: a compare with nil needs no type info
	}
	return ptrKeyOf(a, info, t) == ptrKeyOf(b, info, t)
}

// ptrKey is the identity of a pointer of type t, for == and map keys.
//   - All pointers to zero-size values are equal (the gc toolchain gives
//     them the same address, runtime.zerobase).
//   - An array pointer is identified by the address of its first element:
//     the array pointer made from a slice is a different cell that shares
//     the elements of the original array.
func ptrKey(v Value, t *types.Pointer) any { return ptrKeyOf(v, 0, t) }

// ptrKeyOf is ptrKey with info = ptrInfoOf(t) when info is not 0 (t can
// then be nil).
func ptrKeyOf(v Value, info ptrInfo, t *types.Pointer) any {
	switch p := v.R.(type) {
	case nil:
		return nil
	case *Value:
		if info == 0 {
			info = ptrInfoOf(t)
		}
		if info&ptrZeroSize != 0 {
			return unsafe.Pointer(zeroSizeAddr)
		}
		if info&ptrArray != 0 {
			if es, ok := p.R.([]Value); ok && len(es) > 0 {
				return unsafe.Pointer(&es[0])
			}
		}
		if rv, ok := p.R.(reflect.Value); ok && rv.Kind() == reflect.Struct && rv.CanAddr() {
			// A native struct that lives in native memory (var z
			// big.Float): native code gets its address, and a pointer it
			// gives back (z.Copy(&x) returns &z) is that address, so the
			// identity is the address of the memory, not of the cell
			// (ExampleFloat_Copy of package math/big). assign writes into
			// that memory, so the key does not change.
			return rv.Addr().UnsafePointer()
		}
		return unsafe.Pointer(p)
	case *byte:
		return unsafe.Pointer(p)
	case roByte:
		return unsafe.Pointer(p)
	case *byteView:
		return unsafe.Pointer(p.ptr()) // the same key as the byte it starts at
	case reflect.Value:
		if p.Kind() != reflect.Pointer && p.Kind() != reflect.UnsafePointer {
			return p.Pointer()
		}
		if p.IsNil() {
			return nil
		}
		if info == 0 {
			info = ptrInfoOf(t)
		}
		if info&ptrZeroSize != 0 {
			return unsafe.Pointer(zeroSizeAddr)
		}
		return p.UnsafePointer()
	}
	return v.R
}
