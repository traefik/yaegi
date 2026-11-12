package main

import (
	"fmt"
	"sync"
	"unsafe"
)

// Unsafe header views on many goroutines at the same time. copyOf follows
// getBytes of github.com/tidwall/gjson: the data pointer of a string header
// becomes the data pointer of a slice header. The VM finds the memory
// behind that pointer in the regions recently converted to uintptr; when
// the ring of these regions was shared by all goroutines, the other
// goroutines pushed the region out before it was used, and most goroutines
// failed with "reaches outside the memory known to the VM" (found by the
// Coraza plugin benchmark under load).

type stringHeader struct {
	data unsafe.Pointer
	len  int
}

type sliceHeader struct {
	data unsafe.Pointer
	len  int
	cap  int
}

func copyOf(json []byte) string {
	s := *(*string)(unsafe.Pointer(&json))
	h := *(*stringHeader)(unsafe.Pointer(&s))
	sh := sliceHeader{data: h.data, len: h.len, cap: h.len}
	if uintptr(sh.data) == 0 {
		return ""
	}
	return string(*(*[]byte)(unsafe.Pointer(&sh)))
}

// u64At reads 8 bytes as a uint64 through a pointer made from a uintptr,
// as xxhash and freecache do: the VM finds the bytes behind the pointer in
// the recent regions (a typed view of the bytes, byteview.go), which must
// also look in the ring of the goroutine.
//
//go:noinline
func u64At(b []byte, i int) uint64 {
	p := unsafe.Pointer(uintptr(unsafe.Pointer(&b[0])) + uintptr(i))
	return *(*uint64)(p)
}

func main() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	var first any
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					failed++
					if first == nil {
						first = r
					}
					mu.Unlock()
				}
			}()
			for i := 0; i < 2000; i++ {
				b := make([]byte, 10449)
				b[len(b)-1] = byte(i)
				if s := copyOf(b); len(s) != len(b) || s[len(s)-1] != byte(i) {
					panic("bad copy")
				}
				b[len(b)-8] = 1 // little endian: the low byte
				if v := u64At(b, len(b)-8); v != 1|uint64(byte(i))<<56 {
					panic(fmt.Sprintf("bad uint64 %#x", v))
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("goroutines that failed:", failed)
	if first != nil {
		fmt.Println("first failure:", first)
	}
}
