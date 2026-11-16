//go:build go1.27

package vm

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"strings"
	"testing"
	"time"
)

// Generic methods (Go 1.27, genmethod.go). The programs that compare them
// with go run are testdata/genmethods.go and testdata/genmethodsreflect.go.

// genMethodBox is a plugin with the main use of generic methods: a method
// whose result is the same generic type with another argument.
const genMethodBox = `//go:build go1.27

package p

type Box[T any] struct{ v T }

func (b Box[T]) Map[R any](f func(T) R) Box[R] { return Box[R]{f(b.v)} }

func (b Box[T]) Get() T { return b.v }

type E struct{ Box[int] }

func Run() any { return Box[int]{} }

func RunE() any { return E{Box[int]{4}} }

func Twice() int { return Box[int]{21}.Map(func(i int) int { return 2 * i }).Get() }

func NewBox() Box[int] { return Box[int]{3} }
`

// TestLoadGenericMethodBox checks that a plugin with a generic method
// whose result is its own type with a new argument loads. The preflight
// walked the signature of Box[int].Map and got Box[R] with a new R each
// time: Load never returned (with LoadTimeout, it returned an error but
// the load kept one CPU and the slot of the plugin forever).
func TestLoadGenericMethodBox(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, genMethodBox)
	for i := 0; i < 2; i++ { // twice: the slot of the plugin is free again
		p, err := Load(Options{Dir: dir, LoadTimeout: time.Minute}, pkgPath)
		if err != nil {
			t.Fatalf("load %d: %v", i, err)
		}
		res, err := p.Call(context.Background(), "Twice")
		if err != nil {
			t.Fatal(err)
		}
		if v, err := res[0].Interface(); err != nil || v != 42 {
			t.Fatalf("Twice() = %v, %v; want 42", v, err)
		}
	}
	// Object.Call finds the methods of the run-time method set only: a
	// generic method has no code without its type arguments.
	p, err := Load(Options{Dir: dir}, pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Call(context.Background(), "NewBox")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := res[0].Call(context.Background(), "Map", func(int) int { return 0 }); err == nil || !strings.Contains(err.Error(), "no exported method Map") {
		t.Errorf("Call of the generic method Map: %v, want an error", err)
	}
	if r, err := res[0].Call(context.Background(), "Get"); err != nil {
		t.Error(err)
	} else if v, err := r[0].Interface(); err != nil || v != 3 {
		t.Errorf("Get() = %v, %v; want 3", v, err)
	}
}

// TestPreflightStopsOnDoneContext checks that the preflight stops when the
// context of the load is done: a load that LoadContext gave up on must not
// keep running (and keep the slot of the plugin).
func TestPreflightStopsOnDoneContext(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, genMethodBox)
	p, err := Load(Options{Dir: dir}, pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = p.M.preflight(ctx, p.Pkg, func(*types.Package) bool { return true }, nil)
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("want a *LoadError for the canceled context, got %v", err)
	}
}

// TestLoadTimeoutPhase checks that the error of a load whose context is
// done after the SSA build does not blame parse or type checking. The
// "built" hook cancels the context and blocks until LoadContext returns, so
// the result does not depend on how long parse and type checking take (a
// short LoadTimeout could pass before the build under -race).
func TestLoadTimeoutPhase(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, "package p\n\nfunc CreateConfig() any { return nil }\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unblock := make(chan struct{})
	hook := func(stage string) {
		if stage == "built" {
			cancel()
			<-unblock
		}
	}
	_, err := LoadContext(ctx, Options{Dir: dir, hooks: &loadHooks{stage: hook}}, pkgPath)
	close(unblock)
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatalf("want *LoadError, got %v", err)
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("want the error of the canceled context, got %v", err)
	}
	if !strings.Contains(err.Error(), "load-time check") || strings.Contains(err.Error(), "type checking") {
		t.Fatalf("the error blames the wrong step: %v", err)
	}
}

// genMethodChain is a chain of n generic methods of the generic type G,
// each calling the next with a type argument twice as large.
func genMethodChain(n int) string {
	return "//go:build go1.27\n\n" + chain("type G[X any] struct{}\ntype P[A, B any] struct{ a A; b B }\n", n,
		func(i int) string {
			return fmt.Sprintf("func (G[X]) F%d[T any]() any { return G[X]{}.F%d[P[T, T]]() }\n", i, i+1)
		},
		fmt.Sprintf("func (G[X]) F%d[T any]() any { var x T; return x }\nfunc H() any { return G[int]{}.F1[int]() }\n", n+1))
}

func init() {
	// The same chain as genericFuncChain, with generic methods of a generic
	// type: typeParams gave only the receiver type parameters, so the sizes
	// of the method ones were never measured (27 levels used 11 GB).
	loadBombs = append(loadBombs, struct{ name, src, want string }{
		"genericMethodChain", genMethodChain(40), "generic instance too large to build"})
}

// TestInstanceGenericMethodSizes checks that the sites of a generic method
// are measured with the sizes of its own type parameters and of those of
// its receiver.
func TestInstanceGenericMethodSizes(t *testing.T) {
	fset, p := checkedPkg(t, genMethodChain(3))
	g, err := measureInstances(fset, []*srcPkg{p})
	if err != nil {
		t.Fatal(err)
	}
	gt := p.types.Scope().Lookup("G").Type().(*types.Named)
	for i, want := range []int64{1, 3, 7, 15} { // T of F1..F4: int, P[int, int], ...
		f := gt.Method(i)
		if got := g.sizes[f]; len(got) != 2 || got[0] != 1 || got[1] != want {
			t.Errorf("sizes of %s: %v, want [1 %d]", f.Name(), got, want)
		}
	}
}
