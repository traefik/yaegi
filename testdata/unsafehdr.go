package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

// Views of string and slice headers through unsafe (docs/UNSAFE.md): the
// reflect headers, local structs with the same layout, the zero-copy
// conversions of byte slices and strings, and addresses of the data.

type sliceHdr struct {
	data     unsafe.Pointer
	len, cap int
}

type strHdr struct {
	data unsafe.Pointer
	n    int
}

// b2s and s2b are the old zero-copy idioms of fasthttp and others.
func b2s(b []byte) (s string) {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	sh.Data = bh.Data
	sh.Len = bh.Len
	return s
}

func s2b(s string) (b []byte) {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bh.Data = sh.Data
	bh.Len = sh.Len
	bh.Cap = sh.Len
	return b
}

// s2bLocal does the same with local header types and pointer fields.
func s2bLocal(s string) []byte {
	var b []byte
	h := (*sliceHdr)(unsafe.Pointer(&b))
	h.data = (*strHdr)(unsafe.Pointer(&s)).data
	h.cap = len(s)
	h.len = len(s)
	return b
}

func b2sCast(b []byte) string { return *(*string)(unsafe.Pointer(&b)) }

func s2bCast(s string) []byte { return *(*[]byte)(unsafe.Pointer(&s)) }

// header gets the header through an unsafe.Pointer of unknown origin.
func header(p unsafe.Pointer) *reflect.StringHeader { return (*reflect.StringHeader)(p) }

func dataOf(s string) uintptr { return *(*uintptr)(unsafe.Pointer(&s)) }

func main() {
	// Zero-copy: the string sees later writes into the bytes, as in gc.
	b := make([]byte, 5)
	copy(b, "hello")
	s1, s2 := b2s(b), b2sCast(b)
	b[0] = 'J'
	fmt.Println(s1, s2, len(s1), len(s2))
	fmt.Println(string(s2b("world")), string(s2bLocal("local")), len(s2bLocal("abc")), cap(s2b("abcd")))
	c := s2bCast("cast")
	fmt.Println(string(c), len(c))
	u := unsafe.String(unsafe.SliceData(b), len(b))
	b[1] = 'E'
	fmt.Println(u, string(unsafe.Slice(unsafe.StringData(u), 3)))

	// Reading the words of a header.
	s := "a string of text"
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	fmt.Println(sh.Len, sh.Data == uintptr(unsafe.Pointer(unsafe.StringData(s))), dataOf(s) == sh.Data)
	fmt.Println(unsafe.Pointer(sh.Data) == unsafe.Pointer(unsafe.StringData(s)), header(unsafe.Pointer(&s)).Len)
	fmt.Println(*(*int)(unsafe.Pointer(uintptr(unsafe.Pointer(&s)) + unsafe.Sizeof(uintptr(0)))))
	copyh := *(*reflect.StringHeader)(unsafe.Pointer(&s))
	fmt.Println(copyh.Len, copyh.Data == sh.Data)

	// Writing the words of a header re-slices the same memory.
	sh.Len = 8
	fmt.Println(s)
	sh.Data += 2
	sh.Len = 6
	fmt.Println(s)
	sub := "a string of text"[9:11]
	fmt.Println(dataOf(sub) == dataOf(string([]byte(sub))))

	x := []int{10, 20, 30, 40, 50}
	h := (*reflect.SliceHeader)(unsafe.Pointer(&x))
	fmt.Println(h.Len, h.Cap, h.Data == uintptr(unsafe.Pointer(&x[0])), *(*int)(unsafe.Pointer(h.Data)))
	h.Len = 2
	fmt.Println(x, len(x), cap(x))
	h.Len = 5
	h.Cap = 4
	h.Len = 4
	fmt.Println(x, len(x), cap(x))
	words := (*[3]int)(unsafe.Pointer(&x))
	fmt.Println(words[1], words[2])
	words[1] = 3
	fmt.Println(x)
	data := unsafe.SliceData(x)
	fmt.Println(unsafe.Pointer(h.Data) == unsafe.Pointer(data), &unsafe.Slice(data, 2)[1] == &x[1])

	// A slice header built by the program.
	backing := make([]byte, 8)
	copy(backing, "abcdefgh")
	var built []byte
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&built))
	bh.Len = 3
	bh.Cap = 6
	bh.Data = uintptr(unsafe.Pointer(&backing[2]))
	fmt.Println(string(built), len(built), cap(built))
	built = append(built, 'X')
	fmt.Println(string(backing))

	// unsafe.Slice and unsafe.String.
	var arr [6]byte
	p1 := unsafe.Pointer(&arr[1])
	fmt.Println(unsafe.Add(p1, 1) == unsafe.Pointer(&arr[2]), unsafe.Add(p1, -1) == unsafe.Pointer(&arr[0]))
	whole := unsafe.Slice(&arr[0], len(arr))
	whole[5] = 'z'
	fmt.Println(arr[5], &whole[0] == &arr[0], unsafe.String(&arr[5], 1))
	fmt.Println(unsafe.Slice((*int)(nil), 0) == nil, unsafe.String(nil, 0) == "")
	for _, f := range []func(){
		func() { _ = unsafe.Slice((*int)(nil), 1) },
		func() { n := -1; _ = unsafe.Slice(new(byte), n) },
		func() { var n uint64 = 1<<64 - 1; _ = unsafe.String(new(byte), n) },
		func() { _ = unsafe.Slice(new(uint64), 1<<61) },
	} {
		func() {
			defer func() { fmt.Println(recover()) }()
			f()
		}()
	}

	// Addresses that do not come from memory: kept, compared, printed.
	addr := uintptr(0x234)
	q1 := (*int)(unsafe.Pointer(addr))
	q2 := (*int)(unsafe.Pointer(uintptr(0x234)))
	fmt.Println(q1 == q2, q1 != nil, uintptr(unsafe.Pointer(q1)) == addr)
}
