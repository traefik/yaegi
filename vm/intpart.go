package vm

import (
	"go/types"
)

// intPart is a pointer to bytes inside an integer held in a cell: the
// address of an element plus an offset inside it, as ristretto's bloom
// filter (badger) reads and writes one byte of a uint64 of its bitset:
// *(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(&bitset[i])) + k)).
//
// The cell has no bytes. A load through the pointer takes the bytes of the
// integer that gc has at this offset (in the order of the host), and a store
// changes them in the integer: the cell is read, changed and written again.
// So, unlike gc, two goroutines that write two bytes of the same integer at
// the same time without a lock can lose one of the writes (a data race in Go
// already when they write the same bytes).
type intPart struct {
	cell *Value
	t    *types.Basic // the integer type of the cell
	off  int64        // offset of the bytes in the integer
}

// intPartOf returns the pointer at offset off inside the integer of type t
// held by cell, or false when t is not an integer wider than off.
func intPartOf(cell *Value, t types.Type, off int64) (Value, bool) {
	b, ok := t.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsInteger == 0 || off <= 0 || off >= gcSizes.Sizeof(b) {
		return Value{}, false
	}
	return Value{R: &intPart{cell: cell, t: b, off: off}}, true
}

// shift returns the bit shift of the n bytes at p in the integer, and the
// mask of n bytes; it refuses n bytes that do not fit in the integer, or a
// type that is not an integer.
func (p *intPart) shift(t types.Type) (uint, uint64) {
	b, ok := t.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsInteger == 0 {
		unsupported("unsafe: reading a %s inside a %s needs byte-level memory", t, p.t)
	}
	size, n := gcSizes.Sizeof(p.t), gcSizes.Sizeof(b)
	if p.off+n > size {
		unsupported("unsafe: %d bytes at offset %d of a %s reach outside the value", n, p.off, p.t)
	}
	at := p.off
	if !littleEndian {
		at = size - p.off - n
	}
	mask := ^uint64(0)
	if n < 8 {
		mask = 1<<(8*uint(n)) - 1
	}
	return uint(8 * at), mask
}

// load reads the bytes at p as a value of the integer type t.
func (p *intPart) load(t types.Type) Value {
	sh, mask := p.shift(t)
	n := loadCell(p.cell, p.t).N
	return Value{N: intOfBits(uint64(n)>>sh&mask, t.Underlying().(*types.Basic))}
}

// store writes v, of the integer type t, into the bytes at p.
func (p *intPart) store(v Value, t types.Type) {
	sh, mask := p.shift(t)
	n := loadCell(p.cell, p.t).N
	u := uint64(n)&^(mask<<sh) | (uint64(v.N)&mask)<<sh
	storeCell(p.cell, Value{N: intOfBits(u, p.t)}, p.t)
}

// intOfBits is the value of the integer type b whose bits are the low bits
// of u, as the VM holds it in Value.N (sign-extended for a signed type).
func intOfBits(u uint64, b *types.Basic) int64 {
	size := gcSizes.Sizeof(b)
	if size >= 8 {
		return int64(u)
	}
	bits := 8 * uint(size)
	u &= 1<<bits - 1
	if b.Info()&types.IsUnsigned == 0 && u>>(bits-1) == 1 {
		return int64(u) - 1<<bits
	}
	return int64(u)
}
