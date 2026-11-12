package vm

import (
	"testing"

	"github.com/traefik/yaegi/v2/internal/stdsrc"
	"github.com/traefik/yaegi/v2/stdtypes"
)

// TestStdSourceList: the standard library packages that the VM runs from
// source (internal/stdsrc, the one list of the VM and of cmd/stdexport) all
// have their sources in the embedded data, and the VM sets are built from
// that list.
func TestStdSourceList(t *testing.T) {
	for _, p := range stdsrc.Generic {
		if !genericStd[p] {
			t.Errorf("genericStd has no %s", p)
		}
	}
	for _, p := range stdsrc.Interpreted {
		if !InterpretedStd[p] {
			t.Errorf("InterpretedStd has no %s", p)
		}
	}
	if len(genericStd)+len(InterpretedStd) != len(stdsrc.All()) {
		t.Errorf("genericStd %v and InterpretedStd %v are not the list of stdsrc %v", genericStd, InterpretedStd, stdsrc.All())
	}
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types for this platform")
	}
	for _, p := range stdsrc.All() {
		if !embeddedStdSource(p) {
			t.Errorf("the embedded data has no source of %s: go generate ./stdtypes", p)
		}
	}
}
