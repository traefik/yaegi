package vm

import (
	"go/types"
	"reflect"
)

// Optimization 5: byte slices coming from native code stay []byte (zero-copy,
// shared with native memory, 1 byte per element instead of a 24-byte Value),
// and make([]byte, n) and []byte(s) make a Go []byte too (memSite.bytes,
// byteArrayOnlySliced): unsafe views of their bytes are real memory
// (docs/UNSAFE.md). Literals and [N]byte arrays used as arrays are cells.
// []byte values may therefore be either []byte- or []Value-backed; the
// slice operations below accept both. A *byte is a Go pointer into a []byte.

func isBytes(r any) bool { _, ok := r.([]byte); return ok }

// isByteType reports whether t is byte (uint8): the elements of a []byte
// in native memory.
func isByteType(t types.Type) bool {
	b := basic(t)
	return b != nil && b.Kind() == types.Uint8
}

func isNilSlice(r any) bool {
	switch s := r.(type) {
	case nil:
		return true
	case []Value:
		return s == nil
	case []byte:
		return s == nil
	case reflect.Value:
		return s.Kind() == reflect.Slice && s.IsNil()
	}
	return false
}

func appendBytes(b []byte, t any) []byte {
	switch t := t.(type) {
	case []byte:
		return append(b, t...)
	case string:
		return append(b, t...)
	case []Value:
		if len(t) > cap(b)-len(b) {
			// The elements go in one append, as in Go: when they do not
			// all fit, none of them is written into the array, which
			// another slice may share.
			tmp := make([]byte, len(t))
			for i, v := range t {
				tmp[i] = byte(v.N)
			}
			return append(b, tmp...)
		}
		for _, v := range t {
			b = append(b, byte(v.N))
		}
	}
	return b
}

func copyBytes(d []byte, t any) int {
	switch t := t.(type) {
	case []byte:
		return copy(d, t)
	case string:
		return copy(d, t)
	case []Value:
		n := 0
		for ; n < len(d) && n < len(t); n++ {
			d[n] = byte(t[n].N)
		}
		return n
	}
	return 0
}
