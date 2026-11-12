package vm_test

import (
	"go/types"
	"slices"
	"testing"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/stdtypes"
	"github.com/traefik/yaegi/v2/vm"
)

// TestAOTGenericHooks checks that every exported generic function of a
// standard package with an AOT copy runs for the types of a program: the
// copy has only the instances that its own code needs, and the glue has no
// hook for a generic function (cmd/aotgen/generics, aotgeneric.go). Go
// 1.27 added encoding/json/v2 and jsontext, whose generic functions were
// refused at load time.
func TestAOTGenericHooks(t *testing.T) {
	u, err := stdtypes.Load()
	if err != nil {
		t.Skip(err)
	}
	for _, path := range vm.AOTPackages() {
		if !stdtypes.IsStd(path) {
			continue
		}
		p, err := u.Import(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		for _, name := range p.Scope().Names() {
			f, ok := p.Scope().Lookup(name).(*types.Func)
			if !ok || !f.Exported() || f.Type().(*types.Signature).TypeParams() == nil {
				continue
			}
			if !vm.GenericImplemented(path + "." + name) {
				t.Errorf("%s.%s: generic function of an AOT copy without hook (add it to cmd/aotgen/generics/%s)", path, name, path)
			}
		}
	}
}

// TestJSONFamilyMode checks that encoding/json/v2 and jsontext run in the
// same mode: they share the types of encoding/json/internal/jsonopts, so a
// copy and a native package of the two cannot be mixed (a jsontext option
// of a native jsontext given to the copy of json/v2 panicked). An explicit
// mode of one is the mode of the other, with -aot too.
func TestJSONFamilyMode(t *testing.T) {
	const v2, text = "encoding/json/v2", "encoding/json/jsontext"
	if !slices.Contains(vm.AOTPackages(), v2) {
		t.Skip("no AOT copy of encoding/json/v2: the toolchain has no json/v2 (Go 1.26 without GOEXPERIMENT=jsonv2)")
	}
	for _, c := range []struct {
		modes      map[string]vm.Mode
		aot        bool
		v2, jstext vm.Mode
	}{
		{nil, true, vm.ModeAOT, vm.ModeAOT},
		{nil, false, vm.ModeNative, vm.ModeNative},
		{map[string]vm.Mode{text: vm.ModeNative}, true, vm.ModeNative, vm.ModeNative},
		{map[string]vm.Mode{v2: vm.ModeNative}, true, vm.ModeNative, vm.ModeNative},
		{map[string]vm.Mode{v2: vm.ModeAOT}, false, vm.ModeAOT, vm.ModeAOT},
	} {
		if got := vm.ModeFor(c.modes, c.aot, v2); got != c.v2 {
			t.Errorf("modes %v, aot %v: json/v2 %v, want %v", c.modes, c.aot, got, c.v2)
		}
		if got := vm.ModeFor(c.modes, c.aot, text); got != c.jstext {
			t.Errorf("modes %v, aot %v: jsontext %v, want %v", c.modes, c.aot, got, c.jstext)
		}
	}
}
