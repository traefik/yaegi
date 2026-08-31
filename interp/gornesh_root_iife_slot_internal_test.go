package interp

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

const gorneshIIFETestTimeout = 3 * time.Second

// globalFrameSlotsGornesh reports the size of the durable global frame, which
// grows only when compilation allocates persistent slots in the global scope.
func globalFrameSlotsGornesh(t *testing.T, i *Interpreter) int {
	t.Helper()
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	i.frame.mutex.RLock()
	defer i.frame.mutex.RUnlock()
	return len(i.frame.data)
}

// TestGorneshRootIIFEEvalAddsNoGlobalSlot pins the func-leading REPL decision:
// an incremental Eval whose statement is an immediately-invoked function
// literal must not permanently extend the global frame (LEARNINGS 2026-08-30,
// "func-leading REPL Eval slot growth").
func TestGorneshRootIIFEEvalAddsNoGlobalSlot(t *testing.T) {
	i := New(Options{})
	base := globalFrameSlotsGornesh(t, i)
	const runs = 200
	for k := 0; k < runs; k++ {
		if _, err := i.Eval(`func(){ _x := 1; _ = _x }()`); err != nil {
			t.Fatal(err)
		}
	}
	after := globalFrameSlotsGornesh(t, i)
	if after != base {
		t.Fatalf("void IIFE Evals grew the global frame by %d slots (%d -> %d), want 0", after-base, base, after)
	}
}

// TestGorneshRootIIFEEvalResultSlotMatchesExprBaseline: a result-bearing IIFE
// must not add the literal's own registry slots on top of the ordinary
// call-expression result slot. Its per-Eval growth must equal a plain
// `len("ab")` style root-level call Eval (the pre-existing expression-slot
// mechanism: exactly one result slot per Eval), not the +3/Eval of the
// registry path.
func TestGorneshRootIIFEEvalResultSlotMatchesExprBaseline(t *testing.T) {
	i := New(Options{})
	const runs = 100
	base := globalFrameSlotsGornesh(t, i)
	for k := 0; k < runs; k++ {
		if _, err := i.Eval(`len("ab")`); err != nil {
			t.Fatal(err)
		}
	}
	exprGrowth := globalFrameSlotsGornesh(t, i) - base

	j := New(Options{})
	jbase := globalFrameSlotsGornesh(t, j)
	for k := 0; k < runs; k++ {
		if _, err := j.Eval(`func() int { return 7 }()`); err != nil {
			t.Fatal(err)
		}
	}
	after := globalFrameSlotsGornesh(t, j)
	if after-jbase != exprGrowth {
		t.Fatalf("result IIFE Evals grew the global frame by %d slots over %d Evals, want exactly the %d of an ordinary call expression", after-jbase, runs, exprGrowth)
	}
}

// TestGorneshRootIIFEEvalSemantics pins the observable behavior of root-level
// immediately invoked literals after the codegen change.
func TestGorneshRootIIFEEvalSemantics(t *testing.T) {
	i := New(Options{})

	// Side effects and argument passing.
	if _, err := i.Eval(`
sum := 0
func(a, b int) { sum = a + b }(20, 22)
`); err != nil {
		t.Fatal(err)
	}
	v, err := i.Eval("sum")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Interface(); got != 42 {
		t.Fatalf("IIFE args/side effect: got %v, want 42", got)
	}

	// Result value.
	v, err = i.Eval(`func() int { return 41 + 1 }()`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Interface(); got != 42 {
		t.Fatalf("IIFE result: got %v, want 42", got)
	}

	// Variadic form.
	v, err = i.Eval(`func(vs ...int) int { t := 0; for _, c := range vs { t += c }; return t }(1, 2, 3, 6, 30)`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Interface(); got != 42 {
		t.Fatalf("variadic IIFE result: got %v, want 42", got)
	}

	// Globals remain visible inside the literal (root-frame access path).
	if _, err := i.Eval(`
base := 40
r := func() int { return base + 2 }()
`); err != nil {
		t.Fatal(err)
	}
	v, err = i.Eval("r")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Interface(); got != 42 {
		t.Fatalf("IIFE global read: got %v, want 42", got)
	}

	// Closures declared at root level keep working (persistent slot path).
	if _, err := i.Eval(`
mk := func() func() int { c := 0; return func() int { c++; return c } }
next := mk()
a1 := next()
a2 := next()
`); err != nil {
		t.Fatal(err)
	}
	v, err = i.Eval("a2")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Interface(); got != 2 {
		t.Fatalf("closure state across calls: got %v, want 2", got)
	}

	// go-statement IIFE executes.
	doneGo := make(chan struct{})
	i2 := New(Options{})
	if err := i2.Use(Exports{"iifetest/iifetest": {
		"Done": reflect.ValueOf(func() { close(doneGo) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i2.Eval(`import "iifetest"; go func(){ iifetest.Done() }()`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-doneGo:
	case <-time.After(gorneshIIFETestTimeout):
		t.Fatal("go IIFE never ran")
	}

	// Deferred root-level IIFE keeps the wrapper path (excluded from the
	// direct activation), and the deferred call still runs.
	doneDefer := make(chan struct{})
	i3 := New(Options{})
	if err := i3.Use(Exports{"iifetest/iifetest": {
		"Done": reflect.ValueOf(func() { close(doneDefer) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i3.Eval(`import "iifetest"; defer func(){ iifetest.Done() }(); 1`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-doneDefer:
	case <-time.After(gorneshIIFETestTimeout):
		t.Fatal("deferred IIFE never ran")
	}

	// An IIFE inside a root-level control body shares the global frame, so it
	// also takes the direct activation and must not grow the frame.
	i4 := New(Options{})

	v4, err := i4.Eval(`
total := 0
if true { total = func() int { return 42 }() }
total
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v4.Interface(); got != 42 {
		t.Fatalf("IIFE in root-level if: got %v, want 42", got)
	}
	loopBase := globalFrameSlotsGornesh(t, i4)
	for k := 0; k < 100; k++ {
		if _, err := i4.Eval(`if true { func(){ _q := 1; _ = _q }() }`); err != nil {
			t.Fatal(err)
		}
	}
	if after := globalFrameSlotsGornesh(t, i4); after != loopBase {
		t.Fatalf("void IIFEs in root-level if bodies grew the global frame by %d slots over 100 Evals", after-loopBase)
	}

	// A panic inside a root-level IIFE propagates as an interpretable panic.
	_, perr := New(Options{}).Eval(`func(){ panic("boom") }()`)
	if perr == nil {
		t.Fatal("IIFE panic was swallowed")
	}
	if pe, ok := perr.(Panic); !ok || fmt.Sprint(pe.Value) != "boom" {
		t.Fatalf("IIFE panic error = %v (%T), want Panic{boom}", perr, perr)
	}
}
