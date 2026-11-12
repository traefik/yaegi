package vm

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// writePlugin writes a single-file plugin into a temp module and returns the
// module directory and its import path.
func writePlugin(t *testing.T, src string) (dir, pkgPath string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, "example.com/p"
}

func needSource(t *testing.T) {
	t.Helper()
	if !stdtypes.Available() {
		t.Skip("no embedded stdlib types for this platform")
	}
}

// genAliasBomb is a tiny source that makes go/types instantiate an
// exponential number of generic aliases: type Tn = P[Tn-1, Tn-1]. go/types
// has no cancellation, so only the load deadline can stop it.
func genAliasBomb(n int) string {
	var b strings.Builder
	b.WriteString("package p\n\ntype P[A, B any] struct{ a A; b B }\ntype T0 = int\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "type T%d = P[T%d, T%d]\n", i, i-1, i-1)
	}
	fmt.Fprintf(&b, "var X T%d\n\nfunc F() any { return X }\n", n)
	return b.String()
}

// TestLoadTimeoutGenerics checks that a plugin whose type checking runs for a
// while returns a *LoadError within the deadline, instead of blocking the
// load. The bomb is kept small (n=18,
// under a second to type-check in full) so that the abandoned background
// goroutine of this test finishes cheaply: a larger one would take tens of
// seconds and gigabytes of memory, because go/types cannot be cancelled.
func TestLoadTimeoutGenerics(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, genAliasBomb(18))
	start := time.Now()
	_, err := Load(Options{Dir: dir, LoadTimeout: 50 * time.Millisecond}, pkgPath)
	elapsed := time.Since(start)
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatalf("want *LoadError, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("load took %v, deadline not honored", elapsed)
	}
}

// TestLoadTimeoutZero checks the deadline wiring without a resource bomb: a
// tiny deadline on a normal plugin makes Load return a *LoadError. The hook
// waits after the SSA build: on Windows the timer of a 1ns deadline can fire
// after a small load ends, and then nothing would see the deadline.
func TestLoadTimeoutZero(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, "package p\n\nfunc CreateConfig() any { return nil }\n")
	hook := func(stage string) {
		if stage == "built" {
			time.Sleep(50 * time.Millisecond)
		}
	}
	_, err := Load(Options{Dir: dir, LoadTimeout: time.Nanosecond, hooks: &loadHooks{stage: hook}}, pkgPath)
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatalf("want *LoadError, got %v", err)
	}
}

// lateCtx is a context whose deadline has passed but whose timer has not
// fired yet: Err is still nil, as on Windows just after the deadline.
type lateCtx struct{ context.Context }

func (lateCtx) Deadline() (time.Time, bool) { return time.Now().Add(-time.Second), true }
func (lateCtx) Err() error                  { return nil }

// TestCtxErrLateTimer checks that ctxErr sees a passed deadline before the
// timer of the context fires (TestLoadTimeoutZero failed on Windows).
func TestCtxErrLateTimer(t *testing.T) {
	err := ctxErr(lateCtx{context.Background()})
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("ctxErr = %v, want a *LoadError for the deadline", err)
	}
	if err := ctxErr(context.Background()); err != nil {
		t.Fatalf("ctxErr(Background) = %v", err)
	}
}

// TestLoadContextCancel checks that an already cancelled context stops the
// load.
func TestLoadContextCancel(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, "package p\n\nfunc CreateConfig() any { return nil }\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := LoadContext(ctx, Options{Dir: dir}, pkgPath)
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatalf("want *LoadError, got %v", err)
	}
}

// TestLoadHugeArray checks that a global whose zero value is a huge array is
// a load error, not a fatal out-of-memory (which no recover can stop). gc
// refuses the same declaration as a symbol too large.
func TestLoadHugeArray(t *testing.T) {
	needSource(t)
	// A composite literal with a huge index materializes the array at init.
	dir, pkgPath := writePlugin(t, wordSized("package p\n\nvar X = [...]byte{1 << 40: 1}\n"))
	_, err := Load(Options{Dir: dir}, pkgPath)
	// With the preflight on (the default), the huge zero value is caught
	// when the preflight compiles init and reported as a *PreflightError;
	// with NoPreflight it is caught at init as a *LoadError. Either way the
	// load is refused, not a fatal out-of-memory.
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want a load error mentioning the size, got %v", err)
	}
}

