// int, uint and uintptr have the size of the platform: 32 bits on 386 and
// arm, 64 bits on amd64 and arm64. The output is compared with go run on
// each platform, so every line checks the VM against gc there.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"strconv"
	"sync/atomic"
	"unsafe"
)

type rec struct {
	A int8
	B int
	C uint
	D uintptr
	E int64
}

//go:noinline
func id(x int) int { return x }

//go:noinline
func uid(x uint) uint { return x }

func main() {
	// Sizes and constants.
	fmt.Println("IntSize", strconv.IntSize, bits.UintSize)
	fmt.Println("MaxInt", math.MaxInt, math.MinInt, uint(math.MaxUint))
	fmt.Println("Sizeof", unsafe.Sizeof(0), unsafe.Sizeof(uint(0)), unsafe.Sizeof(uintptr(0)), unsafe.Sizeof(rec{}), unsafe.Sizeof([]int{}), unsafe.Sizeof(""))
	fmt.Println("Alignof", unsafe.Alignof(rec{}.E), unsafe.Alignof(rec{}.B))
	fmt.Println("Offsetof", unsafe.Offsetof(rec{}.B), unsafe.Offsetof(rec{}.C), unsafe.Offsetof(rec{}.D), unsafe.Offsetof(rec{}.E))
	fmt.Println("reflect", reflect.TypeOf(0).Size(), reflect.TypeOf(0).Kind(), reflect.TypeOf(rec{}).Size(), reflect.TypeOf(uintptr(0)).Bits())

	// Arithmetic wraps at the size of int.
	s := 0
	for i := 0; i < 10000000; i++ {
		s += i
	}
	fmt.Println("sum", s)
	m := id(math.MaxInt)
	fmt.Println("add", m+1, m+m, -m-2)
	fmt.Println("sub", id(math.MinInt)-1)
	fmt.Println("mul", m*3, id(65536)*id(65536), id(1000003)*id(1000003))
	fmt.Println("neg", -id(math.MinInt), ^id(math.MaxInt))
	fmt.Println("div", id(math.MinInt)/id(-1), id(math.MinInt)%id(-1))
	fmt.Println("shl", id(1)<<id(31), id(1)<<id(32), id(3)<<id(62), id(1)<<id(63), id(1)<<id(64))
	fmt.Println("shr", id(-1)>>id(40), id(math.MinInt)>>id(1))

	// uint and uintptr.
	u := uid(0) - 1
	fmt.Println("uint", u, u+2, u*u, u>>1, u/3, u%7, u > uid(1<<31), uid(1)<<id(32))
	p := uintptr(uid(0)) - 1
	fmt.Println("uintptr", p, p+2, p>>4)
	var ap uintptr = 5
	atomic.AddUintptr(&ap, ^uintptr(0))
	fmt.Println("atomic", ap)
	fmt.Println("atomic", atomic.AddUintptr(&ap, ^uintptr(4)))

	// Conversions.
	big := int64(1)<<40 + 7
	neg := int64(-1) << 35
	fmt.Println("int64->int", int(big), int(neg), uint(big), uintptr(big))
	fmt.Println("int->uint", uint(id(-1)), uint64(uint(id(-1))), int64(uint(id(-2))))
	fmt.Println("int->int64", int64(id(-5)), uint64(id(-5)))
	f := 3e9
	fmt.Println("float->int", int(float64(id(2000000000))), uint(f), int64(f))
	fmt.Println("int->float", float64(m), float64(u), float32(u))

	// Values crossing to and from native code.
	fmt.Println("native", strconv.Itoa(m+1), strconv.FormatUint(uint64(u), 10), fmt.Sprint(m+1, u+1))
	r := rec{A: -1, B: m + 1, C: u, D: p, E: big}
	b, _ := json.Marshal(r)
	fmt.Println("json", string(b))
	var back rec
	_ = json.Unmarshal(b, &back)
	fmt.Println("back", back == r, back.B, back.C)
	n, err := strconv.Atoi("4294967296")
	fmt.Println("Atoi", n, err != nil)
	rv := reflect.ValueOf(&r).Elem()
	rv.Field(1).SetInt(big)
	fmt.Println("SetInt", r.B, rv.Field(1).OverflowInt(big))
	// Compare the value: fmt boxes it for native code, which hides a value
	// that was not cut to the size of int.
	fmt.Println("SetInt value", r.B == int(big), reflect.ValueOf(r.B).Int(), map[int]bool{int(big): true}[r.B])
	rv.Field(2).SetUint(uint64(big))
	fmt.Println("SetUint value", r.C == uint(big), reflect.ValueOf(r.C).Uint())

	// len and cap of slices with int arithmetic on them.
	xs := make([]int, 3, 10)
	fmt.Println("len", len(xs), cap(xs), len(xs)-cap(xs), uint(len(xs)-cap(xs)))
	// The len and cap words of a slice header: one word is the size of int.
	hs := make([]byte, 3, 10)
	h := (*[3]int)(unsafe.Pointer(&hs))
	lp := (*int)(unsafe.Add(unsafe.Pointer(&hs), unsafe.Sizeof(uintptr(0))))
	cp := (*int)(unsafe.Add(unsafe.Pointer(&hs), 2*unsafe.Sizeof(0)))
	fmt.Println("header", h[1], h[2], *lp, *cp)
	mp := map[int]string{m + 1: "min"}
	fmt.Println("map", mp[math.MinInt], len(mp))

	// make with an int64 length or capacity past the range of int: gc
	// panics on 32 bits (makeslice64), and makes the slice on 64 bits.
	big64 := int64(1)<<32 + 4
	for _, f := range []func() int{
		func() int { return len(make([]struct{}, big64)) },
		func() int { return cap(make([]struct{}, 1, big64)) },
		func() int { return len(make([]struct{}, -big64<<8)) },
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println("make64 panic:", r)
				}
			}()
			fmt.Println("make64", f())
		}()
	}
}
