package vm

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

// TestFrameLocal checks which locals keep their cell in the frame
// (escape.go): those whose address only goes to loads, stores, field and
// element addresses, and parameters of interpreted functions that do not
// keep it. Everything else gets a new cell at each run (OpAlloc).
// testdata/framelocal.go checks the results against go run.
func TestFrameLocal(t *testing.T) {
	const src = `package p

import "fmt"

type Op struct{ Kind int; Arg string }
type Rule struct{ ID int; Ops [3]Op }

func read(op *Op) bool { return op.Kind == 1 }
func write(r *Rule) { r.ID = 1; r.Ops[0].Kind = 2 }
func sum(b []int) (s int) { for _, x := range b { s += x }; return }
func both(r *Rule) bool { write(r); return read(&r.Ops[1]) }

var g *Rule
func store(r *Rule) { g = r }
func ret(r *Rule) *Rule { return r }
func viaStore(r *Rule) { store(r) }
func self(r *Rule, n int) int { if n == 0 { return r.ID }; return self(r, n-1) }

func Local(rules []Rule) (n int) {
	for _, r := range rules {
		for j := range r.Ops {
			if read(&r.Ops[j]) { n++ }
		}
		var x Rule
		write(&x)
		if both(&x) { n++ }
		var a [4]int
		n += sum(a[:])
	}
	return n
}

func Stored() { var r Rule; store(&r) }
func StoredDeep() { var r Rule; viaStore(&r) }
func Returned() *Rule { var r Rule; return ret(&r) }
func Native() { var r Rule; fmt.Println(&r) }
func Iface() any { var r Rule; return &r }
func Closure() func() int { var r Rule; return func() int { return r.ID } }
func FuncValue(f func(*Rule)) { var r Rule; f(&r) }
func Recursive() int { var r Rule; return self(&r, 3) }
func Phi(n int) *Rule {
	var p *Rule
	for i := 0; i < n; i++ { var r Rule; r.ID = i; if p != nil { r.ID += p.ID }; p = &r }
	return p
}
func Appended() []int { var a [4]int; return append(a[:0], 1) }

func c1(r *Rule) int { return c2(r) }
func c2(r *Rule) int { return c3(r) }
func c3(r *Rule) int { return c4(r) }
func c4(r *Rule) int { return c5(r) }
func c5(r *Rule) int { return c6(r) }
func c6(r *Rule) int { return c7(r) }
func c7(r *Rule) int { return c8(r) }
func c8(r *Rule) int { return c9(r) }
func c9(r *Rule) int { return c10(r) }
func c10(r *Rule) int { return r.ID }
func Chain() int { var r Rule; return c1(&r) }

func s1(r *Rule) { s2(r) }
func s2(r *Rule) { s3(r) }
func s3(r *Rule) { s4(r) }
func s4(r *Rule) { s5(r) }
func s5(r *Rule) { s6(r) }
func s6(r *Rule) { s7(r) }
func s7(r *Rule) { s8(r) }
func s8(r *Rule) { s9(r) }
func s9(r *Rule) { s10(r) }
func s10(r *Rule) { g = r }
func ChainStored() { var r Rule; s1(&r) }

func ping(r *Rule, n int) int { if n == 0 { return r.ID }; return pong(r, n-1) }
func pong(r *Rule, n int) int { return ping(r, n) }
func Cycle() int { var r Rule; return ping(&r, 3) }

func kping(r *Rule, n int) { if n == 0 { g = r; return }; kpong(r, n-1) }
func kpong(r *Rule, n int) { kping(r, n) }
func CycleStored() { var r Rule; kpong(&r, 3) }

func la(r *Rule, n int) int { if n > 0 { return lb(r, n-1) }; return r.ID }
func lb(r *Rule, n int) int { x := la(r, n); lc(r, n); return x }
func lc(r *Rule, n int) { if n == 7 { ld(r, n) } }
func ld(r *Rule, n int) { if n > 0 { lc(r, n-1); return }; keep(r) }
func keep(r *Rule) { g = r }
func CycleLate() int { var r Rule; return la(&r, 3) }
`
	m, spkg := buildSrc(t, src)
	count := func(name string) (local, heap int) {
		fn := m.funcOf(spkg.Func(name))
		fn.ensure()
		for _, in := range fn.code {
			switch in.Op {
			case OpAllocLocal:
				local++
			case OpAlloc:
				heap++
			}
		}
		return
	}
	for _, c := range []struct {
		name        string
		local, heap int
	}{
		{"Local", 3, 0},
		{"Stored", 0, 1},
		{"StoredDeep", 0, 1},
		{"Returned", 0, 1},
		{"Native", 0, 2}, // &r and the array of the ...any arguments
		{"Iface", 0, 1},
		{"Closure", 0, 1},
		{"FuncValue", 0, 1},
		// A recursion that only passes the pointer down and reads it
		// (escape.go: the summaries of a recursion are a fixed point).
		{"Recursive", 1, 0},
		{"Phi", 0, 1},
		{"Appended", 0, 1},
		// A chain of 10 callees that only read the pointer: each summary
		// has its own depth budget, so the depth of the chain does not
		// matter.
		{"Chain", 1, 0},
		{"ChainStored", 0, 1},
		{"Cycle", 1, 0},
		// A cycle that stores the pointer, at the end of the recursion or
		// in a function that the cycle only reaches through another one.
		{"CycleStored", 0, 1},
		{"CycleLate", 0, 1},
	} {
		if l, h := count(c.name); l != c.local || h != c.heap {
			t.Errorf("%s: %d frame-local and %d new cells, want %d and %d", c.name, l, h, c.local, c.heap)
		}
	}
}

