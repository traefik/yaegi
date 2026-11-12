package vm

// flag.BoolVar, IntVar, DurationVar... with a variable of the program.
//
// The pointer to the variable crosses the boundary as a native copy (copied
// in and out around the call). flag keeps the pointer in its *boolValue and
// Parse writes it later: the variable of the program was never set. A
// variable of these kinds is all in the N word of its cell (value.go), with
// the layout of the native type: int64, uint64 and Duration are 8-byte
// integers, float64 is its bits, and bool is 0 or 1. The XxxVar calls give
// flag a native pointer to that word, so flag reads and writes the variable
// of the program itself, as in Go: no copy to keep in sync.
//
// On a 32-bit platform, int and uint are 4 bytes: flag gets a pointer to
// the low half of the word. A uint keeps a zero high half. An int must be
// sign extended in the word (intsize.go), and flag only writes the low
// half: the machine keeps these cells (int32Cells) and sign extends them
// again each time interpreted code runs after native code (ctx.checkFault,
// callCtx). On a 64-bit platform this code is removed (intIs32 is a
// constant).
//
// A string is in the R word of its cell, which has not the layout of a
// native string: StringVar gives flag a Value (Var) that reads and writes
// the cell. PrintDefaults then shows "-s value" where Go shows "-s
// string", and the default without quotes. TextVar gives flag a proxy whose
// MarshalText and UnmarshalText call the methods of the variable in its
// cell (a type of the program, or a native type such as net.IP). The Get
// method of that flag (flag.Getter) returns the proxy, not the variable.
//
// The methods of *flag.FlagSet are hooked on direct, deferred and go calls
// (storageHook): a call through an interface, a method value or reflect's
// MethodByName still gives flag a native copy.

import (
	"encoding"
	"fmt"
	"go/types"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
	"weak"

	"golang.org/x/tools/go/ssa"
)

type flagKind interface {
	bool | int | int64 | uint | uint64 | float64 | time.Duration
}

// lowOffset returns the offset in an int64 of the low size bytes: the
// bytes where a *T of that size sees the value.
func lowOffset(size uintptr) uintptr {
	x := int64(1)
	if *(*byte)(unsafe.Pointer(&x)) == 1 {
		return 0 // little endian
	}
	return 8 - size
}

// cellPtr returns a native pointer of type *T to the N word of cell (to
// its low bytes when a T is smaller than 8 bytes: bool, and int and uint
// on a 32-bit platform).
func cellPtr[T flagKind](cell *Value) *T {
	return (*T)(unsafe.Add(unsafe.Pointer(&cell.N), lowOffset(unsafe.Sizeof(*new(T)))))
}

// int32Cells is the list of the int cells that flag writes on a 32-bit
// platform (see the top of this file). The pointers are weak: a cell that
// the program and flag no longer use is dropped.
type int32Cells struct {
	mu    sync.Mutex
	cells atomic.Pointer[[]weak.Pointer[Value]]
}

// add records cell and sign extends it at once.
func (c *int32Cells) add(cell *Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var next []weak.Pointer[Value]
	if old := c.cells.Load(); old != nil {
		for _, w := range *old {
			if w.Value() != nil {
				next = append(next, w)
			}
		}
	}
	next = append(next, weak.Make(cell))
	c.cells.Store(&next)
	signExtend32(cell)
}

// fix sign extends the cells again, after native code that may have
// written their low half. It does nothing on a 64-bit platform.
func (c *int32Cells) fix() {
	if !intIs32 {
		return
	}
	p := c.cells.Load()
	if p == nil {
		return
	}
	for _, w := range *p {
		if cell := w.Value(); cell != nil {
			signExtend32(cell)
		}
	}
}

// signExtend32 makes the N word of cell the sign extension of its low
// 32 bits. It writes only when the word changes.
func signExtend32(cell *Value) {
	low := *(*int32)(unsafe.Add(unsafe.Pointer(&cell.N), lowOffset(4)))
	if v := int64(low); cell.N != v {
		cell.N = v
	}
}

// flagVarHook implements XxxVar(p, name, value, usage) of flag, or of a
// *flag.FlagSet when method is set (a[0] is the receiver). A pointer that
// is not a cell of the program (a variable of native memory) is given as
// it is.
func flagVarHook[T flagKind](method bool) storageHook {
	return func(m *Machine, f *ssa.Function, a []Value) Value {
		i := 0
		if method {
			i = 1
		}
		if cell, ok := a[i].R.(*Value); ok {
			a = append([]Value(nil), a...)
			a[i] = Value{R: reflect.ValueOf(cellPtr[T](cell))}
			if _, isInt := any(*new(T)).(int); isInt && intIs32 {
				defer m.flagInts.add(cell) // after flag wrote the default value
			}
		}
		return m.callFlag(f, a)
	}
}

// cellString is the flag.Value of StringVar on a string variable of the
// program: flag reads and writes the cell itself.
type cellString struct{ cell *Value }

func (s *cellString) String() string {
	if s == nil || s.cell == nil {
		return "" // the zero Value of isZeroValue: reflect.New(cellString)
	}
	v, _ := s.cell.R.(string)
	return v
}

// Get implements flag.Getter, as the *stringValue of flag.
func (s *cellString) Get() any { return s.String() }

func (s *cellString) Set(v string) error {
	*s.cell = Value{R: v}
	return nil
}

// stringVarHook implements StringVar(p, name, value, usage) of flag, or of a
// *flag.FlagSet when method is set, with Var and a cellString.
func stringVarHook(method bool) storageHook {
	return func(m *Machine, f *ssa.Function, a []Value) Value {
		i := 0
		if method {
			i = 1
		}
		cell, ok := a[i].R.(*Value)
		if !ok {
			return m.callFlag(f, a)
		}
		*cell = a[i+2]
		args := append(append([]Value(nil), a[:i]...), Value{R: reflect.ValueOf(&cellString{cell})}, a[i+1], a[i+3])
		return m.callFlag(flagSibling(m, f, "Var"), args)
	}
}

