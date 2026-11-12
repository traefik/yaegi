package vm

import (
	"go/types"
	"runtime"
	"testing"
	"weak"
)

// TestLoadReleasesTypes loads a plugin that compares pointers and uses them
// as map keys several times, then checks that the Machine and the types of
// each load can be freed. A global cache keyed by types (such as the old
// ptrInfo cache) would keep the type graph of every loaded plugin alive.
func TestLoadReleasesTypes(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/ptrplugin"
	var ms []weak.Pointer[Machine]
	var ps []weak.Pointer[types.Package]
	for i := 0; i < 3; i++ {
		p, err := Load(Options{Dir: ".."}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if g := p.M.globals[p.Pkg.Var("Count")]; g.R == nil || g.R.(*Value).N != 12 {
			t.Fatalf("Count = %v, want 12", g)
		}
		ms = append(ms, weak.Make(p.M))
		ps = append(ps, weak.Make(p.Pkg.Pkg))
	}
	for i := 0; i < 5; i++ {
		runtime.GC()
	}
	for i := range ms {
		if ms[i].Value() != nil {
			t.Errorf("load %d: the Machine is still alive", i)
		}
		if ps[i].Value() != nil {
			t.Errorf("load %d: the types of the plugin are still alive", i)
		}
	}
}
