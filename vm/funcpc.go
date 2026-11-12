package vm

// Names of interpreted functions for runtime.FuncForPC.
//
// The idiom runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name() gives
// the name of a function (loggers, routers, test helpers). An interpreted
// function has no machine code: reflect's Pointer returns the address of
// its *Func (one per function, like a code pointer), which the machine
// remembers. FuncForPC of such an address returns a stand-in *runtime.Func.
//
// The stand-in has the layout of the runtime's funcinl: the record that
// runtime.FuncForPC itself returns for an inlined function. So the real
// methods of *runtime.Func (Name, Entry, FileLine) read it correctly from
// any caller: a static call of the program, a method value, an interface
// call, or native code that gets the *runtime.Func. A check at start-up
// makes sure that the runtime still reads this layout; if not, FuncForPC
// gives nil for interpreted functions, like for an address that is not code.

import (
	"go/types"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// funcPC returns the address used as the code pointer of fn, and remembers
// it for FuncForPC.
func (m *Machine) funcPC(fn *Func) unsafe.Pointer {
	p := unsafe.Pointer(fn)
	if _, ok := m.funcPCs.Load(uintptr(p)); !ok {
		m.funcPCs.Store(uintptr(p), fn) // same value for a key: no race
	}
	return p
}

// funcInl has the layout of runtime.funcinl (runtime/runtime2.go).
type funcInl struct {
	ones      uint32 // ^0: tells the runtime that this is not a _func
	entry     uintptr
	name      string
	file      string
	line      int32
	startLine int32
}

// funcInlOK reports whether the runtime reads funcInl as a funcinl.
var funcInlOK = probeFuncInl(readRuntimeFunc)

// readRuntimeFunc reads f with the methods of *runtime.Func.
func readRuntimeFunc(f *runtime.Func) (name string, entry uintptr, file string, line int) {
	file, line = f.FileLine(0)
	return f.Name(), f.Entry(), file, line
}

// probeFuncInl reports whether read gives back the fields of a funcInl. It
// runs when package vm starts, in every host: a runtime that reads another
// layout may panic in read (an index out of range in its tables) or read a
// bad address, and this must not end the host: the panic, and the fault
// (SetPanicOnFault), only make the probe fail.
func probeFuncInl(read func(*runtime.Func) (string, uintptr, string, int)) (ok bool) {
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	probe := &funcInl{ones: ^uint32(0), entry: 42, name: "vm.probe", file: "probe.go", line: 7, startLine: 7}
	name, entry, file, line := read((*runtime.Func)(unsafe.Pointer(probe)))
	return name == "vm.probe" && entry == 42 && file == "probe.go" && line == 7
}

// runtimeFuncOf returns the stand-in *runtime.Func of fn (nil when the
// runtime does not read the funcinl layout, see funcInlOK).
func (m *Machine) runtimeFuncOf(fn *Func) *runtime.Func {
	if !funcInlOK {
		return nil
	}
	if v, ok := m.funcStandIns.Load(fn); ok {
		return (*runtime.Func)(unsafe.Pointer(v.(*funcInl)))
	}
	pos := fn.ssa.Prog.Fset.Position(fn.ssa.Pos())
	s := &funcInl{
		ones:      ^uint32(0),
		entry:     uintptr(unsafe.Pointer(fn)),
		name:      funcName(fn.ssa),
		file:      pos.Filename,
		line:      int32(pos.Line),
		startLine: int32(pos.Line),
	}
	v, _ := m.funcStandIns.LoadOrStore(fn, s) // keeps the stand-in alive
	return (*runtime.Func)(unsafe.Pointer(v.(*funcInl)))
}

func init() {
	vmHooks["runtime.FuncForPC"] = func(m *Machine, a []Value) Value {
		pc := uintptr(a[0].N)
		if fn, ok := m.funcPCs.Load(pc); ok {
			return Value{R: reflect.ValueOf(m.runtimeFuncOf(fn.(*Func)))}
		}
		if fn, k, ok := m.decodePC(pc); ok { // a PC of the stack (callers.go)
			return Value{R: reflect.ValueOf(m.lineFunc(fn, k))}
		}
		return Value{R: reflect.ValueOf(runtime.FuncForPC(pc))}
	}
}

// funcName returns the name gc gives to f: "main.f", "main.(*T).M",
// "main.main.func1", "main.T.M-fm" (method value), "main.G[...].M"
// (method of a generic type), "main.f[...]" (generic function).
func funcName(f *ssa.Function) string {
	if f.Parent() != nil {
		// Anonymous function: SSA names it parent$N.
		name := funcName(f.Parent())
		n := f.Name()[strings.LastIndexByte(f.Name(), '$')+1:]
		if f.Parent().Parent() == nil {
			return name + ".func" + n
		}
		return name + "." + n
	}
	if o := f.Origin(); o != nil && o.Signature.Recv() == nil && f.Signature.Recv() == nil {
		return funcName(o) + "[...]" // instance of a generic function
	}
	pkg := ""
	if f.Pkg != nil {
		pkg = reflectPkgPath(f.Pkg.Pkg) + "."
	}
	obj, _ := f.Object().(*types.Func)
	var recv types.Type // receiver type as gc names it
	name, suffix := initName(f.Name()), ""
	switch {
	case obj != nil && f.Synthetic != "":
		// Wrapper of a method: the method's name, and the receiver of the
		// wrapper (U.M for a method M promoted from an embedded T,
		// (*T).M for a method M of T).
		name = obj.Name()
		if obj.Pkg() != nil {
			pkg = reflectPkgPath(obj.Pkg()) + "."
		}
		switch {
		case strings.HasPrefix(f.Synthetic, "bound method wrapper"):
			recv, suffix = obj.Type().(*types.Signature).Recv().Type(), "-fm" // method value x.M
		case strings.HasPrefix(f.Synthetic, "thunk"):
			recv = f.Signature.Params().At(0).Type() // method expression T.M
		case f.Signature.Recv() != nil:
			recv = f.Signature.Recv().Type()
		}
	case f.Signature.Recv() != nil:
		recv = f.Signature.Recv().Type()
		if obj != nil {
			name = obj.Name()
		}
	}
	if recv == nil {
		return pkg + name
	}
	ptr := false
	if p, ok := types.Unalias(recv).(*types.Pointer); ok {
		recv, ptr = p.Elem(), true
	}
	n, ok := types.Unalias(recv).(*types.Named)
	if !ok {
		// method of an unnamed interface type
		return "go:" + typeString(recv) + "." + name + suffix
	}
	if n.Obj().Pkg() != nil {
		pkg = reflectPkgPath(n.Obj().Pkg()) + "."
	}
	tn := n.Obj().Name()
	if n.TypeArgs().Len() > 0 || n.TypeParams().Len() > 0 {
		tn += "[...]"
	}
	if ptr {
		tn = "(*" + tn + ")"
	}
	return pkg + tn + "." + name + suffix
}

// initName returns the gc name of a function named name by ssa: ssa
// numbers the init functions of a package init#1, init#2..., gc names them
// init.0, init.1...
func initName(name string) string {
	if n, ok := strings.CutPrefix(name, "init#"); ok {
		if i, err := strconv.Atoi(n); err == nil {
			return "init." + strconv.Itoa(i-1)
		}
	}
	return name
}
