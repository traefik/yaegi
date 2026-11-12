// sync/atomic functions on plain variables: AddInt32/64, AddUint32/64,
// Load*, Store*, Swap*, CompareAndSwap*, And*/Or*, and the Pointer
// functions through a typed wrapper, also on fields and slice elements.
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// The 64-bit fields come first: they must be 64-bit aligned on 32-bit
// platforms for the atomic functions.
type counters struct {
	b int64
	d uint64
	a int32
	c uint32
}

func main() {
	var c counters
	arr := make([]int64, 4)
	var wg sync.WaitGroup
	for g := 0; g < 25; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				atomic.AddInt32(&c.a, 1)
				atomic.AddInt64(&c.b, -2)
				atomic.AddUint32(&c.c, 3)
				atomic.AddUint64(&c.d, 1<<32)
				atomic.AddInt64(&arr[j%4], int64(j))
			}
		}()
	}
	wg.Wait()
	fmt.Println(atomic.LoadInt32(&c.a), atomic.LoadInt64(&c.b), atomic.LoadUint32(&c.c), atomic.LoadUint64(&c.d))
	fmt.Println(arr)

	var x int32 = 10
	fmt.Println(atomic.SwapInt32(&x, 20))
	fmt.Println(x)
	fmt.Println(atomic.CompareAndSwapInt32(&x, 10, 30), atomic.CompareAndSwapInt32(&x, 20, 30))
	fmt.Println(x)
	atomic.StoreInt32(&x, -1)
	fmt.Println(atomic.AddInt32(&x, 1))
	fmt.Println(x)

	var y int64 = 1 << 40
	fmt.Println(atomic.SwapInt64(&y, 5), atomic.CompareAndSwapInt64(&y, 5, 6), atomic.LoadInt64(&y))
	atomic.StoreInt64(&y, 1<<62)
	fmt.Println(atomic.AddInt64(&y, 1<<62))

	var u uint32
	fmt.Println(atomic.AddUint32(&u, ^uint32(0)), atomic.SwapUint32(&u, 7), atomic.CompareAndSwapUint32(&u, 7, 8), atomic.LoadUint32(&u))
	atomic.StoreUint32(&u, 100)
	fmt.Println(u)

	var w uint64
	fmt.Println(atomic.AddUint64(&w, ^uint64(0)), atomic.SwapUint64(&w, 9), atomic.CompareAndSwapUint64(&w, 9, 10), atomic.LoadUint64(&w))
	atomic.StoreUint64(&w, 1<<63)
	fmt.Println(w)

	var up uintptr = 3
	fmt.Println(atomic.AddUintptr(&up, 4), atomic.SwapUintptr(&up, 1), atomic.CompareAndSwapUintptr(&up, 1, 2), atomic.LoadUintptr(&up))
	atomic.StoreUintptr(&up, 50)
	fmt.Println(up)

	// And / Or functions return the old value.
	var m32 int32 = 0x7F
	oldm32 := atomic.AndInt32(&m32, 0x0F)
	fmt.Println(oldm32, m32)
	oldm32 = atomic.OrInt32(&m32, 0x30)
	fmt.Println(oldm32, m32)
	var mu32 uint32 = 0xFFFF
	oldmu32 := atomic.AndUint32(&mu32, 0xFF)
	fmt.Println(oldmu32, mu32)
	oldmu32 = atomic.OrUint32(&mu32, 1<<31)
	fmt.Println(oldmu32, mu32)
	var m64 int64 = -1
	oldm64 := atomic.AndInt64(&m64, 1<<40)
	fmt.Println(oldm64, m64)
	oldm64 = atomic.OrInt64(&m64, 1)
	fmt.Println(oldm64, m64)
	var mu64 uint64 = 1<<64 - 1
	oldmu64 := atomic.AndUint64(&mu64, 0xF0)
	fmt.Println(oldmu64, mu64)
	oldmu64 = atomic.OrUint64(&mu64, 1<<63)
	fmt.Println(oldmu64, mu64)
	var mup uintptr = 0xF
	oldmup := atomic.AndUintptr(&mup, 0x3)
	fmt.Println(oldmup, mup)
	oldmup = atomic.OrUintptr(&mup, 0x10)
	fmt.Println(oldmup, mup)

	// Concurrent Or on bit positions: all bits set at the end.
	var bits uint64
	for g := 0; g < 64; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.OrUint64(&bits, 1<<uint(g))
		}()
	}
	wg.Wait()
	fmt.Printf("%#x\n", bits)

	// Concurrent And clears bits.
	var mask uint32 = 1<<32 - 1
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AndUint32(&mask, ^(uint32(1) << uint(2*g)))
		}()
	}
	wg.Wait()
	fmt.Printf("%#x\n", mask)

	// CAS loop to compute a max.
	var maxv int64
	for g := 0; g < 30; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val := int64((g * 37) % 101)
			for {
				cur := atomic.LoadInt64(&maxv)
				if val <= cur || atomic.CompareAndSwapInt64(&maxv, cur, val) {
					return
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("max", maxv)
}
