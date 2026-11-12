package vm

import (
	"runtime"
	"testing"
)

// TestRuntimeCouplings: the VM relies on details of the Go
// runtime that no API promises. Each one has a probe that runs when package
// vm starts; when the probe fails, the VM turns the feature off or refuses
// it (fail closed) instead of reading memory with a wrong layout. A host on
// a new Go release keeps working, but loses the feature without a word: this
// test makes the change loud. When it fails after a Go update, read the
// comment of the probe, fix the mirror type for the new runtime, and keep
// the probe.
//
// The other couplings fail at build time: //go:linkname of runtime.nanotime
// and runtime.getAuxv (linkname.go) and runtime.findObject (fatal.go: the
// linker refuses a pull of a symbol that the runtime no longer exports), and
// the generated data (release_guard.go of stdtypes, stdsyms and aot/all).
func TestRuntimeCouplings(t *testing.T) {
	probes := []struct {
		name string
		ok   bool
		what string
	}{
		{"funcInlOK", funcInlOK, "runtime.funcinl layout (funcpc.go): runtime.FuncForPC of interpreted functions gives nil"},
		{"framesOK", framesOK, "runtime.Frames layout (callers.go): runtime.CallersFrames of interpreted frames loses them"},
		{"embedFSLayout", embedFSLayout(), "embed.FS layout (embed.go): //go:embed of an embed.FS is refused"},
		{"inHeapOK", inHeapOK, "runtime.AddCleanup on non-heap memory (unsafe.go): unsafe views keep strong references, string bytes are read-only"},
		{"cellStartOK", cellStartOK, "runtime.findObject and the malloc header (fatal.go): with PanicOnFatal, runtime.SetFinalizer on a pointer inside an object still ends the host"},
	}
	if runtime.GOARCH != "wasm" {
		probes = append(probes, struct {
			name string
			ok   bool
			what string
		}{"stackGuardOK", stackGuardOK, "stack bounds of the goroutine structure (stackguard_g.go): the stack guard of deep AOT walks is off"})
	}
	for _, p := range probes {
		if !p.ok {
			t.Errorf("%s is false on %s: %s", p.name, runtime.Version(), p.what)
		}
	}
}

// TestInHeapProbe: the probe of inHeap passes on this runtime, and fails
// (without a panic) when AddCleanup would tell a global or a string
// constant from the heap in another way.
func TestInHeapProbe(t *testing.T) {
	if !probeInHeap(cleanupInHeap[byte]) {
		t.Fatal("the AddCleanup test of inHeap does not tell heap, global and constant memory apart")
	}
	heap := new(int)
	if !inHeap(heap) || inHeap(&inHeapProbeGlobal[0]) {
		t.Fatal("inHeap is wrong for a heap object or a global")
	}
	for name, test := range map[string]func(*byte) bool{
		"always true":  func(*byte) bool { return true }, // AddCleanup accepts every pointer
		"always false": func(*byte) bool { return false },
		"panics":       func(*byte) bool { panic("new runtime") },
	} {
		if probeInHeap(test) {
			t.Errorf("probe passed with a test that is %s", name)
		}
	}
}
