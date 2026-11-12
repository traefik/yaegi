// Native boundary of encoding/binary: program types given to binary (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// Named basic types, over the types of the fast paths of binary.
type (
	NBool bool
	NI8   int8
	NU8   uint8
	NI16  int16
	NU16  uint16
	NI32  int32
	NU32  uint32
	NI64  int64
	NU64  uint64
	NF32  float32
	NF64  float64
	NC64  complex64
	Bytes []byte
	U16s  []uint16
	Arr   [3]NU16
)

// Hdr is a fixed-size struct with named fields.
type Hdr struct {
	Magic NU32
	_     [2]byte
	Flags [2]NBool
	Len   NI16
	F     NF32
	Sub   Sub
}

type Sub struct {
	A NU8
	B NI64
}

// Pair is a generic fixed-size struct.
type Pair[T any] struct{ A, B T }

// NotFixed has a field of a size that is not fixed.
type NotFixed struct {
	A NU8
	S string
}

// encodeAll gives v to Size, Write, Append and Encode.
func encodeAll(label string, v any) {
	out("%s Size: %d", label, binary.Size(v))
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.LittleEndian, v)
	out("%s Write: %x %v", label, buf.Bytes(), err)
	b, err := binary.Append([]byte{0xff}, binary.BigEndian, v)
	out("%s Append: %x %v", label, b, err)
	dst := make([]byte, 40)
	n, err := binary.Encode(dst, binary.LittleEndian, v)
	out("%s Encode: %d %x %v", label, n, dst[:max(n, 0)], err)
	n, err = binary.Encode(make([]byte, 1), binary.LittleEndian, v)
	out("%s Encode short: %d %v", label, n, err)
}

var data = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

// decodeAll decodes data with Read and Decode into new targets made by mk.
func decodeAll(label string, mk func() any) {
	t := mk()
	err := binary.Read(bytes.NewReader(data), binary.BigEndian, t)
	out("%s Read: %+v %v", label, deref(t), err)
	t = mk()
	n, err := binary.Decode(data, binary.LittleEndian, t)
	out("%s Decode: %d %+v %v", label, n, deref(t), err)
	t = mk()
	n, err = binary.Decode(data[:1], binary.LittleEndian, t)
	out("%s Decode short: %d %v", label, n, err)
}

func deref(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		return rv.Elem().Interface()
	}
	return v
}