// flagSibling returns the function name of package flag, or the method
// name of *flag.FlagSet when f is a method.
func flagSibling(m *Machine, f *ssa.Function, name string) *ssa.Function {
	recv := f.Signature.Recv()
	if recv == nil {
		return f.Pkg.Func(name)
	}
	return m.prog.MethodValue(m.prog.MethodSets.MethodSet(recv.Type()).Lookup(f.Pkg.Pkg, name))
}

// textProxy is the variable that TextVar gives flag for a variable of the
// program: its methods call the methods of the variable in its cell.
type textProxy struct{ o vmObj }

func (p *textProxy) MarshalText() ([]byte, error) {
	out := p.o.call("MarshalText")
	return rwOut[[]byte](out, 0), rwOut[error](out, 1)
}

func (p *textProxy) UnmarshalText(b []byte) error {
	return rwOut[error](p.o.call("UnmarshalText", reflect.ValueOf(b)), 0)
}

// textVarHook implements TextVar(p, name, value, usage) of flag, or of a
// *flag.FlagSet when method is set. flag sets *p to the default value with
// native reflect: the hook does it in the cell (with the checks and panics
// of flag), and gives flag the proxy as the variable and as the default.
func textVarHook(method bool) storageHook {
	return func(m *Machine, f *ssa.Function, a []Value) Value {
		i := 0
		if method {
			i = 1
		}
		iv, _ := a[i].R.(*Iface)
		if iv == nil || iv.T.T == nil {
			return m.callFlag(f, a) // nil, or a native value
		}
		pt, ok := iv.T.T.Underlying().(*types.Pointer)
		if !ok {
			panic("variable value type must be a pointer")
		}
		cell, ok := iv.V.R.(*Value)
		if !ok {
			return m.callFlag(f, a) // a pointer to native memory
		}
		*cell = deepCopy(m.textDefault(a[i+2], pt.Elem()), pt.Elem())
		px := Value{R: reflect.ValueOf(&textProxy{vmObj{m: m, iv: iv, origin: m.originAcct()}})}
		args := append([]Value(nil), a...)
		args[i], args[i+2] = px, px
		return m.callFlag(f, args)
	}
}

// textDefault returns the default value v of TextVar (a pointer to it, or
// the value itself) as a value of the type et of the variable, or panics
// as flag does.
func (m *Machine) textDefault(v Value, et types.Type) Value {
	dv, _ := v.R.(*Iface)
	if dv == nil {
		panic(fmt.Sprintf("default type does not match variable type: %v != %s", nil, typeString(et)))
	}
	if dv.T.T == nil {
		rv, _ := dv.V.R.(reflect.Value)
		if rv.Kind() == reflect.Pointer {
			rv = rv.Elem()
		}
		if nt := m.reflectType(et); rv.Type() != nt {
			panic(fmt.Sprintf("default type does not match variable type: %v != %v", rv.Type(), nt))
		}
		return m.fromNative(rv, et)
	}
	t, x := dv.T.T, dv.V
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
		if c, ok := x.R.(*Value); ok {
			x = *c
		} else if rv, ok := x.R.(reflect.Value); ok {
			x = m.fromNative(rv.Elem(), t)
		}
	}
	if !types.Identical(t, et) {
		panic(fmt.Sprintf("default type does not match variable type: %s != %s", typeString(t), typeString(et)))
	}
	return x
}

var (
	_ encoding.TextMarshaler   = (*textProxy)(nil)
	_ encoding.TextUnmarshaler = (*textProxy)(nil)
)

// callFlag calls f, a function of package flag, as the program would
// without the hooks of flagvars.go: the AOT copy, the native function, or
// the interpreted one.
func (m *Machine) callFlag(f *ssa.Function, a []Value) Value {
	if m.isInterp(f) {
		return m.Call(m.funcOf(f), a, nil)
	}
	if m.modeOf(f.Pkg.Pkg.Path()) == ModeAOT {
		regMu.RLock()
		h := nativeHooks[hookName(f)]
		regMu.RUnlock()
		if h != nil {
			r := h(m, a)
			m.flushShadows()
			return r
		}
	}
	idx := make([]int32, len(a))
	for i := range idx {
		idx[i] = int32(i)
	}
	return m.callNativeSite(m.nativeSite(f, idx), a)
}

// flagVarHooks registers the hooks of flag.<kind>Var and of the method of
// *flag.FlagSet of that name.
func flagVarHooks[T flagKind](kind string) {
	flagHooks(kind+"Var", flagVarHook[T])
}

// flagHooks registers hook(false) for the function name of package flag,
// and hook(true) for the method of *flag.FlagSet of that name.
func flagHooks(name string, hook func(method bool) storageHook) {
	fn := hook(false)
	vmHooks["flag."+name] = func(m *Machine, a []Value) Value {
		return fn(m, m.prog.ImportedPackage("flag").Func(name), a)
	}
	vmHooksAOT["flag."+name] = true
	storageHooks["(*flag.FlagSet)."+name] = hook(true)
}

func init() {
	flagHooks("StringVar", stringVarHook)
	flagHooks("TextVar", textVarHook)
	flagVarHooks[bool]("Bool")
	flagVarHooks[int]("Int")
	flagVarHooks[int64]("Int64")
	flagVarHooks[uint]("Uint")
	flagVarHooks[uint64]("Uint64")
	flagVarHooks[float64]("Float64")
	flagVarHooks[time.Duration]("Duration")
}
