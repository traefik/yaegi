package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// stackLimitSrc is the plugin of the MaxStack tests. f recurses with small
// frames, g with a frame-local [100]int (a cell of 101 Values), d defers in
// a loop, h recurses in a goroutine.
const stackLimitSrc = `package p

var depth int

func f(n int) int { depth = n; return f(n+1) + 1 }

func g(n int) int {
	var a [100]int
	a[n%100] = n
	depth = n
	return g(n+1) + a[(n+1)%100]
}

func nop() {}

func d() int {
	for i := 1; i < 1<<30; i++ {
		depth = i
		defer nop()
	}
	return 0
}

func F() int { return f(1) }

func G() int { return g(1) }

func D() int { return d() }

func Depth() int { return depth }
`

// stackFunc returns the compiled function name of p.
func stackFunc(t *testing.T, p *Plugin, name string) *Func {
	t.Helper()
	p.M.mu.Lock()
	defer p.M.mu.Unlock()
	for s, fn := range p.M.funcs {
		if s.Name() == name && s.Pkg != nil && s.Pkg.Pkg.Path() == "example.com/p" {
			fn.ensure()
			return fn
		}
	}
	t.Fatalf("no function %s", name)
	return nil
}

// stackDepth calls name in p, wants ErrStackOverflow, and returns the depth
// that the recursion reached.
func stackDepth(t *testing.T, p *Plugin, name string) int {
	t.Helper()
	_, err := p.Call(context.Background(), name)
	if !errors.Is(err, ErrStackOverflow) {
		t.Fatalf("%s: error %v, want ErrStackOverflow", name, err)
	}
	// The message names the limits: an overflow of MaxStack used to
	// read "(max depth)".
	if !strings.Contains(err.Error(), "MaxStack") {
		t.Errorf("%s: error %q does not name MaxStack", name, err)
	}
	res, err := p.Call(context.Background(), "Depth")
	if err != nil {
		t.Fatal(err)
	}
	v, err := res[0].Interface()
	if err != nil {
		t.Fatal(err)
	}
	return v.(int)
}

// TestMaxStackExact checks the exact depth at which Options.MaxStack stops a
// recursion: the frames of F and of the recursive function, and the cells
// of the frame-local arrays, fill the bound and the next frame does not
// fit. One Value more in the bound (or one less) must not change the depth
// unless a whole frame fits (or no longer fits): a check loosened or
// tightened by anything fails here.
func TestMaxStackExact(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, stackLimitSrc)
	for _, limit := range []int64{1 << 20, 3<<20 + 7*valueSize, 8 << 20} {
		p, err := Load(Options{Dir: dir, MaxStack: limit}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if p.M.MaxStack != limit {
			t.Fatalf("Machine.MaxStack %d, want %d", p.M.MaxStack, limit)
		}
		vals := limit / valueSize
		// Small frames: only the registers count.
		got := stackDepth(t, p, "F")
		fF, f := stackFunc(t, p, "F"), stackFunc(t, p, "f")
		want := int((vals - int64(fF.nregs)) / int64(f.nregs))
		if got != want {
			t.Errorf("MaxStack %d: f reached depth %d, want %d (%d registers per frame)", limit, got, want, f.nregs)
		}
		// Frames with a local array: its cell counts too.
		got = stackDepth(t, p, "G")
		fG, g := stackFunc(t, p, "G"), stackFunc(t, p, "g")
		if len(g.locals) != 1 || g.locals[0].site.cellVals != 101 {
			t.Fatalf("g: %d frame-local cells, want one of 101 Values", len(g.locals))
		}
		want = int((vals - int64(fG.nregs)) / int64(g.nregs+101))
		if got != want {
			t.Errorf("MaxStack %d: g reached depth %d, want %d (%d registers and 101 cell Values per frame)", limit, got, want, g.nregs)
		}
	}
}

// TestMaxStackDefer checks that the deferred calls of a loop count in
// MaxStack: the loop stops with ErrStackOverflow, after as many defers as
// the bound holds. A record counts twice, for the copy of a full slice, and
// the check runs when the slice is full: after it, the slice fills up to its
// new capacity, 1.25 times the old one for a large slice.
func TestMaxStackDefer(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, stackLimitSrc)
	const limit = 4 << 20
	p, err := Load(Options{Dir: dir, MaxStack: limit}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	n := stackDepth(t, p, "D")
	most := int(limit / valueSize / int64(2*deferVals))
	if n > most*5/4 || n < most/2 {
		t.Errorf("%d defers before the overflow, want between %d and %d", n, most/2, most*5/4)
	}
}

