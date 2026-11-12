// Plain data seen as another type through unsafe: the variable lives in
// real bytes (vm/byteview.go, bytesAlloc).
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

type hdr struct {
	A uint32
	B uint16
	C uint16
}

// entryHdr is the entry header of coocood/freecache, laid over a local
// [24]byte buffer.
type entryHdr struct {
	accessTime uint32
	expireAt   uint32
	keyLen     uint16
	hash16     uint16
	valLen     uint32
	valCap     uint32
	deleted    bool
	slotId     uint8
	reserved   uint16
}

type counter struct {
	n    uint64
	tags [8]byte
}

func main() {
	// A local [8]byte read as a uint64.
	var a [8]byte
	a[0], a[7] = 1, 2
	u := *(*uint64)(unsafe.Pointer(&a))
	fmt.Println(u == 1|2<<56 || u == 1<<56|2, a[:], len(a))

	// A struct seen as bytes, both ways.
	h := hdr{1, 2, 3}
	b := (*[8]byte)(unsafe.Pointer(&h))
	fmt.Println(b[:], unsafe.Slice((*byte)(unsafe.Pointer(&h)), unsafe.Sizeof(h)))
	b[4] = 9
	fmt.Println(h.B, h, &h)
	h2 := h // a copy is a new value
	h2.A = 7
	fmt.Println(h.A, h2.A)

	// float32 bits, uint16 as [2]byte.
	f := float32(1.5)
	bits := (*uint32)(unsafe.Pointer(&f))
	fmt.Printf("%x %v\n", *bits, *bits == math.Float32bits(f))
	*bits = math.Float32bits(-2.25)
	fmt.Println(f)
	x := uint16(0x0102)
	xb := (*[2]byte)(unsafe.Pointer(&x))
	fmt.Println(xb[0] == byte(x), xb[1] == byte(x>>8))

	// freecache: write a header into a local buffer, copy it into a ring,
	// read it back through another buffer.
	ring := make([]byte, 64)
	var wbuf [24]byte
	w := (*entryHdr)(unsafe.Pointer(&wbuf[0]))
	w.keyLen, w.valLen, w.expireAt, w.deleted = 3, 5, 99, true
	copy(ring[16:], wbuf[:])
	var rbuf [24]byte
	copy(rbuf[:], ring[16:16+24])
	r := (*entryHdr)(unsafe.Pointer(&rbuf[0]))
	fmt.Println(r.keyLen, r.valLen, r.expireAt, r.deleted, *r == *w)

	// A variable in bytes that escapes: a closure, a heap pointer, atomic
	// operations from several goroutines.
	c := &counter{}
	tags := (*uint64)(unsafe.Pointer(&c.tags))
	*tags = 0x0807060504030201
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				atomic.AddUint64(&c.n, 1)
			}
		}()
	}
	wg.Wait()
	inc := func() { c.tags[0]++ }
	inc()
	fmt.Println(atomic.LoadUint64(&c.n), c.tags)

	// A variable in bytes is still read by reflect, json and fmt, sliced,
	// copied and compared like any other (before: reflect panicked with a
	// nil pointer and a[1:3] was refused).
	var rh struct {
		A uint32
		B uint16
		C uint16
	}
	rhb := (*[8]byte)(unsafe.Pointer(&rh))
	rh.A, rh.B = 3, 4
	rv := reflect.ValueOf(&rh).Elem()
	js, jerr := json.Marshal(&rh)
	fmt.Println(rv.Field(0).Uint(), rv.Field(1).Uint(), reflect.ValueOf(rh).Field(1).Uint(), rhb[4], string(js), jerr)
	jerr = json.Unmarshal([]byte(`{"A":7,"B":8,"C":9}`), &rh)
	rh2 := rh
	rh2.C = 1
	fmt.Println(rh, rhb[0], rh == rh2, jerr)
	var arr [4]uint16
	ab := (*[8]byte)(unsafe.Pointer(&arr))
	for i := range arr {
		arr[i] = uint16(i * 257)
	}
	s := arr[1:3]
	s[0] = 9
	s = append(s, 10)
	fmt.Println(arr, ab[2], len(s), cap(s), arr[1:1], arr[:0:0])
}