// TestLoadHugeArrayNested checks the same for an array of arrays, whose total
// element count overflows: the per-dimension length is small.
func TestLoadHugeArrayNested(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, "package p\n\nvar X [1 << 24][1 << 24]byte\n\nvar Y = X[0][0]\n")
	_, err := Load(Options{Dir: dir}, pkgPath)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want a load error mentioning the size, got %v", err)
	}
}

// TestLoadSourceTooLarge checks the total source size limit.
func TestLoadSourceTooLarge(t *testing.T) {
	needSource(t)
	big := "package p\n\nvar S = \"" + strings.Repeat("a", 2<<20) + "\"\n"
	dir, pkgPath := writePlugin(t, big)
	limits := DefaultLoadLimits
	limits.MaxTotalSourceBytes = 1 << 20 // 1 MiB, smaller than the file
	_, err := Load(Options{Dir: dir, LoadLimits: &limits}, pkgPath)
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), "source too large") {
		t.Fatalf("want source too large *LoadError, got %v", err)
	}
}

// TestLoadHeaderReadBounded checks that the reads of go/build (the header
// of each file) and of the yaegi:tags scan stop at the total source budget:
// before, a file with a huge comment before its package clause was read
// into memory whole, before parseFile refused its size.
func TestLoadHeaderReadBounded(t *testing.T) {
	std, err := stdtypes.Load()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644)
	name := filepath.Join(dir, "p.go")
	os.WriteFile(name, []byte("// "+strings.Repeat("a", 1000)+"\npackage p\n"), 0o644)
	limits := DefaultLoadLimits
	limits.MaxTotalSourceBytes = 100
	l, err := newSrcLoader(context.Background(), dir, "", "", "example.com/p", std, newLoadBudget(limits))
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.ctx.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 100 {
		t.Errorf("OpenFile read %d bytes, want 100 (the budget)", len(b))
	}
}

// TestLoadGoModTooLarge checks the size limit of a go.mod (maxModFileBytes):
// the Go files have the load budget, a go.mod was read whole whatever its
// size.
func TestLoadGoModTooLarge(t *testing.T) {
	needSource(t)
	dir := t.TempDir()
	gomod := "module example.com/p\n\ngo 1.22\n\n// " + strings.Repeat("a", maxModFileBytes) + "\n"
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644)
	os.WriteFile(filepath.Join(dir, "p.go"), []byte("package p\n"), 0o644)
	_, err := Load(Options{Dir: dir}, "example.com/p")
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("want larger than *LoadError, got %v", err)
	}
}

// TestLoadTooManyPackages checks the package count limit on a module tree.
func TestLoadTooManyPackages(t *testing.T) {
	needSource(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// p imports p/a, p/a imports p/b: three packages.
	os.WriteFile(filepath.Join(dir, "p.go"), []byte("package p\n\nimport _ \"example.com/p/a\"\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "a.go"), []byte("package a\n\nimport _ \"example.com/p/b\"\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "b", "b.go"), []byte("package b\n"), 0o644)
	limits := DefaultLoadLimits
	limits.MaxPackages = 2
	_, err := Load(Options{Dir: dir, LoadLimits: &limits}, "example.com/p")
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), "too many source packages") {
		t.Fatalf("want too many packages *LoadError, got %v", err)
	}
}

// TestLoadGoodStaysGood checks that a normal plugin still loads with the
// default limits and no deadline.
func TestLoadGoodStaysGood(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, "package p\n\nfunc CreateConfig() any { return nil }\n")
	if _, err := Load(Options{Dir: dir}, pkgPath); err != nil {
		t.Fatal(err)
	}
	// A short deadline must not trip on a tiny plugin.
	if _, err := Load(Options{Dir: dir, LoadTimeout: 10 * time.Second}, pkgPath); err != nil {
		t.Fatal(err)
	}
}

