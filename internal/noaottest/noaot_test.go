package noaottest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/vm"
)

// TestCheckAOTWithoutCopies: in a host that does not import aot/all,
// CheckAOT(false) returns an error that names aot/all, and a load in AOT
// mode says in Plugin.Notes that the standard library runs natively.
// CheckAOT returned nil and the load said nothing, so the CheckAOT(false)
// test that the README asks of Traefik passed without the copies.
func TestCheckAOTWithoutCopies(t *testing.T) {
	if got := vm.AOTPackages(); len(got) != 0 {
		t.Fatalf("this test binary has AOT copies (%v): it must not import aot/all", got)
	}
	for _, allowStale := range []bool{false, true} {
		if err := vm.CheckAOT(allowStale); err == nil || !strings.Contains(err.Error(), "yaegi/v2/aot/all") {
			t.Errorf("CheckAOT(%v) = %v, want an error that names aot/all", allowStale, err)
		}
	}
	dir := t.TempDir()
	for name, data := range map[string]string{
		"go.mod":  "module example.com/plug\n\ngo 1.26\n",
		"plug.go": "package plug\n\nimport \"fmt\"\n\nfunc F() string { return fmt.Sprint(1) }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hasNote := func(p *vm.Plugin) bool {
		for _, n := range p.Notes {
			if strings.Contains(n, "no AOT copy") && strings.Contains(n, "yaegi/v2/aot/all") {
				return true
			}
		}
		return false
	}
	p, err := vm.Load(vm.Options{Dir: dir, AOT: true}, "example.com/plug")
	if err != nil {
		t.Fatal(err)
	}
	if !hasNote(p) {
		t.Errorf("AOT mode: Notes = %q, want a note about the missing AOT copies", p.Notes)
	}
	p, err = vm.Load(vm.Options{Dir: dir}, "example.com/plug")
	if err != nil {
		t.Fatal(err)
	}
	if hasNote(p) {
		t.Errorf("default mode: Notes = %q, want no note about AOT copies", p.Notes)
	}
}
