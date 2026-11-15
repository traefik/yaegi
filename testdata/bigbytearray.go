package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// A [n]byte variable whose address escapes (to an interface, a pool, a
// pointer) lives in real bytes from n >= 64 (bigByteArray in byteview.go).
// In cells, each native call on a slice of a 4 MiB array copied the whole
// array: the lz4 compressor of pierrec/lz4, which takes its block buffers
// from a sync.Pool of *[4 << 20]byte and calls binary.LittleEndian.Uint32 on
// them in a loop, took more than a minute per call of Write. The array must
// keep the semantics of an array: copies, comparison, map keys, range,
// arguments by value, methods of a named type, reflect and encoding.

type block [64]byte

func (b *block) fill(v byte) {
	for i := range b {
		b[i] = v
	}
}

func (b block) sum() int {
	s := 0
	for _, v := range b {
		s += int(v)
	}
	return s
}

var pool = sync.Pool{New: func() any { return &[4 << 20]byte{} }}

func byValue(a [100]byte) [100]byte {
	a[0] = 42
	return a
}

func main() {
	// A big array through an interface, then sliced: native calls on the
	// slice share the array and do not copy it.
	var boxed any = &[4 << 20]byte{}
	s := boxed.(*[4 << 20]byte)[:]
	binary.LittleEndian.PutUint32(s[10:], 0xdeadbeef)
	start := time.Now()
	var x uint32
	for i := 0; i < 1000; i++ {
		x += binary.LittleEndian.Uint32(s[10:])
	}
	fmt.Println("boxed:", x, time.Since(start) < 2*time.Second)
	fmt.Println("shared:", boxed.(*[4 << 20]byte)[10] == 0xef)

	// The same through a sync.Pool.
	p := pool.Get().(*[4 << 20]byte)
	buf := p[:]
	copy(buf, "hello pool")
	fmt.Println("pool:", string(buf[:10]), bytes.Index(buf[:64], []byte("pool")), len(buf), cap(buf))
	pool.Put(p)

	// Copies are independent.
	a := &[128]byte{1, 2, 3}
	b := *a
	b[0] = 9
	fmt.Println("copy:", a[0], b[0], a[1], b[1])
	*a = b
	fmt.Println("assign:", a[0])

	// Comparison and map keys.
	var k1, k2 [64]byte
	k1[63] = 1
	k2[63] = 1
	pk := &k1
	fmt.Println("equal:", k1 == k2, *pk == k2)
	m := map[[64]byte]int{k1: 1}
	m[k2]++
	fmt.Println("map:", len(m), m[k1])

	// Range and arguments by value.
	var big [100]byte
	for i := range big {
		big[i] = byte(i)
	}
	pb := &big
	out := byValue(*pb)
	fmt.Println("byvalue:", big[0], out[0], out[99])
	n := 0
	for _, v := range pb {
		n += int(v)
	}
	fmt.Println("range:", n)

	// A named type with methods.
	var blk block
	blk.fill(2)
	var ib any = &blk
	fmt.Println("named:", ib.(*block).sum(), blk.sum())

	// reflect reads and writes the array.
	rv := reflect.ValueOf(&big).Elem()
	rv.Index(5).SetUint(200)
	fmt.Println("reflect:", big[5], rv.Index(6).Uint(), rv.Len())
	var other [100]byte
	other[7] = 77
	rv.Set(reflect.ValueOf(other))
	fmt.Println("reflect set:", big[5], big[7], rv.Interface().([100]byte)[7])
	fmt.Println("reflect deepequal:", reflect.DeepEqual(big, other), reflect.DeepEqual(&big, &other))
	big[8] = 88
	fmt.Println("reflect slice:", rv.Slice(6, 9).Bytes(), len(rv.Bytes()), rv.CanAddr())

	var dst [64]byte
	err := json.Unmarshal([]byte("[1,2,3]"), &dst)
	fmt.Println("json unmarshal:", err, dst[:4])

	// fmt and encoding/json see an array.
	small := &[64]byte{1, 2}
	fmt.Printf("fmt: %v %T\n", small[:4], small)
	j, err := json.Marshal(small)
	fmt.Println("json:", len(j), err, string(j[:8]))
}
