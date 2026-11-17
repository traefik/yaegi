// The unsafe idioms of badger (dgraph-io/badger/v4) and ristretto that read
// and write its tables: slice headers between []byte and []uint32, a struct
// of a method receiver copied from bytes, and one byte of a uint64 of a
// bloom filter.
package main

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"unsafe"
)

// y.BytesToU32Slice: a []uint32 over the bytes of a []byte.
func bytesToU32Slice(b []byte) []uint32 {
	if len(b) == 0 {
		return nil
	}
	var u32s []uint32
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&u32s))
	hdr.Len = len(b) / 4
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&b[0]))
	return u32s
}

// y.U32SliceToBytes: the bytes of a []uint32.
func u32SliceToBytes(u32s []uint32) []byte {
	if len(u32s) == 0 {
		return nil
	}
	var b []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	hdr.Len = len(u32s) * 4
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&u32s[0]))
	return b
}

const headerSize = 4

// table.header: the header of an entry of a block.
type header struct {
	overlap uint16
	diff    uint16
}

func (h header) Encode() []byte {
	var b [4]byte
	*(*header)(unsafe.Pointer(&b[0])) = h
	return b[:]
}

func (h *header) Decode(buf []byte) {
	copy(((*[headerSize]byte)(unsafe.Pointer(h))[:]), buf[:headerSize])
}

type entry struct {
	h   header
	idx int
}

var mask = []uint8{1, 2, 4, 8, 16, 32, 64, 128}

// z.Bloom: one byte of a uint64 of the bitset.
type bloom struct {
	bitset []uint64
}

func (bl *bloom) Set(idx uint64) {
	ptr := unsafe.Pointer(uintptr(unsafe.Pointer(&bl.bitset[idx>>6])) + uintptr((idx%64)>>3))
	*(*uint8)(ptr) |= mask[idx%8]
}

func (bl *bloom) IsSet(idx uint64) bool {
	ptr := unsafe.Pointer(uintptr(unsafe.Pointer(&bl.bitset[idx>>6])) + uintptr((idx%64)>>3))
	return ((*(*uint8)(ptr))>>(idx%8))&1 == 1
}

func main() {
	// The entry offsets of a block, written and read back.
	var offsets []uint32
	for i := range 5 {
		offsets = append(offsets, uint32(i*100+1))
	}
	data := append([]byte{0xff, 0xff}, u32SliceToBytes(offsets)...)
	fmt.Println(len(data), data)
	u := bytesToU32Slice(data[2:])
	fmt.Println(len(u), cap(u), u)
	u[1] = 77
	fmt.Println(binary.LittleEndian.Uint32(data[6:]) == 77 || binary.BigEndian.Uint32(data[6:]) == 77, u)
	fmt.Println(bytesToU32Slice(nil) == nil, u32SliceToBytes(nil) == nil)

	// Entry headers.
	enc := header{overlap: 3, diff: 513}.Encode()
	var h header
	h.Decode(enc)
	fmt.Println(h.overlap, h.diff)
	var e entry
	e.h.Decode([]byte{9, 0, 8, 0, 1})
	e.idx = 4
	fmt.Println(e.h, e.idx)

	// A bloom filter.
	bl := &bloom{bitset: make([]uint64, 4)}
	for _, i := range []uint64{0, 9, 63, 64, 130, 255} {
		bl.Set(i)
	}
	n := 0
	for i := range uint64(256) {
		if bl.IsSet(i) {
			n++
		}
	}
	fmt.Println(bl.bitset, n, bl.IsSet(9), bl.IsSet(10))

	// Two bytes inside a signed word.
	ws := []int32{-1, 5}
	p := (*int16)(unsafe.Pointer(uintptr(unsafe.Pointer(&ws[0])) + 2))
	fmt.Println(*p)
	*p = 7
	fmt.Println(ws)
}
