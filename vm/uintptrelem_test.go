package vm

import (
	"math"
	"testing"
)

// TestUintptrArrayElem checks that uintptr(p) of an element of an array
// stored as cells is base + offset, as in gc: a loop of
// unsafe.Pointer(uintptr(x) + 1) and - 1 then finds the region it recorded
// before and allocates nothing (GOROOT test fixedbugs/issue36516.go).
func TestUintptrArrayElem(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates on its own: the counts mean nothing")
	}
	const src = `package p

import "unsafe"

var buf [2]byte
var x unsafe.Pointer = unsafe.Pointer(&buf[0])

func Loop() {
	for i := 0; i < 1000; i++ {
		x = unsafe.Pointer(uintptr(x) + 1)
		x = unsafe.Pointer(uintptr(x) - 1)
	}
}

func Same() bool { return x == unsafe.Pointer(&buf[0]) }
`
	m, spkg := buildSrc(t, src)
	m.Call(m.funcOf(spkg.Func("init")), nil, nil)
	loop := m.funcOf(spkg.Func("Loop"))
	m.Call(loop, nil, nil)
	n := math.Inf(1)
	for range 3 {
		n = min(n, testing.AllocsPerRun(10, func() { m.Call(loop, nil, nil) }))
	}
	// A call from the host starts with an empty ring of regions: a few
	// allocations per call, none per round of the loop.
	if n > 50 {
		t.Errorf("allocations for 1000 rounds: %v, want at most 50", n)
	}
	if r := m.Call(m.funcOf(spkg.Func("Same")), nil, nil); r.N != 1 {
		t.Errorf("x is not &buf[0] after the loop: %v", r)
	}
}