// TestMaxStackOptions checks the MaxStack of a plugin: DefaultMaxStack for
// 0, none for a negative value.
func TestMaxStackOptions(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, "package p\n\nfunc F() int { return 1 }\n")
	for _, tt := range []struct{ opt, want int64 }{{0, DefaultMaxStack}, {-1, 0}, {1 << 30, 1 << 30}} {
		p, err := Load(Options{Dir: dir, MaxStack: tt.opt}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if p.M.MaxStack != tt.want {
			t.Errorf("Options.MaxStack %d: Machine.MaxStack %d, want %d", tt.opt, p.M.MaxStack, tt.want)
		}
	}
}

// TestMaxMemorySmallLocals checks that a small frame-local array is not
// counted in MaxMemory. The first local array of each call from the host
// reserved 1 MiB, held by a token until the next garbage collection: 51
// calls kept 51 MiB of the quota.
func TestMaxMemorySmallLocals(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, "package p\n\nfunc H() int {\n\tvar a [4]int\n\tfor i := range a {\n\t\ta[i] = i\n\t}\n\treturn a[3]\n}\n")
	p, err := Load(Options{Dir: dir, MaxMemory: 64 << 20}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	for range 200 {
		if _, err := p.Call(context.Background(), "H"); err != nil {
			t.Fatal(err)
		}
	}
	if h := stackFunc(t, p, "H"); len(h.locals) != 1 {
		t.Fatalf("H: %d frame-local cells, want 1", len(h.locals))
	}
	if used := p.M.Memory(); used > 1<<20 {
		t.Errorf("after 200 calls with a [4]int local: %d bytes in the quota, want less than 1 MiB", used)
	}
}

// TestDefaultMaxStackDeep checks that the default MaxStack lets a valid
// recursion run: 3000 frames with a local [4096]byte use about 12 MB of
// stack under gc, but 290 MiB in the VM (a Value of 24 bytes per byte of the
// array). It stopped with ErrStackOverflow when the default was 256 MiB.
func TestDefaultMaxStackDeep(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, "package p\n\nfunc f(n int) int {\n\tvar b [4096]byte\n\tb[n%4096] = byte(n)\n\tif n == 0 {\n\t\treturn int(b[0])\n\t}\n\treturn f(n-1) + int(b[n%4096])\n}\n\nfunc F() int { return f(3000) }\n")
	p, err := Load(Options{Dir: dir}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call(context.Background(), "F"); err != nil {
		t.Fatalf("F: %v, want no error with the default MaxStack", err)
	}
	if f := stackFunc(t, p, "f"); len(f.locals) != 1 || f.locals[0].site.cellVals*valueSize*3000 <= 256<<20 {
		t.Fatalf("f: the frames must pass 256 MiB in the VM: %d frame-local cells", len(f.locals))
	}
}

// TestMaxMemoryGlobals checks that the package-level variables count in
// MaxMemory: a variable that does not fit is a load error before it is
// allocated, one that fits is counted.
func TestMaxMemoryGlobals(t *testing.T) {
	needSource(t)
	const quota = 64 << 20
	dir, pkg := writePlugin(t, "package p\n\nvar big [4 << 20]byte\n\nfunc F() int { return int(big[1]) }\n")
	_, err := Load(Options{Dir: dir, MaxMemory: quota}, pkg)
	if err == nil || !strings.Contains(err.Error(), "package-level variable example.com/p.big") || !strings.Contains(err.Error(), "MaxMemory") {
		t.Fatalf("load of a 96 MiB variable with a 64 MiB quota: %v, want a load error", err)
	}
	dir, pkg = writePlugin(t, "package p\n\nvar a, b [1 << 20]byte\n\nvar c [1 << 20]byte\n\nfunc F() int { return int(a[1] + b[1] + c[1]) }\n")
	_, err = Load(Options{Dir: dir, MaxMemory: quota}, pkg)
	if err == nil || !strings.Contains(err.Error(), "package-level variable") {
		t.Fatalf("load of three 24 MiB variables with a 64 MiB quota: %v, want a load error", err)
	}
	dir, pkg = writePlugin(t, "package p\n\nvar a, b [1 << 20]byte\n\nfunc F() int { return int(a[1] + b[1]) }\n")
	p, err := Load(Options{Dir: dir, MaxMemory: quota}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	// The variables are made when the code that uses them is compiled.
	if _, err := p.Call(context.Background(), "F"); err != nil {
		t.Fatal(err)
	}
	if got, want := p.M.Memory(), int64(2*(1<<20)*valueSize); got < want {
		t.Errorf("Memory() %d after two 24 MiB variables, want at least %d", got, want)
	}
}

// stackHostileEnv names the case of TestStackHostileChild.
const stackHostileEnv = "YAEGI_STACK_HOSTILE"

// stackHostileCases are plugins that grow the stack of a goroutine until
// the host runs out of memory, with no limit other than the default ones
// of a plugin: a recursion with big frames (MaxDepth is far), a defer in a
// long loop, the same with closures, and large package-level variables
// (each under the bound of one value, maxZeroElems).
var stackHostileCases = map[string]struct{ src, want string }{
	"bigFrames":    {"package p\n\nfunc f(n int) int { var a [2000]int; a[n%2000] = n; return f(n+1) + a[0] }\n\nfunc F() int { return f(0) }\n", ErrStackOverflow.Error()},
	"manyLocals":   {"package p\n\nfunc f(n int) int {\n" + manyLocals(400) + "\treturn f(n+1)" + sumLocals(400) + "\n}\n\nfunc F() int { return f(0) }\n", ErrStackOverflow.Error()},
	"deferLoop":    {"package p\n\nfunc g(a, b int) {}\n\nfunc F() int { for i := 0; i < 1<<30; i++ { defer g(i, i) }; return 0 }\n", ErrStackOverflow.Error()},
	"deferClosure": {"package p\n\nfunc F() (s int) { for i := 0; i < 1<<30; i++ { defer func() { s += i }() }; return 0 }\n", ErrStackOverflow.Error()},
	"goroutines":   {"package p\n\nfunc f(n int) int { var a [2000]int; a[n%2000] = n; return f(n+1) + a[0] }\n\nfunc F() int { for i := 0; i < 4; i++ { go f(0) }; return 0 }\n", "4 goroutines, ErrMemory: true"},
	"global":       {"package p\n\nvar big [60 << 20]byte\n\nfunc F() int { return int(big[1]) }\n", "package-level variable"},
	"globals":      {"package p\n\nvar a, b, c, d, e, f [8 << 20]byte\n\nfunc F() int { return int(a[1] + b[1] + c[1] + d[1] + e[1] + f[1]) }\n", "package-level variable"},
}

// manyLocals declares n int locals v0..vn-1 that stay in registers.
func manyLocals(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "\tv%d := n*%d + 1\n", i, i+1)
	}
	return b.String()
}