// TestZeroEntries checks the count of Value entries of zero values,
// including arrays inside structs, and that it saturates instead of
// overflowing.
func TestZeroEntries(t *testing.T) {
	m := &Machine{}
	byteT := types.Typ[types.Byte]
	arr := func(n int64, e types.Type) types.Type { return types.NewArray(e, n) }
	strct := func(fs ...types.Type) types.Type {
		var vs []*types.Var
		for i, f := range fs {
			vs = append(vs, types.NewField(0, nil, fmt.Sprintf("f%d", i), f, false))
		}
		return types.NewStruct(vs, nil)
	}
	over := int64(maxZeroElems + 1)
	tests := []struct {
		t    types.Type
		want int64
	}{
		{byteT, 0},
		{arr(10, byteT), 10},
		{arr(10, arr(3, byteT)), 10 * 4},
		{strct(byteT, arr(5, byteT)), 2 + 5},
		{arr(1<<20, strct(arr(1<<20, byteT))), over},
		{strct(arr(1<<25, byteT), arr(1<<25, byteT), arr(1<<25, byteT)), over},
		{arr(1<<62, arr(1<<62, byteT)), over},
		{arr(1<<40, strct()), 0}, // a zslice
	}
	for _, tt := range tests {
		if got := m.zeroEntries(tt.t); got != tt.want {
			t.Errorf("zeroEntries(%s) = %d, want %d", tt.t, got, tt.want)
		}
	}
	// A type whose size doubles at each level: the count stops early.
	ty := types.Type(byteT)
	for range 100 {
		ty = strct(ty, ty)
	}
	if got := m.zeroEntries(ty); got != over {
		t.Errorf("zeroEntries of a 2^100 struct = %d, want %d", got, over)
	}
}

// BenchmarkZero measures m.zero with its size check: a flat struct and a
// struct with arrays inside (the size check must not cost much at run time:
// clear of a slice of structs, reflect.New, decoders).
func BenchmarkZero(b *testing.B) {
	m := &Machine{}
	byteT := types.Typ[types.Byte]
	field := func(name string, t types.Type) *types.Var { return types.NewField(0, nil, name, t, false) }
	flat := types.NewStruct([]*types.Var{field("a", types.Typ[types.Int]), field("b", types.Typ[types.String]), field("c", byteT)}, nil)
	inner := types.NewStruct([]*types.Var{field("x", types.NewArray(byteT, 8)), field("y", types.Typ[types.Int])}, nil)
	nested := types.NewStruct([]*types.Var{field("a", types.NewArray(inner, 4)), field("b", inner), field("c", types.NewArray(byteT, 16))}, nil)
	for _, bt := range []struct {
		name string
		t    types.Type
	}{{"flat", flat}, {"nested", nested}} {
		b.Run(bt.name+"/zero", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m.zero(bt.t)
			}
		})
		b.Run(bt.name+"/zeroValue", func(b *testing.B) { // no size check
			b.ReportAllocs()
			for b.Loop() {
				m.zeroValue(bt.t)
			}
		})
		b.Run(bt.name+"/checkZero", func(b *testing.B) { // the count kept per type
			b.ReportAllocs()
			for b.Loop() {
				m.checkZero(bt.t)
			}
		})
		b.Run(bt.name+"/zeroEntries", func(b *testing.B) { // the count computed at each call
			b.ReportAllocs()
			for b.Loop() {
				m.zeroEntries(bt.t)
			}
		})
	}
}

// hugeChildEnv carries the plugin source of TestHugeLoadChild.
const hugeChildEnv = "YAEGI_HUGE_LOAD_SRC"

// hugeChildLimit is the heap size at which the watchdog of TestHugeLoadChild
// stops the child process. A normal load stays far below it.
const hugeChildLimit = 1 << 30

// TestHugeLoadChild is run by TestLoadHugeAlloc in a child process: it loads
// the plugin source of hugeChildEnv and prints the error. A watchdog ends the
// process when the heap passes hugeChildLimit, so a huge allocation that the
// VM does not refuse fails the test instead of taking down the machine.
func TestHugeLoadChild(t *testing.T) {
	src := os.Getenv(hugeChildEnv)
	if src == "" {
		t.Skip("helper process of TestLoadHugeAlloc")
	}
	startHeapWatchdog()
	dir, pkgPath := writePlugin(t, src)
	_, err := Load(Options{Dir: dir, AOT: os.Getenv(hugeChildAOTEnv) != ""}, pkgPath)
	fmt.Printf("RESULT: %v\n", err)
}

// convChildEnv names the case of TestNativeConvChild.
const convChildEnv = "YAEGI_HUGE_CONV_CASE"

