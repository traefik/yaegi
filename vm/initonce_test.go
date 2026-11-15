package vm

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// initTablesSrc is a plugin with the shape of the tables of brotli
// (github.com/andybalholm/brotli, used by rewrite-body plugins): a large
// array of struct literals and a large array of numbers, set by the
// package init, a func init, and a function literal that the init stores.
func initTablesSrc(n int) string {
	var b strings.Builder
	b.WriteString("package p\n\ntype word struct {\n\tlen, transform byte\n\tidx uint16\n}\n\n")
	fmt.Fprintf(&b, "var words = [%d]word{\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\t{%d, %d, %d},\n", i%25, i%121, (i*7919)%65536)
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "var hashes = [%d]uint16{\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%d,", (i*104729)%65536)
		if i%16 == 15 {
			b.WriteString("\n")
		}
	}
	b.WriteString("}\n\n")
	b.WriteString(`var sum int

var double func(int) int

func init() {
	for _, w := range words {
		sum += int(w.idx)
	}
	double = func(x int) int { return 2 * x }
}

func Get(i int) int { return int(words[i].idx) + int(hashes[i]) }

func Sum() int { return double(sum) }
`)
	return b.String()
}

// TestInitCompiledOnce checks that a load compiles the init functions of
// the plugin once: the preflight compiles them on the machine of the
// plugin, which runs them. They were compiled twice (by the check, on a
// machine that is then dropped, and again to run): on the tables of brotli,
// the check and the init took 0.8 s each. It also checks that the code and
// the SSA of the init functions are dropped once they have run, and that
// the function literal stored by the init still runs.
func TestInitCompiledOnce(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, initTablesSrc(2000))
	var mu sync.Mutex
	count := map[string]int{}
	hooks := &loadHooks{compiled: func(f *ssa.Function) {
		if f.Pkg != nil && f.Pkg.Pkg.Path() == pkgPath {
			mu.Lock()
			count[f.Name()]++
			mu.Unlock()
		}
	}}
	p, err := Load(Options{Dir: dir, hooks: hooks}, pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	for _, name := range []string{"init", "init#1", "init#1$1", "Get", "Sum"} {
		if count[name] != 1 {
			t.Errorf("%s compiled %d times by the load, want 1", name, count[name])
		}
	}
	mu.Unlock()
	for _, name := range []string{"init", "init#1"} {
		f := p.Pkg.Func(name)
		if len(f.Blocks) != 0 {
			t.Errorf("%s keeps %d SSA blocks after it ran", name, len(f.Blocks))
		}
		if fn := p.M.funcs[f]; fn != nil && fn.code != nil {
			t.Errorf("%s keeps %d instructions of compiled code after it ran", name, len(fn.code))
		}
	}
	if lit := p.Pkg.Func("init#1").AnonFuncs; len(lit) != 1 || len(lit[0].Blocks) == 0 {
		t.Errorf("the function literal of init lost its SSA")
	}
	ctx := t.Context()
	for _, c := range []struct {
		name string
		args []any
		want int
	}{{"Get", []any{7}, 7*7919%65536 + 7*104729%65536}, {"Sum", nil, 0}} {
		res, err := p.Call(ctx, c.name, c.args...)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		v, err := res[0].Interface()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := v.(int)
		if c.name == "Sum" {
			want := 0
			for i := 0; i < 2000; i++ {
				want += (i * 7919) % 65536
			}
			c.want = 2 * want
		}
		if got != c.want {
			t.Errorf("%s = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestInitHeapAfterLoad checks the heap that a load keeps for a plugin
// with large tables in its package init: the compiled code and the SSA of
// the init are dropped once it has run. With the tables of brotli (31,705
// structs and 32,768 numbers), the load kept 148 MB, 24 MB since. Here
// 20,000 of each: 73 MB kept before, 3 MB since.
func TestInitHeapAfterLoad(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("large plugin")
	}
	dir, pkgPath := writePlugin(t, initTablesSrc(20000))
	before := liveHeap()
	p, err := Load(Options{Dir: dir}, pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	kept := int64(liveHeap()) - int64(before)
	runtime.KeepAlive(p)
	t.Logf("heap kept by the load: %d MB", kept>>20)
	if kept > 24<<20 {
		t.Errorf("the load keeps %d MB, want at most 24 MB", kept>>20)
	}
}

// TestLoadCachedRunsImportInit checks that LoadCached runs the init of the
// interpreted packages that the plugin imports on its own machine. The
// load drops the SSA of the init of the plugin package only: the SSA of
// an imported package is shared with LoadCached, whose machine runs its
// init again for its own globals (it saw no blocks and skipped the init:
// a nil map).
func TestLoadCachedRunsImportInit(t *testing.T) {
	needSource(t)
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/p\n\ngo 1.22\n",
		"p.go": `package p

import "example.com/p/sub"

func N() int { return len(sub.Table) }
`,
		"sub/sub.go": `package sub

var Table = map[string]int{"a": 1}

func init() { Table["b"] = 2 }
`,
	})
	p, err := Load(Options{Dir: dir}, "example.com/p")
	if err != nil {
		t.Fatal(err)
	}
	q, err := p.LoadCached(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		p    *Plugin
	}{{"Load", p}, {"LoadCached", q}} {
		res, err := c.p.Call(t.Context(), "N")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		v, err := res[0].Interface()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if v.(int) != 2 {
			t.Errorf("%s: len(sub.Table) = %v, want 2", c.name, v)
		}
	}
}

// TestInitCallersAfterDrop checks that the PCs taken in a package init
// still give the init frames after the init code is dropped (a package
// variable that keeps a stack, as github.com/pkg/errors does). decodePC
// compared the PC with the length of the dropped code and refused it.
func TestInitCallersAfterDrop(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, `package p

import "runtime"

var pcs = capture()

var pcs2 []uintptr

func init() { pcs2 = capture() }

func capture() []uintptr {
	b := make([]uintptr, 8)
	return b[:runtime.Callers(1, b)]
}

func names(pc []uintptr) string {
	s := ""
	fr := runtime.CallersFrames(pc)
	for {
		f, more := fr.Next()
		s += f.Function + "\n"
		if !more {
			return s
		}
	}
}

func Frames() string { return names(pcs) + "--\n" + names(pcs2) }
`)
	p, err := Load(Options{Dir: dir}, pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Call(t.Context(), "Frames")
	if err != nil {
		t.Fatal(err)
	}
	v, err := res[0].Interface()
	if err != nil {
		t.Fatal(err)
	}
	s := v.(string)
	for _, want := range []string{"example.com/p.init\n", "example.com/p.init.0\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("frames %q: no %q", s, want)
		}
	}
}

// TestInitCompilePanicNotOwn checks the error of a load when the compiler
// panics on the init of an interpreted package that is not the plugin's
// (Options.Src). The check compiles the init on the machine of the
// plugin and does not report the panic (a gap of the VM); the init then
// ran a Func without code (sync.Once counts a panic as done) and the
// load failed with an index out of range instead of the panic.
func TestInitCompilePanicNotOwn(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, `package p

import "path"

func Bad(s string) bool {
	_, err := path.Match("[", s)
	return err == path.ErrBadPattern
}
`)
	const msg = "test: compiler panic in path.init"
	hooks := &loadHooks{compiled: func(f *ssa.Function) {
		if f.Pkg != nil && f.Pkg.Pkg.Path() == "path" && f.Name() == "init" {
			panic(msg)
		}
	}}
	_, err := Load(Options{Dir: dir, Src: []string{"path"}, hooks: hooks}, pkgPath)
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Fatalf("load error %v, want the panic of the compiler %q", err, msg)
	}
}
