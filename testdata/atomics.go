package main

// sync/atomic on interpreted variables, struct fields and slice elements,
// from many goroutines (run under -race by TestTestdataRace).

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

type counters struct {
	hits  int64
	bytes uint64
	flags uint32
}

type item struct{ name string }

func main() {
	var n int64
	var u uint32 = 1<<32 - 50 // wraps around
	var i32 int32 = -1 << 31  // wraps around downwards
	var c counters
	slots := make([]int32, 4)
	var max int64
	var ptr unsafe.Pointer
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				atomic.AddInt64(&n, 1)
				atomic.AddUint32(&u, 1)
				atomic.AddInt32(&i32, -1)
				atomic.AddInt64(&c.hits, 1)
				atomic.AddUint64(&c.bytes, 3)
				atomic.AddInt32(&slots[j%4], 1)
				atomic.OrUint32(&c.flags, 1<<uint(g%8))
				for v := int64(g*100 + j); ; {
					old := atomic.LoadInt64(&max)
					if v <= old || atomic.CompareAndSwapInt64(&max, old, v) {
						break
					}
				}
				it := &item{fmt.Sprint(g)}
				atomic.StorePointer(&ptr, unsafe.Pointer(it))
				if p := atomic.LoadPointer(&ptr); p == nil || (*item)(p).name == "" {
					panic("bad pointer")
				}
			}
		}(g)
	}
	wg.Wait()
	fmt.Println(atomic.LoadInt64(&n), atomic.LoadUint32(&u), atomic.LoadInt32(&i32))
	fmt.Println(c.hits, c.bytes, c.flags, slots, max)

	old := atomic.SwapInt64(&n, 7)
	cas1 := atomic.CompareAndSwapInt64(&n, 6, 8)
	cas2 := atomic.CompareAndSwapInt64(&n, 7, 8)
	fmt.Println(old, cas1, cas2, n)
	oldFlags := atomic.AndUint32(&c.flags, 0x0f)
	fmt.Println(oldFlags, c.flags)
	a, b := &item{"a"}, &item{"b"}
	atomic.StorePointer(&ptr, unsafe.Pointer(a))
	fmt.Println(atomic.CompareAndSwapPointer(&ptr, unsafe.Pointer(b), nil), atomic.CompareAndSwapPointer(&ptr, unsafe.Pointer(a), unsafe.Pointer(b)))
	fmt.Println((*item)(atomic.SwapPointer(&ptr, nil)).name, atomic.LoadPointer(&ptr) == nil)
}
