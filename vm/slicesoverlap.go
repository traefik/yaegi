package vm

// slices.overlaps.
//
// slices.Insert and slices.Replace call overlaps(a, b), which compares the
// unsafe addresses of the first and last elements of a and b. In the VM
// the address of an element through unsafe depends on the subslice it
// comes from (docs/UNSAFE.md): &a[2] taken from a[1:] and from a[2:] are
// two different uintptr values. The check was wrong, and Insert with
// overlapping values gave a wrong result. The VM implements overlaps with
// the real memory of the two slices: the cells of a []Value, the bytes of a
// Go []byte, the memory of a native slice view.

import (
	"reflect"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

func slicesOverlaps(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	return func(regs []Value) Value {
		alo, ahi, aok := sliceMem(regs[args[0]])
		blo, bhi, bok := sliceMem(regs[args[1]])
		return Value{N: b2i(aok && bok && alo < bhi && blo < ahi)}
	}
}

// sliceMem returns the memory [lo, hi) of the elements of the slice v. ok
// is false for an empty slice, a slice of elements of size 0, and a
// representation without memory of its own.
func sliceMem(v Value) (lo, hi uintptr, ok bool) {
	switch s := v.R.(type) {
	case []Value:
		if len(s) == 0 {
			return 0, 0, false
		}
		lo = uintptr(unsafe.Pointer(&s[0]))
		return lo, lo + uintptr(len(s))*unsafe.Sizeof(s[0]), true
	case []byte:
		if len(s) == 0 {
			return 0, 0, false
		}
		lo = uintptr(unsafe.Pointer(&s[0]))
		return lo, lo + uintptr(len(s)), true
	case reflect.Value:
		if s.Kind() != reflect.Slice || s.Len() == 0 || s.Type().Elem().Size() == 0 {
			return 0, 0, false
		}
		lo = s.Pointer()
		return lo, lo + uintptr(s.Len())*s.Type().Elem().Size(), true
	}
	return 0, 0, false // nil, a zslice (elements of size 0)
}
