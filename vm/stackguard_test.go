//go:build !wasm

package vm

import (
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
)

// TestStackGuard checks that the stack bounds read from the goroutine
// structure are right on this platform (else the guard of RV.Elem is off
// and an AOT walk of a value nested very deep ends the host again), and
// that checkStack panics with errStackTooDeep near the stack limit.
func TestStackGuard(t *testing.T) {
	if !stackGuardOK {
		t.Fatal("stackBoundsOK is false: the layout of the goroutine structure changed")
	}
	if u := stackUsed(); u == 0 || u > 1<<20 {
		t.Fatalf("stackUsed = %d at the start of a test goroutine", u)
	}
	done := make(chan any)
	go func() {
		defer func() { done <- recover() }()
		var deep func(n int) int
		deep = func(n int) int {
			var pad [1024]byte
			checkStack()
			pad[n%len(pad)] = byte(n)
			return deep(n+1) + int(pad[0])
		}
		deep(0)
	}()
	r := <-done
	if err, _ := r.(error); err == nil || !strings.Contains(err.Error(), "nested too deep") {
		t.Fatalf("recover() = %v, want errStackTooDeep", r)
	}
}

// TestStackGuardMax: the guard is at 127/128 of the largest stack that the
// runtime allows (512 MiB by default on 64-bit platforms, 128 MiB on
// 32-bit), and it follows the limit that the host sets with
// runtime/debug.SetMaxStack (read again at each NewMachine), which stays
// as the host set it.
func TestStackGuardMax(t *testing.T) {
	want := uintptr(512<<20) / 128 * 127
	if strconv.IntSize == 32 {
		want = (128 << 20) / 128 * 127
	}
	if got := stackGuardMax.Load(); got != want {
		t.Fatalf("stackGuardMax = %d, want %d", got, want)
	}
	old := debug.SetMaxStack(64 << 20)
	defer func() {
		debug.SetMaxStack(old)
		setStackGuard()
	}()
	setStackGuard()
	if got := stackGuardMax.Load(); got != (64<<20)/128*127 {
		t.Fatalf("stackGuardMax = %d after SetMaxStack(64 MiB), want %d", got, (64<<20)/128*127)
	}
	if limit := debug.SetMaxStack(64 << 20); limit != 64<<20 {
		t.Fatalf("SetMaxStack: the limit of the host is %d after the read, want %d", limit, 64<<20)
	}
}
