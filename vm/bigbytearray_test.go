package vm

import (
	"context"
	"errors"
	"go/types"
	"math"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// Tests of the [n]byte variables in real bytes (bigByteArray in
// byteview.go). The programs of testdata/bigbytearray*.go check the
// semantics against gc.

// bigLoadSrc loads a big byte array twice: each load decodes it into a
// []Value of 24 bytes per byte.
const bigLoadSrc = `package p

import "fmt"

var (
	keep *[1 << 26]byte
	sink bool
)

func Load() (s string) {
	defer func() { s = fmt.Sprint(recover()) }()
	keep = new([1 << 26]byte)
	sink = *keep == *keep
	return "no panic"
}
`

// TestBigByteArrayLoadQuota checks that the load of a big byte array is
// counted in MaxMemory before it is decoded. The array itself counts 64
// MiB, but each load made 1.5 GiB of Values that the quota did not see:
// a few goroutines that compared such arrays made the host run out of
// memory, under a quota of 256 MiB.
func TestBigByteArrayLoadQuota(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("allocates 64 MiB")
	}
	dir, pkg := writePlugin(t, bigLoadSrc)
	p, err := Load(Options{Dir: dir, MaxMemory: 256 << 20}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Call(context.Background(), "Load")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := res[0].Interface(); err != nil || got != ErrMemory.Error() {
		t.Fatalf("Load() = %v (%v), want the panic %q", got, err, ErrMemory)
	}
}

// TestBigByteArrayIndexAllocs checks that a[i] of a [256]byte whose address
// escapes (in real bytes from 64 bytes, bigByteArray) reads and writes the
// byte in place. Each access made a new byteView: 2 allocations per
// iteration of this loop, where an array in cells made none. The array
// escapes: a local that does not escape stays a frame-local cell
// (TestBigByteArrayLocalQuota).
func TestBigByteArrayIndexAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates on its own: the counts mean nothing")
	}
	const src = `package p

var keep *[256]byte

func F(n int) (s int) {
	var buf [256]byte
	keep = &buf
	for i := 0; i < n; i++ {
		buf[i&255] = byte(i)
		s += int(buf[(i*7)&255])
	}
	return
}
`
	m, spkg := buildSrc(t, src)
	if b, c := bigByteAllocs(m, spkg.Func("F")); b != 1 || c != 0 {
		t.Fatalf("F: %d big byte arrays in real bytes and %d in cells, want 1 and 0", b, c)
	}
	fn := m.funcOf(spkg.Func("F"))
	args := []Value{{N: 1000}}
	if got := m.Call(fn, args, nil).N; got != 105660 {
		t.Fatalf("F(1000) = %d, want 105660", got)
	}
	n := math.Inf(1)
	for range 3 {
		n = min(n, testing.AllocsPerRun(20, func() { m.Call(fn, args, nil) }))
	}
	// A few allocations per call (the array), none per iteration.
	if n > 20 {
		t.Errorf("F(1000): %.0f allocations per call, want at most 20", n)
	}
}

// bigByteAllocs counts the [n]byte variables of fn with n >= 64
// (bigByteArray) that bytesAlloc puts in real bytes and in cells.
func bigByteAllocs(m *Machine, fn *ssa.Function) (inBytes, inCells int) {
	c := &compiler{m: m, f: &Func{m: m, ssa: fn}, regs: map[ssa.Value]int32{}, blockPC: map[*ssa.BasicBlock]int{}, fused: map[ssa.Value]bool{}}
	for _, b := range fn.Blocks {
		for _, in := range b.Instrs {
			a, ok := in.(*ssa.Alloc)
			if !ok || !bigByteArray(a.Type().(*types.Pointer).Elem()) {
				continue
			}
			if c.bytesAlloc(a) {
				inBytes++
			} else {
				inCells++
			}
		}
	}
	return inBytes, inCells
}

// bigLocalSrc is a recursion with a local [4096]byte that does not escape:
// 1000 frames hold 4 MB of bytes, but 98 MB of cells (a Value of 24 bytes
// per byte; 65 MB with a Value of 16 bytes).
const bigLocalSrc = `package p

func f(n int) int {
	var b [4096]byte
	b[n%4096] = byte(n)
	if n == 0 {
		return int(b[0])
	}
	return f(n-1) + int(b[n%4096])
}

func F(n int) int { return f(n) }
`

// TestBigByteArrayLocalQuota checks that a local [n]byte that does not
// escape stays a frame-local cell, counted in MaxStack and in MaxMemory.
// In real bytes (bigByteArray), it was a new []byte per call that neither
// quota counted (below memMin, an allocation is not counted): a recursion
// with a local [4096]byte passed a MaxStack or a MaxMemory of 16 MiB with
// 1000 frames, where each frame holds 4097 Values of cells.
func TestBigByteArrayLocalQuota(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, bigLocalSrc)
	for _, tt := range []struct {
		name string
		opts Options
		want error
	}{
		{"MaxStack", Options{Dir: dir, MaxStack: 16 << 20}, ErrStackOverflow},
		{"MaxMemory", Options{Dir: dir, MaxStack: -1, MaxMemory: 16 << 20}, ErrMemory},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Load(tt.opts, pkg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Call(context.Background(), "F", 1000); !errors.Is(err, tt.want) {
				t.Fatalf("F(1000): %v, want %v", err, tt.want)
			}
			if f := stackFunc(t, p, "f"); len(f.locals) != 1 {
				t.Fatalf("f: %d frame-local cells, want 1", len(f.locals))
			}
			// A short recursion fits in both quotas.
			res, err := p.Call(context.Background(), "F", 10)
			if err != nil {
				t.Fatalf("F(10): %v", err)
			}
			if v, _ := res[0].Interface(); v != 55 {
				t.Fatalf("F(10) = %v, want 55", v)
			}
		})
	}
}

// TestBigByteArrayHostElem checks that Elem of a pointer to a [64]byte
// laid over host bytes (a view of UseUnsafe, hostmem.go) is a copy that
// reflect cannot set. It was addressable, and Index(0).SetUint wrote the
// byte through byteElem without the host check that Set has.
func TestBigByteArrayHostElem(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, "package p\n\nimport \"reflect\"\n\nfunc F(x any) string { return reflect.TypeOf(x).String() }\n")
	p, err := Load(Options{Dir: dir}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	if p.M.vr == nil {
		t.Fatal("no VM reflect")
	}
	at := types.NewArray(types.Typ[types.Uint8], 64)
	host := make([]byte, 64) // stands for host memory: the view says so
	ptr := mkVM(p.M.vr.vtypeOf(types.NewPointer(at)), &Value{R: &byteView{b: host, t: at, host: true}}, 0)
	e := ptr.Elem()
	if e.CanAddr() || e.CanSet() || e.Index(0).CanSet() {
		t.Errorf("Elem of a view over host bytes: CanAddr %v, CanSet %v, Index(0).CanSet %v; want false",
			e.CanAddr(), e.CanSet(), e.Index(0).CanSet())
	}
	func() {
		defer func() { _ = recover() }()
		e.Index(0).SetUint(1)
	}()
	if host[0] != 0 {
		t.Errorf("SetUint wrote the host byte: %d", host[0])
	}
}