// startHeapWatchdog ends the process when its heap passes hugeChildLimit.
func startHeapWatchdog() {
	go func() {
		s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		for {
			metrics.Read(s)
			if s[0].Value.Uint64() > hugeChildLimit {
				fmt.Printf("WATCHDOG: heap passed %d bytes\n", uint64(hugeChildLimit))
				os.Exit(3)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// nativeConvCases are native values that cost nothing in Go (zero-sized
// elements) but one Value per element in the VM, with the conversion that
// TestNativeConvChild runs on them.
var nativeConvCases = map[string]func(m *Machine) Value{
	"fromNativeSlice": func(m *Machine) Value {
		return m.fromNative(reflect.ValueOf(make([]struct{}, 1<<30)), types.NewSlice(types.NewStruct(nil, nil)))
	},
	"fromNativeArray": func(m *Machine) Value {
		rv := reflect.New(reflect.ArrayOf(1<<30, reflect.TypeFor[struct{}]())).Elem()
		return m.fromNative(rv, types.NewArray(types.NewStruct(nil, nil), 1<<30))
	},
	"fromNativeDeepSlice": func(m *Machine) Value {
		return m.fromNativeDeep(reflect.ValueOf(make([]struct{}, 1<<30)), types.NewSlice(types.NewStruct(nil, nil)))
	},
	"fromNativeDeepArray": func(m *Machine) Value {
		rv := reflect.New(reflect.ArrayOf(1<<30, reflect.TypeFor[struct{}]())).Elem()
		return m.fromNativeDeep(rv, types.NewArray(types.NewStruct(nil, nil), 1<<30))
	},
	"natToVMSlice": func(m *Machine) Value {
		return m.natToVM(reflect.ValueOf(make([]struct{}, 1<<30)), types.NewSlice(types.NewStruct(nil, nil)))
	},
	"retVMSlice": func(m *Machine) Value {
		return m.retVM(reflect.ValueOf(make([]struct{}, 1<<30)))
	},
	// The bound is on the whole conversion: the second slice alone is at
	// the limit, but the conversion already has a few entries.
	"natToVMNested": func(m *Machine) Value {
		s := [][]struct{}{make([]struct{}, 1), make([]struct{}, maxZeroElems)}
		return m.natToVM(reflect.ValueOf(s), types.NewSlice(types.NewSlice(types.NewStruct(nil, nil))))
	},
}

// TestNativeConvChild is run by TestNativeConvTooLarge in a child process:
// it runs the conversion of convChildEnv and prints the panic.
func TestNativeConvChild(t *testing.T) {
	name := os.Getenv(convChildEnv)
	if name == "" {
		t.Skip("helper process of TestNativeConvTooLarge")
	}
	startHeapWatchdog()
	m, _ := buildSrc(t, "package p\n")
	defer func() { fmt.Printf("RESULT: %v\n", recover()) }()
	nativeConvCases[name](m)
}

// TestNativeConvTooLarge checks that the conversions of a native value to
// the VM refuse a value that needs more than maxZeroElems Value entries, with
// a *LoadError panic before the allocation (the recover of Load and of the
// calls turns it into an error). Each case runs in a child process with a
// heap watchdog.
func TestNativeConvTooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("runs child processes")
	}
	for name := range nativeConvCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeConvChild$", "-test.count=1")
			cmd.Env = append(os.Environ(), convChildEnv+"="+name, "GOMEMLIMIT=1GiB")
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "RESULT: ") {
				t.Fatalf("child process failed (%v):\n%s", err, out)
			}
			if !strings.Contains(string(out), "too large") {
				t.Fatalf("want a panic mentioning the size, got:\n%s", out)
			}
		})
	}
}

// TestNatSeenGrow checks the count of natSeen.grow: it reaches the limit,
// panics past it, and does not overflow on a huge length.
func TestNatSeenGrow(t *testing.T) {
	panics := func(f func()) (p bool) {
		defer func() { p = recover() != nil }()
		f()
		return false
	}
	var s natSeen
	if panics(func() { s.grow(maxZeroElems - 1); s.grow(1) }) {
		t.Fatal("grow panics at the limit")
	}
	if !panics(func() { s.grow(1) }) {
		t.Fatal("grow does not panic past the limit")
	}
	var s2 natSeen
	s2.grow(10)
	if !panics(func() { s2.grow(math.MaxInt) }) {
		t.Fatal("grow does not panic on a huge length")
	}
}

