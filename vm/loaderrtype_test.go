package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// TestLoadErrorType checks that every error of Load, LoadContext and
// LoadCached is a *LoadError at the top of the chain, and that
// errors.Is and errors.As still see the cause: a host (Traefik) falls back
// to another engine on a *LoadError. Before, a type error came as a
// *pathsHidden around an *errors.joinError, and a missing go.mod and a
// panic of the package init were not *LoadError either.
func TestLoadErrorType(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	write := func(t *testing.T, dir, name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plugin := func(t *testing.T, src string) string {
		t.Helper()
		dir := t.TempDir()
		write(t, dir, "go.mod", "module example.com/p\n\ngo 1.22\n")
		write(t, dir, "p.go", src)
		return dir
	}
	isLoadError := func(t *testing.T, what string, err error) {
		t.Helper()
		if _, ok := err.(*LoadError); !ok {
			t.Errorf("%s: %T %v, want a *LoadError", what, err, err)
		}
	}

	// A type error.
	_, err := Load(Options{Dir: plugin(t, "package p\n\nvar X = undefinedName\n")}, "example.com/p")
	isLoadError(t, "type error", err)

	// A missing go.mod (Root stops the search).
	noMod := t.TempDir()
	write(t, noMod, "p.go", "package p\n")
	_, err = Load(Options{Dir: noMod, Root: noMod}, "example.com/p")
	isLoadError(t, "no go.mod", err)
	if !errors.Is(err, errNoGoMod) {
		t.Errorf("no go.mod: errors.Is(err, errNoGoMod) is false: %v", err)
	}

	// An import that a restricted plugin may not use.
	_, err = Load(Options{Dir: plugin(t, "package p\n\nimport \"os/exec\"\n\nfunc F() { exec.Command(\"x\").Run() }\n")}, "example.com/p")
	isLoadError(t, "refused import", err)

	// A panic of the package init, with and without a context.
	panicky := plugin(t, "package p\n\nfunc init() { panic(\"boom\") }\n")
	_, err = Load(Options{Dir: panicky}, "example.com/p")
	isLoadError(t, "init panic", err)
	_, err = LoadContext(context.Background(), Options{Dir: panicky, LoadTimeout: time10s}, "example.com/p")
	isLoadError(t, "init panic with a deadline", err)

	// LoadCached: a type error in the second plugin.
	base, err := Load(Options{Dir: plugin(t, "package p\n\nfunc F() int { return 1 }\n")}, "example.com/p")
	if err != nil {
		t.Fatal(err)
	}
	bad := t.TempDir()
	write(t, bad, "p.go", "package p\n\nvar Y = alsoUndefined\n")
	_, err = base.LoadCached(bad, Options{LoadTimeout: time10s})
	isLoadError(t, "cached type error", err)
	good := t.TempDir()
	write(t, good, "p.go", "package p\n\nfunc G() int { return 2 }\n")
	q, err := base.LoadCached(good, Options{LoadTimeout: time10s})
	if err != nil {
		t.Fatalf("cached load: %v", err)
	}
	if res, err := q.Call(context.Background(), "G"); err != nil || len(res) != 1 {
		t.Errorf("G: %v %v", res, err)
	}
}

const time10s = 10_000_000_000