// sumLocals is " + v0 + v1 ... + vn-1".
func sumLocals(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, " + v%d", i)
	}
	return b.String()
}

// TestStackHostileChild is run by TestStackHostile in a child process: it
// loads the case with the limits of a host that runs plugins it does not
// trust (as internal/pluginhost, but half its MaxMemory: the heap watchdog
// of the child stops at 1 GiB), calls F, and prints the error. MaxStack is
// 256 MiB, as in internal/pluginhost, below the quota, so that the bound of
// the stack stops the cases of one goroutine before the quota does.
func TestStackHostileChild(t *testing.T) {
	name := os.Getenv(stackHostileEnv)
	if name == "" {
		t.Skip("helper process of TestStackHostile")
	}
	startHeapWatchdog()
	c := stackHostileCases[name]
	dir, pkg := writePlugin(t, c.src)
	var mu sync.Mutex
	var panics []any
	opts := Options{Dir: dir, Budget: 100_000_000, MaxMemory: 512 << 20, MaxStack: 256 << 20, MaxGoroutines: 1000, PanicOnFatal: true, Env: []string{},
		GoPanic: func(r any) { mu.Lock(); panics = append(panics, r); mu.Unlock() }}
	if name == "goroutines" {
		// Each goroutine stays under MaxStack, and together they pass
		// the quota: the cells count in MaxMemory too. Smaller limits
		// than the default keep the test short: past the quota, the
		// quota forces garbage collections and makes the plugin wait.
		opts.MaxStack, opts.MaxMemory = 64<<20, 128<<20
	}
	p, err := Load(opts, pkg)
	if err != nil {
		fmt.Printf("RESULT: %v\n", err)
		return
	}
	_, err = p.Call(context.Background(), "F")
	if name == "goroutines" {
		// The goroutines of the plugin stop on their own: GoPanic gets
		// each error.
		for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			n := len(panics)
			mu.Unlock()
			if n == 4 {
				break
			}
		}
		mu.Lock()
		mem := false
		for _, r := range panics {
			mem = mem || r == ErrMemory
		}
		err = fmt.Errorf("%d goroutines, ErrMemory: %v: %v", len(panics), mem, panics)
		mu.Unlock()
	}
	fmt.Printf("RESULT: %v\n", err)
}

// TestStackHostile checks that a plugin that grows the stack of a goroutine
// (big frames, deferred calls) or declares large package-level variables
// stops with ErrStackOverflow or a load error, under the default limits of
// a plugin, and the host does not run out of memory. Each case runs in a
// child process with a heap watchdog (1 GiB): before the bound in bytes,
// these plugins took the host past 3 GiB.
func TestStackHostile(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("runs child processes")
	}
	for name, c := range stackHostileCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStackHostileChild$", "-test.count=1")
			cmd.Env = append(os.Environ(), stackHostileEnv+"="+name, "GOMEMLIMIT=1GiB")
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "RESULT: ") {
				t.Fatalf("child process failed (%v):\n%s", err, out)
			}
			if !strings.Contains(string(out), c.want) {
				t.Fatalf("want an error with %q, got:\n%s", c.want, out)
			}
		})
	}
}