// wordSized returns the source src of a huge value for this platform. On
// 32-bit platforms an array of 1<<34 or 1<<40 bytes does not type check
// (the length overflows int), and make([][1 << 20]byte, 1<<12) is 4 GiB,
// more than gc allows there: it panics with "makeslice: len out of range"
// before the VM counts the entries, as gc does. Smaller values that are
// still far above maxZeroElems test the same checks.
func wordSized(src string) string {
	if strconv.IntSize == 64 {
		return src
	}
	return strings.NewReplacer("1 << 40", "1 << 30", "1 << 34", "1 << 30", "F(1 << 12)", "F(1 << 10)").Replace(src)
}

// hugeChildAOTEnv, when set, makes TestHugeLoadChild load in AOT mode.
const hugeChildAOTEnv = "YAEGI_HUGE_LOAD_AOT"

// TestLoadHugeAlloc checks that each way to allocate a huge value in the VM
// is refused with an error, and not a fatal out-of-memory: globals, locals,
// new, composite literals, make of arrays (constant and run-time length),
// and the zero values of map lookups and channel receives, for arrays of
// plain elements (the flat path of compiler.mem) and for arrays hidden in
// structs, and the native arrays that the VM converts to its own values.
// Each case runs in a child process with a heap watchdog, with default
// options and in AOT mode.
func TestLoadHugeAlloc(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("runs child processes")
	}
	tests := []struct{ name, src string }{
		{"local", "func F() byte { var a [1 << 34]byte; return a[1] }\n\nvar Y = F()\n"},
		{"new", "var Y = new([1 << 34]byte)\n"},
		{"localLiteral", "func F() byte { a := [...]byte{1 << 34: 1}; return a[1] }\n\nvar Y = F()\n"},
		{"structInArray", "var X [1 << 20]struct{ a [1 << 20]byte }\n\nfunc F() byte { return X[1].a[2] }\n\nvar Y = F()\n"},
		{"structFields", "var X struct{ a, b, c [1 << 25]byte }\n\nfunc F() byte { return X.c[1] }\n\nvar Y = F()\n"},
		{"newStructInArray", "var Y = new([1 << 20]struct{ a [1 << 20]byte })\n"},
		{"makeConst", "var Y = make([][1 << 20]byte, 1 << 12)\n"},
		{"makeRuntime", "func F(n int) [][1 << 20]byte { return make([][1 << 20]byte, n) }\n\nvar Y = F(1 << 12)\n"},
		{"makeRuntimeStruct", "type S struct{ a, b [1 << 10]byte }\n\nfunc F(n int) []S { return make([]S, n) }\n\nvar Y = F(1 << 20)\n"},
		{"mapLookup", "var M map[int][1 << 20]struct{ a [1 << 20]byte }\n\nfunc F() byte { return M[0][1].a[2] }\n\nvar Y = F()\n"},
		{"chanRecv", "func F() byte { c := make(chan [1 << 20]struct{ a [1 << 20]byte }); close(c); v := <-c; return v[1].a[2] }\n\nvar Y = F()\n"},
		// A native array loaded into the VM: [N]byte(s) of a []byte goes
		// through a native pointer (sliceToArrayPointer), and the load of
		// that pointer converts the array to one Value per byte. The
		// []byte itself is lazy (no page is touched).
		{"nativeArrayIface", "func F() any { s := make([]byte, 1<<28); return [1 << 28]byte(s) }\n\nvar Y = F()\n"},
		{"nativeArrayMap", "var M = map[int][1 << 28]byte{}\n\nfunc F() int { s := make([]byte, 1<<28); M[0] = [1 << 28]byte(s); return len(M) }\n\nvar Y = F()\n"},
		{"nativeArrayChan", "func F() int { s := make([]byte, 1<<28); c := make(chan [1 << 28]byte, 1); c <- [1 << 28]byte(s); return len(c) }\n\nvar Y = F()\n"},
		{"nativeArrayOneOver", "func F() any { s := make([]byte, 1<<26+1); return [1<<26 + 1]byte(s) }\n\nvar Y = F()\n"},
		{"nativeArrayBig", "func F() any { s := make([]byte, 1<<29); return [1 << 29]byte(s) }\n\nvar Y = F()\n"},
	}
	for _, aot := range []bool{false, true} {
		for _, tt := range tests {
			name := tt.name
			if aot {
				name += "/aot"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHugeLoadChild$", "-test.count=1")
				cmd.Env = append(os.Environ(), hugeChildEnv+"=package p\n\n"+wordSized(tt.src), "GOMEMLIMIT=1GiB")
				if aot {
					cmd.Env = append(cmd.Env, hugeChildAOTEnv+"=1")
				}
				out, err := cmd.CombinedOutput()
				if err != nil || !strings.Contains(string(out), "RESULT: ") {
					t.Fatalf("child process failed (%v):\n%s", err, out)
				}
				if !strings.Contains(string(out), "too large") {
					t.Fatalf("want a load error mentioning the size, got:\n%s", out)
				}
			})
		}
	}
}

