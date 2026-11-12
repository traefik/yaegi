package vm

import (
	"context"
	"go/ast"
	"go/types"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// shrinkPlugin has a function that only runs at its first request (Plain,
// compiled then), a loop that unevaluatedRange must see (Count), and a
// generic method that ssa first builds at run time, from its syntax and
// types.Info: Box[int].Get is only reached through an interface.
const shrinkPlugin = `package p

import (
	"context"
	"fmt"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

var table = []string{"alpha", "beta", "gamma"}

func Plain(x int) int {
	f := func(y int) int { return y + len(table) }
	return f(x * 2)
}

func Map[T any](s []T, f func(T) T) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		out = append(out, f(v))
	}
	return out
}

type Box[T any] struct{ v T }

func (b Box[T]) Get() T { return b.v }

// Count does not read *p: a nil p does not panic.
func Count(p *[4]int) int {
	n := 0
	for range *p {
		n++
	}
	return n
}

type getter interface{ Get() int }

func box(v int) any { return Box[int]{v} }

type handler struct{}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g := box(7).(getter)
	fmt.Fprint(w, Plain(20), Map([]string{"a"}, func(s string) string { return s + "!" }), Count(nil), g.Get())
}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{}, nil
}
`

// infoOf returns the types.Info that ssa keeps for fn (an unexported field).
func infoOf(t *testing.T, fn *ssa.Function) *types.Info {
	t.Helper()
	f := reflect.ValueOf(fn).Elem().FieldByName("info")
	if !f.IsValid() {
		t.Fatal("ssa.Function has no info field any more: update this test")
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Interface().(*types.Info)
}

// TestLoadDropsSyntax checks that a loaded plugin no longer keeps the
// syntax and the types.Info that only the load needed (shrink.go), and
// that it still runs: functions compiled after the load, a range loop that
// is not evaluated, and a generic method that ssa builds at run time.
func TestLoadDropsSyntax(t *testing.T) {
	needSource(t)
	for _, golist := range []bool{false, true} {
		dir, pkg := writePlugin(t, shrinkPlugin)
		p, err := Load(Options{Dir: dir, GoList: golist}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		decl := func(name string) *ast.FuncDecl {
			fn := p.Pkg.Func(name)
			if fn == nil {
				t.Fatalf("no func %s", name)
			}
			return fn.Syntax().(*ast.FuncDecl)
		}
		if n := len(decl("Plain").Body.List); n != 0 {
			t.Errorf("GoList=%v: Plain keeps %d statements after the load, want 0", golist, n)
		}
		if decl("Plain").Name.Obj != nil {
			t.Errorf("GoList=%v: the name resolution of go/parser is kept", golist)
		}
		if len(decl("Count").Body.List) == 0 {
			t.Errorf("GoList=%v: the body of Count was dropped, unevaluatedRange needs it", golist)
		}
		if len(decl("Map").Body.List) == 0 {
			t.Errorf("GoList=%v: the body of the generic Map was dropped", golist)
		}
		// Since x/tools v0.51.0, ssa releases the types.Info of a
		// function once it is built, except for a generic function
		// (its instances are built later): the info of the package is
		// read through Map.
		info := infoOf(t, p.Pkg.Func("Map"))
		if info.FileVersions != nil || info.Scopes != nil {
			t.Errorf("GoList=%v: types.Info keeps FileVersions (%d) or Scopes (%d)", golist, len(info.FileVersions), len(info.Scopes))
		}
		plain := decl("Plain")
		for id := range info.Uses {
			if plain.Pos() <= id.Pos() && id.Pos() < plain.End() {
				t.Errorf("GoList=%v: types.Info.Uses keeps %s of Plain", golist, id.Name)
			}
		}
		if len(info.Uses) == 0 {
			t.Errorf("GoList=%v: types.Info.Uses lost the uses of the generic functions", golist)
		}

		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		b, _ := io.ReadAll(w.Body)
		if got, want := string(b), "43 [a!] 4 7"; got != want {
			t.Errorf("GoList=%v: response %q, want %q", golist, got, want)
		}

		// A cached load on the same program still type checks and runs.
		q, err := p.LoadCached(filepath.Join(dir), Options{})
		if err != nil {
			t.Fatalf("GoList=%v: LoadCached: %v", golist, err)
		}
		cfg, err = createConfig(q)
		if err != nil {
			t.Fatal(err)
		}
		h, err = newHandler(context.Background(), q, http.NotFoundHandler(), cfg, "y")
		if err != nil {
			t.Fatal(err)
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if got, want := w.Body.String(), "43 [a!] 4 7"; got != want {
			t.Errorf("GoList=%v: LoadCached: response %q, want %q", golist, got, want)
		}
		if n := len(q.Pkg.Func("Plain").Syntax().(*ast.FuncDecl).Body.List); n != 0 {
			t.Errorf("GoList=%v: LoadCached: Plain keeps %d statements, want 0", golist, n)
		}
	}
}
