package vm

import (
	"runtime"
	"testing"
)

// TestFuncInlProbe: the start-up probe of the funcinl layout
// passes on this runtime, and a runtime that panics when it reads the
// stand-in makes the probe fail instead of ending the host. Before, the
// probe had no recover: the panic ended every host at start-up.
func TestFuncInlProbe(t *testing.T) {
	if !funcInlOK || !probeFuncInl(readRuntimeFunc) {
		t.Fatal("the runtime does not read the funcinl stand-in")
	}
	var ok bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("the probe let a panic through: %v", r)
			}
		}()
		ok = probeFuncInl(func(*runtime.Func) (string, uintptr, string, int) {
			var tab []int
			i := 3
			return "vm.probe", 42, "probe.go", tab[i] // index out of range, as a runtime with another layout
		})
	}()
	if ok {
		t.Fatal("the probe passed although reading the stand-in panicked")
	}
	if probeFuncInl(func(*runtime.Func) (string, uintptr, string, int) { return "x", 42, "probe.go", 7 }) {
		t.Fatal("the probe passed with a wrong name")
	}
}
