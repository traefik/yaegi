// What gc does not allocate, the VM does not allocate either, for the
// cases below (GOROOT test closure.go): a native call whose arguments are
// native pointers or native values, a field of a native struct, and the
// first call of a function of the program (compiled before main runs).
package main

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

type state struct {
	mu    sync.Mutex
	stats runtime.MemStats
	n     uint64
}

var s state

func readStats() { runtime.ReadMemStats(&s.stats) }

func lockUnlock() {
	s.mu.Lock()
	s.n++
	s.mu.Unlock()
}

func field() { s.n += s.stats.Mallocs + s.stats.Frees }

var d time.Duration

func nativeValue() { d += time.Duration(s.n) }

func adder1() func(int) int { return func(x int) int { return x + 1 } }
func adder2() func(int) int { return func(x int) int { return x + 2 } }
func adder3() func(int) int { return func(x int) int { return x + 3 } }

func deep(n int) int {
	if n == 0 {
		return 0
	}
	return deep(n-1) + 1
}

func main() {
	ms := new(runtime.MemStats)
	fmt.Println("ReadMemStats:", testing.AllocsPerRun(100, func() { runtime.ReadMemStats(ms) }))
	fmt.Println("ReadMemStats of a field:", testing.AllocsPerRun(100, readStats))
	fmt.Println("Mutex:", testing.AllocsPerRun(100, lockUnlock))
	fmt.Println("native fields:", testing.AllocsPerRun(100, field))
	fmt.Println("native value:", testing.AllocsPerRun(100, nativeValue))

	// The first call of an adder and of its literal. The register stack of
	// the VM grows at the first call that goes deeper than before, as the
	// stack of a goroutine in gc, but it is heap memory: deep makes it
	// large enough first, so that only the compiler could allocate here.
	// Mallocs counts the whole process, and another goroutine of the host
	// can allocate between the two reads (seen under qemu, much slower):
	// the first calls of three functions, the smallest count.
	deep(20)
	least, ok := ^uint64(0), true
	for i, adder := range []func() func(int) int{adder1, adder2, adder3} {
		runtime.ReadMemStats(ms)
		n0 := ms.Mallocs
		f, g := adder(), adder()
		ok = ok && f(1) == 2+i && g(2) == 3+i
		runtime.ReadMemStats(ms)
		least = min(least, ms.Mallocs-n0)
	}
	fmt.Println("first call:", least, ok)
}
