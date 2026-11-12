// Typed views of real bytes (vm/byteview.go): plain data laid over a Go
// []byte or over the bytes of a string, as xxhash, freecache and x/sys do.
// The idioms of docs/VM2-DESIGN.md (13.3): the word read of xxhash at an
// unaligned offset, the entry header of freecache written through a struct
// pointer and copied back, (*[N]byte) over a buffer, and the ways a view
// is used afterwards: field and element addresses, unsafe.Add,
// unsafe.Slice back to bytes, pointer equality, a view given to native
// code, a read-only view of a string. The expected words are assembled by
// hand in little-endian order: on a big-endian host both go run and the VM
// print false there, and still agree.
package main

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

type entryHdr struct {
	accessTime uint32
	expireAt   uint32
	keyLen     uint16
	hash16     uint16
	valLen     uint32
	valCap     uint32
	deleted    bool
	slotID     uint8
	reserved   uint16
}

type nested struct {
	A   int8
	B   int16
	C   [3]uint16
	D   float32
	E   float64
	F   [2]struct{ X, Y int32 }
	Pad bool
}

type pair struct {
	A uint32
	B uint16
}

// heap keeps the buffers that get a float64 or float32 view on the heap,
// where a buffer of 16 or 64 bytes is 8-byte aligned. On the stack a byte
// array has no alignment, and on arm the floating point loads and stores
// of go run died with SIGBUS (they need an aligned address).
var heap [][]byte

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
func le32(b []byte) uint32 { return uint32(le16(b)) | uint32(le16(b[2:]))<<16 }
func le64(b []byte) uint64 { return uint64(le32(b)) | uint64(le32(b[4:]))<<32 }

func wordReads() {
	b := make([]byte, 16)
	heap = append(heap, b)
	for i := range b {
		b[i] = byte(i + 1)
	}
	w0 := *(*uint64)(unsafe.Pointer(&b[0]))
	w1 := *(*uint64)(unsafe.Pointer(&b[1])) // unaligned
	u32 := *(*uint32)(unsafe.Pointer(&b[3]))
	i16 := *(*int16)(unsafe.Pointer(&b[6]))
	fmt.Println(w0 == le64(b), w1 == le64(b[1:]), u32 == le32(b[3:]), i16 == int16(le16(b[6:])))
	// A write through the view shows in the bytes.
	*(*uint32)(unsafe.Pointer(&b[8])) = 0xdeadbeef
	fmt.Println(le32(b[8:]) == 0xdeadbeef, b[0], b[15])
	// Signed and narrow views keep their sign.
	b[0] = 0xff
	fmt.Println(*(*int8)(unsafe.Pointer(&b[0])), *(*uint8)(unsafe.Pointer(&b[0])), *(*int32)(unsafe.Pointer(&b[0])) < 0)
	// Floats.
	*(*float64)(unsafe.Pointer(&b[0])) = 2.5
	*(*float32)(unsafe.Pointer(&b[8])) = -1.25
	fmt.Println(*(*float64)(unsafe.Pointer(&b[0])), *(*float32)(unsafe.Pointer(&b[8])))
	// The pointer data of a slice made by unsafe.SliceData.
	p := unsafe.SliceData(b)
	fmt.Println(*(*uint16)(unsafe.Pointer(p)) == le16(b))
	// A bool view of a byte that is not 0 or 1 reads as true.
	b[2] = 9
	fmt.Println(*(*bool)(unsafe.Pointer(&b[2])), *(*bool)(unsafe.Pointer(&b[3])) == (b[3] != 0))
}

func header() {
	buf := make([]byte, 64)
	hdr := (*entryHdr)(unsafe.Pointer(&buf[0]))
	hdr.accessTime = 1700000000
	hdr.expireAt = 1700003600
	hdr.keyLen = 5
	hdr.hash16 = 0xabcd
	hdr.valLen = 1000
	hdr.valCap = 1024
	hdr.deleted = true
	hdr.slotID = 7
	fmt.Println(le32(buf[0:]), le32(buf[4:]), le16(buf[8:]), le16(buf[10:]), le32(buf[12:]), le32(buf[16:]), buf[20], buf[21])
	// The whole struct, read back and copied elsewhere in the buffer.
	h := *hdr
	h.valLen = 2000
	*(*entryHdr)(unsafe.Pointer(&buf[32])) = h
	hdr2 := (*entryHdr)(unsafe.Pointer(&buf[32]))
	fmt.Println(hdr2.accessTime, hdr2.valLen, hdr.valLen, hdr2.deleted, hdr2.slotID, unsafe.Sizeof(h))
	// Field by field through a second view of the same bytes.
	fmt.Println(hdr2.keyLen == hdr.keyLen, hdr2.hash16 == hdr.hash16)
	// Pointer equality and arithmetic.
	fmt.Println(unsafe.Pointer(hdr) == unsafe.Pointer(&buf[0]), unsafe.Pointer(hdr2) == unsafe.Add(unsafe.Pointer(hdr), 32))
	// Back to bytes.
	raw := unsafe.Slice((*byte)(unsafe.Pointer(hdr)), int(unsafe.Sizeof(*hdr)))
	fmt.Println(len(raw), raw[0] == buf[0], &raw[0] == &buf[0])
	raw[0] = 9
	fmt.Println(hdr.accessTime&0xff == 9 || hdr.accessTime>>24 == 9)
	// unsafe.Add then a view.
	v := (*uint32)(unsafe.Add(unsafe.Pointer(&buf[0]), 12))
	fmt.Println(*v == hdr.valLen)
}