// TestFrameLocalAllocs checks that a struct local of a loop body whose
// address goes to a function that only reads it is not a new object at
// each iteration (the pattern of the hot loops of the Coraza plugin).
func TestFrameLocalAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates on its own: the counts mean nothing")
	}
	const src = `package p

type Op struct{ Kind int; Arg string; Negate bool }
type Rule struct{ ID, Phase int; Ops [3]Op; Disabled bool }

var rules = make([]Rule, 100)

func eval(op *Op) bool { return op.Kind == 1 != op.Negate }

func Run() int {
	n := 0
	for _, r := range rules {
		for j := range r.Ops {
			if eval(&r.Ops[j]) { n++ }
		}
	}
	return n
}
`
	m, spkg := buildSrc(t, src)
	m.Call(m.funcOf(spkg.Func("init")), nil, nil)
	fn := m.funcOf(spkg.Func("Run"))
	m.Call(fn, nil, nil)
	got := testing.AllocsPerRun(20, func() { m.Call(fn, nil, nil) })
	// One frame-local cell per call (a Rule: the cell, the fields, the
	// array and its 3 Op), plus the call itself; 100 times that without.
	if got > 20 {
		t.Errorf("%.0f allocations per call of a loop over 100 rules, want at most 20", got)
	}
}

// TestStaticCallRunsBytecode checks that the predicate the escape
// summaries trust (staticCallRunsBytecode) says true for each kind of
// static callee exactly when compiler.call emits OpCall for it.
func TestStaticCallRunsBytecode(t *testing.T) {
	const src = `package p

import (
	"math/rand/v2"
	"slices"
	"strings"
)

type T struct{ n int }

func (t *T) get() int { return t.n }

func id[E any](x E) E { return x }

func plain(x int) int { return x }

func Plain() int { return plain(1) }
func Method() int { var t T; return t.get() }
func Generic() int { return id(1) }
func Native() string { return strings.ToUpper("a") }
func StdGeneric() bool { return slices.Contains([]int{1}, 1) }
func Intrinsic() int { return rand.N(10) }
`
	m, spkg := buildSrc(t, src)
	for _, c := range []struct {
		name     string
		bytecode bool
	}{
		{"Plain", true},
		{"Method", true},
		{"Generic", true},    // an instance of an interpreted generic function
		{"StdGeneric", true}, // a generic of the standard library, from its source
		{"Native", false},    // OpCallNative
		{"Intrinsic", false}, // a generic intrinsic (OpFunc)
	} {
		name := c.name
		sf := spkg.Func(name)
		var callee *ssa.Function
		for _, b := range sf.Blocks {
			for _, ins := range b.Instrs {
				if call, ok := ins.(*ssa.Call); ok {
					if f, ok := call.Call.Value.(*ssa.Function); ok {
						callee = f
					}
				}
			}
		}
		if callee == nil {
			t.Fatalf("%s: no static call", name)
		}
		fn := m.funcOf(sf)
		fn.ensure()
		calls := 0
		for _, in := range fn.code {
			if in.Op == OpCall {
				calls++
			}
		}
		pred := m.staticCallRunsBytecode(callee)
		if got := calls == 1; got != pred || got != c.bytecode {
			t.Errorf("%s: OpCall emitted %v, staticCallRunsBytecode(%s) = %v, want %v", name, got, callee, pred, c.bytecode)
		}
	}
}

