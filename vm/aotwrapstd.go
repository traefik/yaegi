package vm

// Wrappers of the interfaces of package encoding for the AOT copies.
//
// cmd/aotgen writes the wrapper of each interface that a copy asserts with
// x.(I) or takes as a parameter. It does not see reflect.TypeAssert[I](v),
// which encoding/gob (Go 1.25 and later) uses for encoding.BinaryMarshaler
// and encoding.BinaryUnmarshaler: the copy of gob found no wrapper and the
// call stopped with "no AOT wrapper for interface encoding.BinaryMarshaler"
// for a plugin type with MarshalBinary. These interfaces are small and
// fixed: the VM registers their wrappers itself. A generated wrapper of the
// same interface is the same code; the first registration wins.

import (
	"encoding"
)

func init() {
	RegisterWrapper(func(o Obj) encoding.BinaryMarshaler { return binaryMarshalerW{o} })
	RegisterWrapper(func(o Obj) encoding.BinaryUnmarshaler { return binaryUnmarshalerW{o} })
	RegisterWrapper(func(o Obj) encoding.TextMarshaler { return textMarshalerW{o} })
	RegisterWrapper(func(o Obj) encoding.TextUnmarshaler { return textUnmarshalerW{o} })
}

type binaryMarshalerW struct{ Obj }

func (w binaryMarshalerW) MarshalBinary() ([]byte, error) {
	out := w.Obj.Call("MarshalBinary")
	return Out[[]byte](out, 0), Out[error](out, 1)
}

type binaryUnmarshalerW struct{ Obj }

func (w binaryUnmarshalerW) UnmarshalBinary(b []byte) error {
	return Out[error](w.Obj.Call("UnmarshalBinary", In(&b)), 0)
}

type textMarshalerW struct{ Obj }

func (w textMarshalerW) MarshalText() ([]byte, error) {
	out := w.Obj.Call("MarshalText")
	return Out[[]byte](out, 0), Out[error](out, 1)
}

type textUnmarshalerW struct{ Obj }

func (w textUnmarshalerW) UnmarshalText(b []byte) error {
	return Out[error](w.Obj.Call("UnmarshalText", In(&b)), 0)
}
