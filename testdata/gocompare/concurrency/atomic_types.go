// sync/atomic types: Int32, Int64, Uint32, Uint64, Uintptr, Bool, Value and
// Pointer[T], with Add, Load, Store, Swap, CompareAndSwap, And and Or, used
// from many goroutines (final values only), and wrap around of unsigned types.
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type config struct {
	name    string
	version int
}

func main() {
	var i32 atomic.Int32
	var i64 atomic.Int64
	var u32 atomic.Uint32
	var u64 atomic.Uint64
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				i32.Add(1)
				i64.Add(int64(j))
				u32.Add(2)
				u64.Add(1 << 33)
			}
		}()
	}
	wg.Wait()
	fmt.Println(i32.Load(), i64.Load(), u32.Load(), u64.Load())

	fmt.Println("Swap", i32.Swap(-5), i32.Load())
	fmt.Println("CAS", i32.CompareAndSwap(-5, 7), i32.CompareAndSwap(-5, 8), i32.Load())
	i32.Store(1<<31 - 1)
	fmt.Println("wrap int32", i32.Add(1))
	i64.Store(-1 << 63)
	fmt.Println("wrap int64", i64.Add(-1))
	u32.Store(0)
	fmt.Println("u32 dec", u32.Add(^uint32(0)))
	u64.Store(5)
	fmt.Println("u64 sub 3", u64.Add(^uint64(3-1)))

	// And / Or return the old value.
	var a32 atomic.Uint32
	a32.Store(0xF0F0)
	fmt.Printf("And old=%#x ", a32.And(0xFF00))
	fmt.Printf("new=%#x\n", a32.Load())
	fmt.Printf("Or old=%#x ", a32.Or(0x000F))
	fmt.Printf("new=%#x\n", a32.Load())
	var a64 atomic.Int64
	a64.Store(-1)
	fmt.Println("And64", a64.And(0x0F), a64.Load(), "Or64", a64.Or(0x30), a64.Load())
	var ai32 atomic.Int32
	ai32.Store(6)
	fmt.Println(ai32.And(3), ai32.Load(), ai32.Or(8), ai32.Load())
	var au64 atomic.Uint64
	au64.Store(1 << 40)
	fmt.Println(au64.Or(1), au64.And(1), au64.Load())

	var up atomic.Uintptr
	up.Store(10)
	fmt.Println("uintptr", up.Add(5), up.Swap(1), up.CompareAndSwap(1, 2), up.Load())

	// Bool.
	var b atomic.Bool
	fmt.Println("bool", b.Load(), b.Swap(true), b.Load(), b.CompareAndSwap(false, true), b.CompareAndSwap(true, false), b.Load())
	var winners atomic.Int32
	var flag atomic.Bool
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if flag.CompareAndSwap(false, true) {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	fmt.Println("one winner", winners.Load(), flag.Load())

	// Value.
	var v atomic.Value
	fmt.Println("empty value", v.Load())
	v.Store(config{"a", 1})
	fmt.Printf("%+v\n", v.Load().(config))
	old := v.Swap(config{"b", 2})
	fmt.Printf("swap old %+v new %+v\n", old, v.Load())
	fmt.Println("CAS", v.CompareAndSwap(config{"b", 2}, config{"c", 3}), v.CompareAndSwap(config{"b", 2}, config{"d", 4}))
	fmt.Printf("%+v\n", v.Load())
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		v.Store("a string")
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		v.Store(nil)
	}()
	var v2 atomic.Value
	fmt.Println("CAS on empty", v2.CompareAndSwap(nil, 5), v2.Load())

	// Pointer[T].
	var p atomic.Pointer[config]
	fmt.Println("nil pointer", p.Load() == nil)
	c1 := &config{"one", 1}
	c2 := &config{"two", 2}
	p.Store(c1)
	fmt.Println(p.Load().name, p.CompareAndSwap(c2, c1), p.CompareAndSwap(c1, c2), p.Load().name)
	prev := p.Swap(nil)
	fmt.Println(prev.name, p.Load() == nil)

	// Pointer updated by many goroutines with a CAS loop.
	p.Store(&config{"count", 0})
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				for {
					cur := p.Load()
					next := &config{cur.name, cur.version + 1}
					if p.CompareAndSwap(cur, next) {
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println(p.Load().name, p.Load().version)

	// Atomic fields in a struct.
	type stats struct {
		hits, misses atomic.Int64
		closed       atomic.Bool
	}
	var s stats
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if j%4 == 0 {
					s.misses.Add(1)
				} else {
					s.hits.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	s.closed.Store(true)
	fmt.Println(s.hits.Load(), s.misses.Load(), s.closed.Load())
}