func arrays() {
	b := make([]byte, 12)
	for i := range b {
		b[i] = byte('a' + i)
	}
	a := (*[4]byte)(unsafe.Pointer(&b[2]))
	fmt.Println(string(a[:]), a[0], len(a))
	a[1] = 'Z'
	fmt.Println(string(b))
	words := (*[3]uint32)(unsafe.Pointer(&b[0]))
	words[2] = 0x01020304
	fmt.Println(le32(b[8:]) == 0x01020304, words[0] == le32(b))
	// Element address of an array view, then a word view of it.
	e := &words[1]
	*e = 42
	fmt.Println(le32(b[4:]), *(*uint16)(unsafe.Pointer(e)) == 42 || *(*uint16)(unsafe.Add(unsafe.Pointer(e), 2)) == 42)
	// A copy of the array value.
	cp := *words
	words[0] = 1
	fmt.Println(cp[0] != 1, cp[2] == 0x01020304)
}

func nestedView() {
	b := make([]byte, 64)
	heap = append(heap, b)
	n := (*nested)(unsafe.Pointer(&b[0]))
	n.A = -3
	n.B = -300
	n.C[1] = 65535
	n.D = 1.5
	n.E = -2.25
	n.F[1].Y = -7
	n.Pad = true
	m := *n
	fmt.Println(m.A, m.B, m.C, m.D, m.E, m.F, m.Pad)
	fmt.Println(b[0], le16(b[unsafe.Offsetof(n.B):]), b[unsafe.Offsetof(n.Pad)])
	m.A = 100
	*n = m
	fmt.Println(int8(b[0]), n.A)
}

func stringView() {
	s := "hello, world!"
	p := unsafe.StringData(s)
	w := *(*uint32)(unsafe.Pointer(p))
	fmt.Println(w == le32([]byte(s)), *(*uint16)(unsafe.Add(unsafe.Pointer(p), 7)) == le16([]byte(s[7:])))
}

func viaNative() {
	b := make([]byte, 8)
	p := (*pair)(unsafe.Pointer(&b[0]))
	p.A = 0x01020304
	p.B = 7
	// Native code gets the real pointer: json reads the fields in place.
	out, err := json.Marshal(p)
	fmt.Println(string(out), err)
	fmt.Println(le32(b) == p.A, le16(b[4:]) == p.B)
	// Retyped through an unsafe.Pointer of unknown origin (a parameter):
	// still the same bytes, and still the real pointer for native code.
	q := asPair(unsafe.Pointer((*uint64)(unsafe.Pointer(&b[0]))))
	q.B = 9
	out, _ = json.Marshal(q)
	fmt.Println(string(out), p.B, le16(b[4:]))
}

// asPair and asInt8 hide the source type from the SSA.
func asPair(p unsafe.Pointer) *pair { return (*pair)(p) }
func asInt8(p unsafe.Pointer) *int8 { return (*int8)(p) }

// narrow checks the canonical form of one-byte views: an int8 read from a
// byte is sign extended, with the source type known or not.
func narrow() {
	b := []byte{0xff, 0x80, 1, 0}
	fmt.Println(*(*int8)(unsafe.Pointer(&b[0])) < 0, *(*int8)(unsafe.Pointer(&b[1])), *asInt8(unsafe.Pointer(&b[0])) < 0)
	p := (*int8)(unsafe.Pointer(&b[1]))
	*p = -2
	fmt.Println(b[1], *p, *(*bool)(unsafe.Pointer(&b[2])), *(*bool)(unsafe.Pointer(&b[3])))
}

// cells checks that an array of bytes held as cells (indexed, not only
// sliced) can still be seen as a smaller array: the cells are shared.
func cells() {
	var arr [8]byte
	for i := range arr {
		arr[i] = byte(i)
	}
	p := (*[4]byte)(unsafe.Pointer(&arr[1]))
	fmt.Println(p[0], p[3], len(p))
	p[0] = 9
	fmt.Println(arr[1], p[1]+p[2])
}

func main() {
	wordReads()
	header()
	arrays()
	nestedView()
	stringView()
	viaNative()
	narrow()
	cells()
}
