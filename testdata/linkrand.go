package main

// The random and hash functions of the runtime that many modules link to
// with //go:linkname (fastrand, cheaprand, memhash, strhash...). The VM
// gives them with the standard library (vm/linkrand.go). Their values are
// random in gc too (the hash key changes at each start), so the program
// prints only what callers rely on: the range of the random numbers, and
// the same hash for the same bytes and seed. Before, the load failed:
// "the function has no Go body". Go 1.27 refuses to link memhash32 and
// memhash64 ("invalid reference"): the VM still gives them, for modules
// written for older releases (vm/linkrand_test.go).

import (
	"fmt"
	"unsafe"
)

//go:linkname cheaprand runtime.cheaprand
func cheaprand() uint32

//go:linkname cheaprandn runtime.cheaprandn
func cheaprandn(n uint32) uint32

//go:linkname cheaprand64 runtime.cheaprand64
func cheaprand64() int64

//go:linkname fastrand runtime.fastrand
func fastrand() uint32

//go:linkname fastrandn runtime.fastrandn
func fastrandn(n uint32) uint32

//go:linkname fastrand64 runtime.fastrand64
func fastrand64() uint64

//go:linkname rand runtime.rand
func rand() uint64

//go:linkname randn runtime.randn
func randn(n uint32) uint32

//go:linkname memhash runtime.memhash
func memhash(p unsafe.Pointer, h, s uintptr) uintptr

//go:linkname strhash runtime.strhash
func strhash(p unsafe.Pointer, h uintptr) uintptr

func main() {
	inRange := true
	seen := map[uint32]bool{}
	for i := 0; i < 1000; i++ {
		seen[cheaprand()] = true
		seen[fastrand()] = true
		_ = fastrand64()
		_ = rand()
		inRange = inRange && cheaprandn(10) < 10 && fastrandn(3) < 3 && cheaprand64() >= 0 && randn(5) < 5
	}
	fmt.Println("random:", inRange, len(seen) > 1000, cheaprandn(0), fastrandn(1), randn(0))

	s := "hello"
	t := string([]byte("hel")) + "lo"
	h1 := memhash(unsafe.Pointer(unsafe.StringData(s)), 0, uintptr(len(s)))
	h2 := memhash(unsafe.Pointer(unsafe.StringData(t)), 0, 5)
	h3 := memhash(unsafe.Pointer(unsafe.StringData(t)), 1, 5)
	h4 := memhash(unsafe.Pointer(unsafe.StringData(t)), 0, 4)
	b := []byte("hello")
	h5 := memhash(unsafe.Pointer(&b[0]), 0, 5)
	fmt.Println("memhash:", h1 == h2, h1 != h3, h1 != h4, h1 == h5)
	fmt.Println("memhash empty:", memhash(nil, 7, 0) == memhash(unsafe.Pointer(&b[0]), 7, 0))

	fmt.Println("strhash:", strhash(unsafe.Pointer(&s), 0) == strhash(unsafe.Pointer(&t), 0),
		strhash(unsafe.Pointer(&s), 0) != strhash(unsafe.Pointer(&s), 9))
	e1, e2 := "", s[5:]
	fmt.Println("strhash empty:", strhash(unsafe.Pointer(&e1), 3) == strhash(unsafe.Pointer(&e2), 3))
}