func main() {
	// row: encoding/binary Size,Write,Append,Encode assert x.(T), T unnamed
	// row: encoding/binary Size,Write,Append,Encode reflect kind
	probe("Encode/basic", func() {
		u16 := uint16(0x0102)
		n16 := NU16(0x0102)
		vals := []struct {
			l string
			v any
		}{
			{"uint16", u16}, {"NU16", n16}, {"*uint16", &u16}, {"*NU16", &n16},
			{"NBool", NBool(true)}, {"*NBool", func() *NBool { b := NBool(true); return &b }()},
			{"NI8", NI8(-1)}, {"NU8", NU8(7)}, {"NI16", NI16(-2)}, {"NI32", NI32(3)}, {"NU32", NU32(4)},
			{"NI64", NI64(-5)}, {"NU64", NU64(6)}, {"NF32", NF32(1.5)}, {"NF64", NF64(-2.25)}, {"NC64", NC64(1 + 2i)},
			{"[]uint16", []uint16{1, 2}}, {"U16s", U16s{1, 2}}, {"[]NU16", []NU16{1, 2}},
			{"[]byte", []byte{1, 2}}, {"Bytes", Bytes{1, 2}}, {"[]NBool", []NBool{true, false}},
			{"[]NF64", []NF64{1}}, {"Arr", Arr{1, 2, 3}}, {"*Arr", &Arr{4, 5, 6}},
		}
		for _, v := range vals {
			encodeAll(v.l, v.v)
		}
	})
	// A nil pointer of a named type takes the reflect path: an error.
	// row: encoding/binary Size,Write,Append,Encode assert x.(T), T unnamed
	probe("Encode/nilnamed", func() {
		encodeAll("nil *NU16", (*NU16)(nil))
	})
	// A nil *uint16 takes the fast path of Write: Go panics there.
	// row: encoding/binary Size,Write,Append,Encode assert x.(T), T unnamed
	probe("Encode/nilfast", func() {
		encodeAll("nil *uint16", (*uint16)(nil))
	})
	// row: encoding/binary Size,Write,Append,Encode reflect walk
	// row: encoding/binary Size,Write,Append,Encode reflect length
	// row: encoding/binary Size,Write,Append,Encode panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Encode/structs", func() {
		h := Hdr{Magic: 0xcafe, Flags: [2]NBool{true, false}, Len: -1, F: 0.5, Sub: Sub{A: 9, B: -9}}
		encodeAll("Hdr", h)
		encodeAll("*Hdr", &h)
		// twice: the second time takes the size from the cache of binary
		encodeAll("[]Hdr", []Hdr{h, h})
		encodeAll("[]Hdr again", []Hdr{h})
		encodeAll("[2]Sub", [2]Sub{{1, 2}, {3, 4}})
		encodeAll("[2]Sub again", [2]Sub{{5, 6}, {7, 8}})
		encodeAll("Pair", Pair[NU16]{1, 2})
		encodeAll("[]Pair", []Pair[NI32]{{1, 2}, {3, 4}})
		encodeAll("[]Pair again", []Pair[NI32]{{5, 6}})
		encodeAll("empty []Sub", []Sub{})
	})
	// row: encoding/binary Write,Append,Encode reflect type name
	probe("Encode/notfixed", func() {
		for _, v := range []any{1, NotFixed{1, "x"}, &NotFixed{}, []NotFixed{{}}, map[NU8]NU8{}, Pair[string]{}, []int{1}, struct{ P *NU8 }{}} {
			encodeAll(fmt.Sprintf("%T", v), v)
		}
	})
	// row: encoding/binary Read,Decode assert x.(T), T unnamed
	// row: encoding/binary Read,Decode reflect kind
	// row: encoding/binary Read,Decode reflect type name
	probe("Decode/basic", func() {
		decodeAll("*uint16", func() any { return new(uint16) })
		decodeAll("*NU16", func() any { return new(NU16) })
		decodeAll("*NBool", func() any { return new(NBool) })
		decodeAll("*NI8", func() any { return new(NI8) })
		decodeAll("*NU8", func() any { return new(NU8) })
		decodeAll("*NI32", func() any { return new(NI32) })
		decodeAll("*NU64", func() any { return new(NU64) })
		decodeAll("*NF32", func() any { return new(NF32) })
		decodeAll("*NF64", func() any { return new(NF64) })
		decodeAll("*NC64", func() any { return new(NC64) })
		decodeAll("[]uint16", func() any { return make([]uint16, 2) })
		decodeAll("U16s", func() any { return make(U16s, 2) })
		decodeAll("[]NU16", func() any { return make([]NU16, 2) })
		decodeAll("Bytes", func() any { return make(Bytes, 3) })
		decodeAll("*Arr", func() any { return &Arr{} })
		decodeAll("NU16 value", func() any { return NU16(0) })
		decodeAll("*int", func() any { return new(int) })
		decodeAll("*NotFixed", func() any { return &NotFixed{} })
	})
	// row: encoding/binary Read,Decode reflect walk
	// row: encoding/binary Read,Decode reflect length
	// row: encoding/binary Read,Decode panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Decode/structs", func() {
		decodeAll("*Hdr", func() any { return &Hdr{} })
		decodeAll("[]Sub", func() any { return make([]Sub, 2) })
		decodeAll("[]Sub again", func() any { return make([]Sub, 1) })
		decodeAll("*[2]Sub", func() any { return &[2]Sub{} })
		decodeAll("*Pair", func() any { return &Pair[NU16]{} })
		decodeAll("[]Pair", func() any { return make([]Pair[NI32], 2) })
		decodeAll("Hdr value", func() any { return Hdr{} })
	})
}
