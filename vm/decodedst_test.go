package vm

import (
	"strings"
	"testing"
)

// TestDecodeDstFuncs checks that each native decoder of decodeDsts that is
// a function (not a method) has its func value in decodeDstFuncs, so that a
// call through a func value finds its destination too (decodeDstPtrs).
func TestDecodeDstFuncs(t *testing.T) {
	for name := range decodeDsts {
		if strings.HasPrefix(name, "(") {
			continue // a method: called through a bound method wrapper of the VM
		}
		if _, ok := decodeDstFuncs[name]; !ok {
			t.Errorf("%s: missing in decodeDstFuncs", name)
		}
	}
	for name := range decodeDstFuncs {
		if _, ok := decodeDsts[name]; !ok {
			t.Errorf("%s: in decodeDstFuncs but not in decodeDsts", name)
		}
	}
	if len(decodeDstPtrs) != len(decodeDstFuncs) {
		t.Errorf("decodeDstPtrs has %d entries, want %d (two functions share a code pointer)", len(decodeDstPtrs), len(decodeDstFuncs))
	}
}