// TestFreeCells checks the free lists of the cells of frame-local Allocs
// (escape.go): a call takes the cell that a returned frame gave back, set
// to zero; two Machines that run on the same pooled ctx never share a
// cell, though their sites have the same index; and the cells kept by a
// ctx hold at most maxFreeCells Values.
func TestFreeCells(t *testing.T) {
	const src = `package p

type R struct{ a, b, c int }

func set(r *R, n int) { r.a, r.b, r.c = n, n+1, n+2 }

func F(n int) int {
	var r R
	if r.a != 0 || r.b != 0 || r.c != 0 { panic("not zero") }
	set(&r, n)
	return r.a + r.b + r.c
}
`
	const srcB = `package p

func fill(a *[5]string, s string) { for i := range a { a[i] = s } }

func F(s string) string {
	var a [5]string
	for _, x := range a { if x != "" { panic("not zero") } }
	fill(&a, s)
	return a[0] + a[4]
}
`
	ma, pa := buildSrc(t, src)
	mb, pb := buildSrc(t, srcB)
	fa, fb := ma.funcOf(pa.Func("F")), mb.funcOf(pb.Func("F"))
	for i := 0; i < 20; i++ {
		if got := ma.Call(fa, []Value{{N: int64(i)}}, nil).N; got != int64(3*i+3) {
			t.Fatalf("A: F(%d) = %d, want %d", i, got, 3*i+3)
		}
		if got, _ := mb.Call(fb, []Value{{R: "x"}}, nil).R.(string); got != "xx" {
			t.Fatalf("B: F = %q, want xx", got)
		}
	}

	// The free lists of one ctx, run by hand.
	s1 := &memSite{flat: 4, local: 0, localSize: 5}
	s2 := &memSite{flat: 2, local: 0, localSize: 3} // the same index, another Machine
	fn := &Func{locals: []localCell{{reg: 0, site: s1}}}
	cx := &ctx{}
	regs := []Value{{R: newCell(s1)}}
	cell := regs[0].R.(*Value)
	cell.R.([]Value)[3] = Value{N: 7}
	cx.freeLocals(fn, regs)
	if regs[0] != (Value{}) || cx.freeSize != 5 {
		t.Fatalf("after a return: register %v, %d Values kept, want an empty register and 5", regs[0], cx.freeSize)
	}
	if got := cx.reuseCell(s2); got != nil {
		t.Fatalf("a cell of another site: %v", got)
	}
	if got := cx.reuseCell(s1); got != cell || got.R.([]Value)[3] != (Value{}) {
		t.Fatalf("reuseCell = %p %v, want the same cell %p, zero", got, got.R, cell)
	}
	if cx.freeSize != 0 {
		t.Fatalf("%d Values kept after the cell was taken back", cx.freeSize)
	}
	// A list of another site is dropped when a cell of this site comes back.
	cx.freeLocals(fn, []Value{{R: newCell(s1)}})
	cx.freeLocals(&Func{locals: []localCell{{reg: 0, site: s2}}}, []Value{{R: newCell(s2)}})
	if cx.freeSize != 3 || cx.reuseCell(s1) != nil || cx.reuseCell(s2) == nil {
		t.Fatalf("after a cell of another Machine: %d Values kept, want 3, and only cells of s2", cx.freeSize)
	}
	// The bound.
	big := &memSite{flat: 1000, local: 1, localSize: 1001}
	fbig := &Func{locals: []localCell{{reg: 0, site: big}}}
	for i := 0; i < 2*maxFreeCells/1001; i++ {
		cx.freeLocals(fbig, []Value{{R: newCell(big)}})
	}
	if cx.freeSize > maxFreeCells || cx.freeSize < maxFreeCells-1001 {
		t.Errorf("free cells hold %d Values, want at most %d, and not much less", cx.freeSize, maxFreeCells)
	}
}
