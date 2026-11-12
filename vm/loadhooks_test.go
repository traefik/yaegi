package vm

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestLoadHooksPerLoad: the hooks of a test belong to
// its load. Before, they were package variables: a load of another test
// that ran at the same time called the stage hook of this one (and saw its
// fake Go version), so such tests could not run in parallel.
func TestLoadHooksPerLoad(t *testing.T) {
	needSource(t)
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/subplugin"
	var built atomic.Int32
	h := &loadHooks{
		stage: func(s string) {
			if s == "built" {
				built.Add(1)
			}
		},
		goVersion: func() string { return "go1.1.0" }, // refuses AOT mode
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = Load(Options{Dir: "..", AOT: true}, pkg) // no hooks
		}()
	}
	if _, err := Load(Options{Dir: "..", hooks: h}, pkg); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Errorf("a load without hooks: %v", err)
		}
	}
	if n := built.Load(); n != 1 {
		t.Errorf("the stage hook saw %d builds, want only the one of its load", n)
	}
}