// loadPanicGCEnv, when set, makes TestLoadPanicGCChild run.
const loadPanicGCEnv = "YAEGI_LOAD_PANIC_GC"

// TestLoadPanicGCChild is run by TestLoadPanicGC in a child process: it
// loads plugins whose init frees the directory string of the load (the only
// reference left is in dead Options copies on the stack) and then panics,
// with a garbage collection while the load recovers the panic. It prints
// the error of each load.
func TestLoadPanicGCChild(t *testing.T) {
	if os.Getenv(loadPanicGCEnv) == "" {
		t.Skip("helper process of TestLoadPanicGC")
	}
	hook := func(stage string) {
		if stage == "recovered" {
			runtime.GC()
		}
	}
	for i := 0; i < 5; i++ {
		dir, pkgPath := writePlugin(t, "package p\n\nimport \"runtime\"\n\nfunc init() { runtime.GC(); panic(\"boom\") }\n")
		_, err := Load(Options{Dir: dir, Unrestricted: true, hooks: &loadHooks{stage: hook}}, pkgPath)
		fmt.Printf("RESULT: %v\n", err)
	}
}

// TestLoadPanicGC checks that a load does not corrupt the heap when the GC
// runs while it recovers a panic of the package init. The Options were
// passed by value down to the frame that recovers, and the closures of the
// load took their address: with the runtime bug go.dev/issue/82394, the
// stack scan of a panicking goroutine follows the frame pointer kept in
// the _panic record and finds that dead copy, whose Dir string is already
// freed: "fatal error: found pointer to free object" (linux/amd64, the race
// shard of CI). The load now takes the Options by pointer (see loadImpl).
// The child process runs the GC at the exact point (loadHooks.stage).
func TestLoadPanicGC(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("runs a child process")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLoadPanicGCChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), loadPanicGCEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil || strings.Count(string(out), "RESULT: vm load: panic: boom") != 5 {
		t.Fatalf("child process failed (%v):\n%s", err, out)
	}
}

// TestLoadCancelBeforeInit checks that a load whose deadline passes at the
// end of the SSA build, or during the preflight, does not run the plugin's
// init. One ssa.Package.Build cannot be interrupted, so the deadline can pass
// while it runs: LoadContext then returns the error, and the abandoned load
// must stop before it runs plugin code (here an init that writes a file).
// The test cancels the context at an exact stage (loadHooks.stage) and runs
// the load synchronously (loadImpl), so it does not depend on timing.
func TestLoadCancelBeforeInit(t *testing.T) {
	needSource(t)
	for _, golist := range []bool{false, true} {
		for _, stage := range []string{"built", "checked"} {
			t.Run(fmt.Sprintf("golist=%v/%s", golist, stage), func(t *testing.T) {
				mark := filepath.Join(t.TempDir(), "init-ran")
				src := fmt.Sprintf("package p\n\nimport \"os\"\n\nfunc init() { os.WriteFile(%q, []byte(\"x\"), 0o644) }\n", mark)
				dir, pkgPath := writePlugin(t, src)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				hook := func(s string) {
					if s == stage {
						cancel()
					}
				}
				_, err := loadImpl(ctx, &Options{Dir: dir, Unrestricted: true, GoList: golist, hooks: &loadHooks{stage: hook}}, pkgPath)
				var le *LoadError
				if !errors.As(err, &le) || !errors.Is(ctx.Err(), context.Canceled) {
					t.Fatalf("want a *LoadError for the cancelled load, got %v", err)
				}
				if _, err := os.Stat(mark); err == nil {
					t.Fatal("the init of the plugin ran after the load was cancelled")
				}
			})
		}
	}
}
