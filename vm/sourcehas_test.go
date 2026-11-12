package vm

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSourceHasPerMachine: the //line check of position
// keeps its answers in the machine, not the files in a process-wide cache
// that grew with every load and was never freed. The answer of a machine
// does not change, and a new machine reads the file again.
func TestSourceHasPerMachine(t *testing.T) {
	name := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(name, []byte("package p\n//line other.go:100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Machine{}
	if !m.sourceHas(name, "line other.go:") || m.sourceHas(name, "line nothere.go:") {
		t.Fatal("sourceHas is wrong for the file")
	}
	if err := os.WriteFile(name, []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !m.sourceHas(name, "line other.go:") {
		t.Error("the machine did not keep its answer")
	}
	if (&Machine{}).sourceHas(name, "line other.go:") {
		t.Error("a new machine got the answer of another one")
	}
	n := 0
	m.sourceAnswers.Range(func(k, v any) bool {
		if _, ok := v.(bool); !ok {
			t.Errorf("the machine keeps a %T for %v, want a bool", v, k)
		}
		n++
		return true
	})
	if n != 2 {
		t.Errorf("%d answers kept, want 2", n)
	}
}
