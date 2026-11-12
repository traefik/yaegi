package vm

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"io"
	"math"
	"reflect"
	"sync"
	"sync/atomic"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// Machine holds the compiled program. It is immutable after compilation of a
// function, so several goroutines (native callbacks) may execute concurrently.
type Machine struct {
	prog    *ssa.Program
	mu      sync.Mutex
	funcs   map[*ssa.Function]*Func
	rtypes  typeutil.Map // types.Type -> *RType
	natives sync.Map     // reflect.Type -> *RType of a native dynamic type (nativeRType)
	globals map[*ssa.Global]Value
	interp  map[*types.Package]bool // packages interpreted from source
	// evalInterp holds the packages that a Session adds after the machine
	// started (eval.go): *types.Package -> true. Code of earlier inputs may
	// still run on other goroutines, so interp, a plain map, cannot grow.
	// nil outside a Session.
	evalInterp *sync.Map

	recFields   sync.Map                  // *types.Struct -> []bool, fields referring back to the struct (reflectType)
	recNamed    sync.Map                  // *types.Named -> bool, a named type that refers to itself (selfRecursive)
	hasCuts     sync.Map                  // types.Type -> bool, native type with a cut recursive field (recursive.go)
	uniform     sync.Map                  // types.Type -> *uniformTypes, uniform unrolled types (recursive.go)
	learned     sync.Map                  // "pkgpath.Name" -> reflect.Type of native types missing from the registry
	wrapCache   sync.Map                  // wrapKey -> *yaegiWrapper (generic interface wrappers)
	rwCache     sync.Map                  // *RType -> rwFeat, optional ResponseWriter methods (rwwriter.go)
	optCache    sync.Map                  // wrapKey -> optCombo, wrappers with optional methods (optwrap.go)
	optVals     sync.Map                  // ifKey -> weak.Pointer[Iface], one payload per value of the wrappers (optwrap.go)
	ptrShadows  sync.Map                  // uintptr -> ptrShadow (native shadows of interpreted pointers)
	valShadows  sync.Map                  // uintptr -> valShadow (native copies of interpreted values in interfaces)
	zeroHandles sync.Map                  // *RType -> reflect.Value (zeroSizeHandle)
	ptrInfos    sync.Map                  // *types.Pointer -> ptrInfo (Machine.ptrInfo)
	plainTypes  sync.Map                  // types.Type -> bool, plain data results (plainType)
	zeroCounts  sync.Map                  // types.Type -> int64 (zeroEntries of a struct or array, see checkZero)
	uregions    uregions                  // memory reachable from pointers converted to uintptr (unsafe.go)
	typeToks    sync.Map                  // *RType or native reflect.Type -> *Value: type words of interfaces (ifaceview.go)
	hints       ptrHints                  // types of the struct cells reached through unsafe, for when the regions forget them (ifaceview.go)
	extents     dataExtents               // memory behind the pointers given by unsafe.SliceData and StringData (unsafeext.go)
	embeds      map[*types.Var]embedValue // content of the //go:embed variables (embed.go)
	// bytes of the new data-shaped types built by this Machine, per use
	// (recursive.go; guarded by shaped.mu)
	shapedBytes [numShapedUses]int
	// native stand-ins of interpreted channels (chanstandin.go). They are
	// separate objects: the cleanups of the stand-ins use them and must
	// not keep the Machine alive.
	chanStandIns *sync.Map // chanStandInKey -> chanStandInRef
	standInChans *sync.Map // uintptr -> *chanStandIn
	closureFuncs *sync.Map // weak.Pointer[byte] of a native func -> closureFunc (native.go)
	// localSites numbers the memory sites of the frame-local Allocs: the
	// index of their free list in a ctx (escape.go).
	localSites atomic.Int32

	// Budget is the number of calls plus backward jumps allowed to each
	// call from the host, shared with its callbacks and with the goroutines
	// it starts; a background goroutine gets Budget steps between two
	// blocking points (budget.go; <=0: unlimited).
	Budget int64
	// MaxGoroutines bounds the live goroutines that the program started
	// (go statements and iter.Pull sequences); past it, the go statement
	// panics with ErrTooManyGoroutines (budget.go; <=0: no cap).
	MaxGoroutines int
	// BackgroundRate bounds the steps per second of all the background
	// goroutines of the machine together; past it, they wait (budget.go,
	// Options.BackgroundRate; <=0: no bound).
	BackgroundRate int64
	bgRate         atomic.Pointer[rateBucket]
	// mem counts the large allocations of the VM for Options.MaxMemory
	// (memquota.go; nil: no quota).
	mem *memQuota
	// Cancel stops a call from the host whose context is done: ServeHTTP
	// (the context of the request), Plugin.Call and Object.Call (Options.Cancel).
	Cancel bool
	// panicOnFatal: some fatal errors of the runtime are panics of the
	// plugin (Options.PanicOnFatal, fatal.go).
	panicOnFatal bool
	// recoverHandler: topHandler recovers the panics of ServeHTTP
	// (Options.RecoverHandler).
	recoverHandler bool
	// MaxDepth is the maximum number of interpreted frames of one goroutine,
	// and MaxStack the maximum size in bytes of its register stack (<=0: no
	// size limit). Past them, the VM panics with ErrStackOverflow.
	MaxDepth int
	MaxStack int64

	// overrides replace native functions of the registry for this machine
	// ("pkgpath.Name"): the environment copy of SetEnv.
	overrides map[string]reflect.Value
	// hookOverrides replace the RegisterNative hooks of packages run in
	// ModeAOT for this machine (hookFor): fmt.Print* of SetStdio, as
	// overrides does for native packages.
	hookOverrides map[string]NativeHook
	// varHooks give the machine its own native variables ("pkgpath.Name"):
	// os.Stdin, os.Stdout and os.Stderr of SetStdio. A hook returns the address of
	// the variable; globalPtr calls it once.
	varHooks map[string]func() reflect.Value
	// stderr is the standard error of SetStdio (nil: os.Stderr).
	stderr io.Writer
	// realStd gives the program the standard library as in Go, without
	// the runtime restrictions of plugins (stdTable): os.Exit and log.Fatal
	// end the process, and os/exec and syscall have their symbols. Set by
	// RunMain without Restricted and RestrictedStd, and by Load with
	// Unrestricted and RealProcess.
	realStd bool
	// restricted applies the restrictions of plugins at run time too: the
	// refused methods panic when the plugin calls them without naming them
	// (refusedmeth.go). Set by Load unless Unrestricted (always with a
	// Sandbox), and by RunMain with Restricted.
	restricted bool
	// unsafeSyscall gives a restricted machine the read-only subset of
	// syscall (unsafesyscall.go). Set by Load and RunMain for a restricted
	// plugin with UseUnsafe.
	unsafeSyscall bool
	// linknames are the //go:linkname targets of the functions of the
	// packages run from source; stubAsm is Options.StubAssembly
	// (linkname.go).
	linknames map[*types.Func]string
	stubAsm   bool
	// execMode is Options.Exec (osexec.go), execSyms its os/exec table,
	// execEnviron the (*exec.Cmd).Environ of ExecMock.
	execMode    ExecMode
	execOnce    sync.Once
	execSyms    map[string]reflect.Value
	execEnviron reflect.Value
	syscallOnce sync.Once
	syscallSyms map[string]reflect.Value
	// xsysSyms: the golang.org/x/sys/unix subset of a restricted machine
	// with UseUnsafe (xsysunix.go), made at the first use.
	xsysOnce sync.Once
	xsysSyms *Package
	// lentFDs are the file descriptors that a RawConn lends to a goroutine
	// of the plugin during a callback, for syscall.Read, SetsockoptInt and
	// GetsockoptInt (rawconn.go).
	fdMu    sync.Mutex
	lentFDs map[lentFD]int
	// env is the environment copy of SetEnv (nil: the environment of the
	// process).
	env *vmEnv
	// flagSt: os.Args, flag.CommandLine and flag.Usage of a restricted
	// machine (exit.go), made at the first use.
	flagOnce sync.Once
	flagSt   *flagState
	// regs: the process-wide registries of the standard library of a
	// restricted machine (hostregs.go), made at the first use.
	regsOnce sync.Once
	regs     *hostRegs
	// PrintOut is where the print and println builtins write. nil means
	// the standard error (SetStdio, else os.Stderr), like the gc runtime.
	// yaegi writes them to its Stdout instead (see RunOptions.PrintOut).
	PrintOut io.Writer
	// GoPanic, when set, receives the panics of the goroutines the program
	// starts instead of ending the process (as Go does). It also receives
	// the panics of the callbacks that native code runs on goroutines of
	// its own (time.AfterFunc, context.AfterFunc, finalizers; see
	// nativego.go).
	GoPanic func(r any)
	// MaxNest is the maximum number of nested runs of the interpreter on
	// one goroutine: calls from native code back into the VM, and deferred
	// calls (see nest.go). Each one uses real Go stack. Past it, the VM
	// panics with ErrStackOverflow.
	MaxNest int

	ctxTypeOnce sync.Once
	ctxType     types.Type // context.Context (contextType)

	esc escState // escape summaries of the interpreted functions (escape.go)

	// gob.Register of interpreted types (gob.go): the mirror type registered
	// natively -> its interpreted type, and the checks of gob on names and
	// types (under mu).
	gobTypes sync.Map // reflect.Type -> *RType
	gobNames map[string]*RType
	gobBases map[*RType]string

	// OpaqueAny: interpreted values passed to native interfaces without a
	// known wrapper become opaque handles (see box).
	OpaqueAny bool
	AOT       bool            // run registered AOT packages in ModeAOT (RegisterAOT)
	Modes     map[string]Mode // explicit mode per import path (see ModeFor)

	// AOTFallback lists the packages that have an AOT copy but are
	// interpreted, because the plugin has another version or a fork of
	// them, with the reason (see aotsource.go).
	AOTFallback map[string]string
	// NativeFallback lists the packages that the host could run natively
	// but that are interpreted, because the plugin's version does not fit
	// the host's copy, with the reason (golang.org/x/sys/unix, see xsys.go).
	NativeFallback map[string]string

	vr      *vmReflect // VM-internal reflect (nil when reflect is native only)
	goTypes map[reflect.Type]types.Type
	uniq    typeutil.Map // types.Type -> *uniqueSet of unique.Make (under mu)

	genMethods sync.Map // *ssa.Function -> genericIntrinsic of a generic method (callGenericMethod)

	// FieldTrack: the program is built with GOEXPERIMENT=fieldtrack, so
	// the methods marked //go:nointerface are not in the run-time method
	// sets (nointerface.go). Set it before the first call.
	FieldTrack bool
	noIface    noIfaceFiles

	// ownReflect: the program's own packages import reflect. Native reflect
	// cannot see interpreted methods or make named types, so the VM reflect
	// is used even without OpaqueAny (set by RunMain and the plugin loaders,
	// see ownReflect).
	ownReflect bool
	// routed: packages whose calls with interpreted values go to their AOT
	// copy (aotroute.go).
	routed map[string]bool
	// progVR: the program's own reflect calls and values use the VM
	// reflect (OpaqueAny, ownReflect). m.vr may also exist only for the
	// AOT routes: the program then keeps native reflect.
	progVR    bool
	pkgTypes  sync.Map // pkgTypeKey -> bool (aotroute.go)
	lostTypes sync.Map // lostRTKey -> error or nil (lostmethods.go)

	funcPCs      sync.Map   // uintptr -> *Func: code pointers given by reflect (funcpc.go)
	funcStandIns sync.Map   // *Func -> *funcInl: stand-in *runtime.Func (funcpc.go)
	locals       localTypes // names of local types (localtypes.go)

	// track: exec records the instruction of each step (ctx.at), for the
	// position of a panic in the stack walks (callers.go). Set when the
	// program prints the traceback of its panics (RunMain), or when an
	// interpreted package imports runtime or runtime/debug.
	track        bool
	pcMu         sync.Mutex
	pcFuncs      []*Func  // functions by Func.pcID-1 (callers.go)
	lineStandIns sync.Map // lineKey -> *funcInl (callers.go)
	// sourceAnswers: sourceKey -> bool, the answers of sourceHas (callers.go).
	sourceAnswers sync.Map

	// The orphan shadows of AOT code (twin_value.go, shadowRef): nOrphans
	// of them. The other shadows are in the gstate of their goroutine.
	shadowMu sync.Mutex
	shadows  []*shadowRef
	nOrphans atomic.Int32

	// flagInts: the int variables of the program that flag writes through
	// a pointer to the low half of their N word, on a 32-bit platform only
	// (flagvars.go).
	flagInts int32Cells

	// live counts the goroutines started by the program (go statements),
	// for GoexitError.Wait.
	liveMu   sync.Mutex
	liveCond *sync.Cond
	live     int
	nGo      atomic.Int64 // live goroutines for MaxGoroutines (budget.go)

	// closed is set by Close, and done is the channel that Close closes,
	// made at the first wait; draining is set by CloseContext (and Close),
	// and inflight counts the calls from the host that run (close.go).
	closeMu  sync.Mutex
	closed   atomic.Bool
	draining atomic.Bool
	inflight atomic.Int64
	done     atomic.Pointer[chan struct{}]
}

// NewMachine returns a machine for prog that interprets the packages of
// interp and calls the others natively. It always gives the program the
// restricted standard library of plugins (yaegi's restricted stdlib):
// os.Exit, log.Fatal and the Fatal methods of the loggers panic instead of
// ending the process, and os/exec and syscall have no symbols. There is no
// way to change this on a machine from NewMachine: the standard library as
// in Go comes only with RunMain (without RunOptions.Restricted and
// RunOptions.RestrictedStd) and with Load (Options.Unrestricted and
// Options.RealProcess).
func NewMachine(prog *ssa.Program, interp map[*types.Package]bool) *Machine {
	setStackGuard() // the host may have changed the stack limit since the start-up
	m := &Machine{
		prog:     prog,
		funcs:    map[*ssa.Function]*Func{},
		globals:  map[*ssa.Global]Value{},
		interp:   interp,
		MaxDepth: 100000,
		track:    readsStack(interp),
		MaxNest:  DefaultMaxNest,

		chanStandIns: new(sync.Map),
		standInChans: new(sync.Map),
		closureFuncs: new(sync.Map),
	}
	// time.Sleep is a blocking point of the budget, and ends early when
	// the call from the host is canceled (budget.go).
	m.addOverride("time.Sleep", m.sleep)
	return m
}

type CompileError struct{ Msg string }

func (e *CompileError) Error() string { return e.Msg }

func unsupported(format string, args ...any) {
	panic(&CompileError{Msg: "unsupported: " + fmt.Sprintf(format, args...)})
}

func (m *Machine) funcOf(f *ssa.Function) *Func {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn, ok := m.funcs[f]; ok {
		return fn
	}
	fn := &Func{m: m, ssa: f, wrapper: isMethodWrapper(f)}
	m.funcs[f] = fn
	return fn
}

// rtypeOf returns the canonical runtime descriptor of an interpreted type.
//
// Other goroutines read the method table of a published RType with no lock
// (OpInvoke, the interface wrappers, fmt through native code), and functions
// compile at their first call, so concurrent first requests build RTypes at
// the same time. The RType is therefore built completely, method table
// included, before it is published in m.rtypes. Building it does not need
// the RType itself (methodSet, MethodValue and funcOf only create the Func
// shells, they compile nothing), so no goroutine sees a partial RType. When
// two goroutines build the same type, the first one to publish wins and the
// other one drops its copy.
func (m *Machine) rtypeOf(t types.Type) *RType {
	m.mu.Lock()
	rt, ok := m.rtypes.At(t).(*RType)
	m.mu.Unlock()
	if ok {
		return rt
	}
	methods := map[string]*Func{}
	// For native named types (and pointers to them), the methods run
	// natively (bodyless functions, see callBodyless); promoted ones through
	// synthetic wrappers. For the others, this is the run-time method set:
	// native code that looks for a method by name (fmt and its Stringer, the
	// yaegi wrappers) does not see the //go:nointerface ones, as in gc.
	for _, sel := range m.methodSet(t) {
		if f := m.prog.MethodValue(sel); f != nil {
			methods[sel.Obj().Name()] = m.funcOf(f)
		}
	}
	nat := m.natKeyOf(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, ok := m.rtypes.At(t).(*RType); ok {
		return rt // published by another goroutine meanwhile
	}
	rt = &RType{T: t, noCmp: !types.Comparable(t), Methods: methods, nat: nat}
	m.rtypes.Set(t, rt)
	return rt
}

// natKeyOf returns the map key of the values of t when t is a comparable
// named native type, which values of native code also have (nil otherwise):
// m[tok] with a tok of json.Decoder.Token finds the key json.Delim('[') of
// the program.
func (m *Machine) natKeyOf(t types.Type) *natKey {
	if _, ok := types.Unalias(t).(*types.Named); !ok || isIface(t) || !types.Comparable(t) || !m.isNativeNamed(t) {
		return nil
	}
	nt := m.counterpart(t)
	if nt == nil || !nt.Comparable() {
		return nil
	}
	return &natKey{rt: m.nativeRType(nt), m: m, t: t}
}

// nativeRType returns the RType of the native type t. It takes no lock:
// every native value that enters the VM goes through it, on every
// goroutine of the plugin.
func (m *Machine) nativeRType(t reflect.Type) *RType {
	if rt, ok := m.natives.Load(t); ok {
		return rt.(*RType)
	}
	rt, _ := m.natives.LoadOrStore(t, &RType{Native: t})
	return rt.(*RType)
}

// interpPkg reports whether the VM interprets package p: a package of
// interp, or one that a Session added (evalInterp).
func (m *Machine) interpPkg(p *types.Package) bool {
	if m.interp[p] {
		return true
	}
	if m.evalInterp == nil {
		return false
	}
	_, ok := m.evalInterp.Load(p)
	return ok
}

func (m *Machine) globalPtr(g *ssa.Global) Value {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.globals[g]; ok {
		return v
	}
	var v Value
	if m.interpPkg(g.Pkg.Pkg) {
		cell := new(Value)
		t := g.Type().(*types.Pointer).Elem()
		n := m.memGlobal(g, t)
		*cell = m.zero(t)
		if n > 0 {
			m.memSliceOf(cell.R, n/int64(max(1, lenValues(cell.R))))
		}
		if gv, _ := g.Object().(*types.Var); gv == nil {
		} else if data, ok := m.embeds[gv]; ok {
			// //go:embed: the content of the file, before the
			// initializers run (embed.go)
			switch {
			case data.isFS:
				fsys := newEmbedFS(data.files)
				*cell = Value{R: reflect.ValueOf(&fsys).Elem()}
			case isString(g.Type().(*types.Pointer).Elem()):
				*cell = Value{R: data.data}
			default:
				*cell = Value{R: []byte(data.data)}
			}
		}
		v = Value{R: cell}
	} else {
		var rv reflect.Value
		ok := false
		if hook, isHook := m.varHooks[g.Pkg.Pkg.Path()+"."+g.Name()]; isHook {
			rv, ok = hook(), true
		} else {
			rv, ok = m.restrictedVar(g.Pkg.Pkg.Path(), g.Name())
		}
		if t := m.table(g.Pkg.Pkg.Path()); t != nil && !ok {
			rv, ok = t.Vars[g.Name()]
		}
		if !ok {
			rv, ok = nativeVars[g.Pkg.Pkg.Path()+"."+g.Name()]
		}
		if !ok {
			// yaegi exports variables as reflect.ValueOf(&v).Elem(): the SSA
			// global is the address of the variable.
			if rv, ok = m.stdSym(g.Pkg.Pkg, g.Name()); ok && rv.CanAddr() {
				rv = rv.Addr()
			}
		}
		if !ok {
			unsupported("native global %s.%s not in registry%s", g.Pkg.Pkg.Path(), g.Name(), m.whyMissing(g.Pkg.Pkg))
		}
		if n, isNamed := types.Unalias(g.Type().(*types.Pointer).Elem()).(*types.Named); isNamed && n.Obj().Pkg() != nil && rv.Kind() == reflect.Pointer {
			// Unexported native types (e.g. encoding/binary.littleEndian) are
			// not in the symbol tables: learn them from the variables.
			key := n.Obj().Pkg().Path() + "." + n.Obj().Name()
			if _, known := m.learned.Load(key); !known {
				m.learned.Store(key, rv.Type().Elem())
			}
		}
		v = Value{R: rv}
	}
	m.globals[g] = v
	return v
}

// isInterp reports whether f runs in the VM (from source) or natively.
func (m *Machine) isInterp(f *ssa.Function) bool {
	if len(f.Blocks) == 0 {
		return false
	}
	if f.Pkg == nil {
		return true // synthetic wrappers, thunks, bound methods
	}
	if m.interpPkg(f.Pkg.Pkg) {
		return true
	}
	if f.Origin() != nil {
		return true // generic instance: no native instantiation exists
	}
	// An unexported function of a standard library package built from its
	// embedded source (slices, maps, iter...): generic code of that package
	// calls it (slices.nextPowerOfTwo and xorshift.Next from pdqsort), and
	// the native symbol tables have no unexported names.
	return unexportedFunc(f) && embeddedStdSource(f.Pkg.Pkg.Path()) && m.modeOf(f.Pkg.Pkg.Path()) == ModeNative
}

// embeddedStdSource reports whether path is a standard library package whose
// source is embedded in stdtypes (built from source by the default loader).
func embeddedStdSource(path string) bool {
	u, err := stdtypes.Load()
	return err == nil && u.Source(path) != nil
}

// unexportedFunc reports whether f is an unexported function or method, or a
// method of an unexported type: a name that no native symbol table has.
func unexportedFunc(f *ssa.Function) bool {
	if f.Parent() != nil || f.Synthetic != "" {
		return false
	}
	if !token.IsExported(f.Name()) {
		return true
	}
	recv := f.Signature.Recv()
	if recv == nil {
		return false
	}
	t := recv.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && !n.Obj().Exported()
}

func (m *Machine) isNativeNamed(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return false
	}
	if m.progVR && isReflectPkg(n.Obj().Pkg()) {
		return false // VM-internal reflect: VM layout
	}
	if n.TypeArgs().Len() > 0 {
		return false // instance of a generic type: no native instance exists
	}
	return !m.interpPkg(n.Obj().Pkg())
}

// nativeMapType returns the native type of a map of a native named type, or
// nil.
func (m *Machine) nativeMapType(t types.Type) reflect.Type {
	if _, ok := t.Underlying().(*types.Map); !ok || !m.isNativeNamed(t) {
		return nil
	}
	return m.tryReflectType(t)
}

// zero returns the zero value of t. It panics with a *LoadError when the
// value is too large for the VM (checkZero, loaddos.go).
func (m *Machine) zero(t types.Type) Value {
	m.checkZero(t)
	return m.zeroValue(t)
}

// zeroValue is zero without the size check (done once by zero for the whole
// value).
func (m *Machine) zeroValue(t types.Type) Value {
	if m.isNativeNamed(t) {
		if _, ok := t.Underlying().(*types.Struct); ok {
			return Value{R: reflect.New(m.reflectType(t)).Elem()}
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Info()&types.IsString != 0 {
			return Value{R: ""}
		}
	case *types.Struct:
		fs := make([]Value, u.NumFields())
		for i := range fs {
			fs[i] = m.zeroValue(u.Field(i).Type())
		}
		return Value{R: fs}
	case *types.Array:
		if u.Len() >= zsliceMin && zeroSized(u.Elem()) {
			// make([]T, N) with a constant N is an array allocation in SSA
			return Value{R: &zslice{s: make([]struct{}, u.Len()), zero: m.zeroValue(u.Elem()), t: u.Elem()}}
		}
		es := make([]Value, u.Len())
		for i := range es {
			es[i] = m.zeroValue(u.Elem())
		}
		return Value{R: es}
	}
	return Value{}
}

func deepCopy(v Value, t types.Type) Value {
	if s, ok := v.R.(*shadowRef); ok {
		// a cell shadowed by AOT code (see shadowRef): copy its current value
		return s.m.fromNativeDeep(s.rv, s.t)
	}
	if rv, ok := v.R.(reflect.Value); ok && rv.Kind() == reflect.Struct {
		c := reflect.New(rv.Type()).Elem()
		c.Set(rv)
		return Value{R: c}
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		src, _ := v.R.([]Value)
		dst := make([]Value, len(src))
		// a field that AOT code shadows holds its value in the shadow
		// (see ownShadow)
		shadows := ownedShadows.Load() > 0
		for i := range src {
			if shadows {
				if sh := ownShadow(&src[i]); sh != nil {
					dst[i] = sh.m.fromNativeDeep(sh.rv, sh.t)
					continue
				}
			}
			dst[i] = deepCopy(loadCell(&src[i], u.Field(i).Type()), u.Field(i).Type())
		}
		return Value{R: dst}
	case *types.Array:
		if _, ok := v.R.(*zslice); ok {
			return v // no state to copy
		}
		src, _ := v.R.([]Value)
		dst := make([]Value, len(src))
		if flatElem(u.Elem()) {
			// Elements are plain values: one copy instead of a call and a
			// type switch per element (a local [1024]byte, GOROOT test
			// fixedbugs/issue13169.go).
			if wordKind(u.Elem()) {
				for i := range src {
					dst[i] = loadWord(&src[i])
				}
			} else {
				copy(dst, src)
			}
			if ownedShadows.Load() > 0 {
				shadowElems(dst, src)
			}
			return Value{R: dst}
		}
		shadows := ownedShadows.Load() > 0
		for i := range src {
			if shadows {
				if sh := ownShadow(&src[i]); sh != nil {
					dst[i] = sh.m.fromNativeDeep(sh.rv, sh.t)
					continue
				}
			}
			dst[i] = deepCopy(loadCell(&src[i], u.Elem()), u.Elem())
		}
		return Value{R: dst}
	}
	return v
}

// shadowElems puts in dst, the copy of the array src, the current value of
// the elements of src that AOT code shadows (see ownShadow).
func shadowElems(dst, src []Value) {
	for i := range src {
		if sh := ownShadow(&src[i]); sh != nil {
			dst[i] = sh.m.fromNativeDeep(sh.rv, sh.t)
		}
	}
}

// flatElem reports whether deepCopy returns values of type t unchanged:
// t is not a struct or an array, and it is not a named type (a cell of a
// native named type may hold a shadowRef or a native struct).
func flatElem(t types.Type) bool {
	switch types.Unalias(t).(type) {
	case *types.Basic, *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	}
	return false
}

func needsCopy(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	}
	return false
}

// numConst returns the register value of the numeric constant val of type t
// (not a complex type). The type decides the representation, not the
// constant kind: x << 1. has a float constant of type uint, and x << (1+0i)
// a complex one.
func numConst(val constant.Value, t types.Type) Value {
	if val.Kind() == constant.Complex {
		val = constant.Real(val)
	}
	b, _ := t.Underlying().(*types.Basic)
	if b == nil {
		// No basic type: only the constant kind can decide.
		if val.Kind() == constant.Float {
			return floatConst(val)
		}
		return intConst(val)
	}
	switch {
	case b.Kind() == types.Float32:
		f, _ := constant.Float32Val(constant.ToFloat(val))
		return Value{N: int64(math.Float64bits(float64(f)))}
	case b.Info()&types.IsFloat != 0:
		return floatConst(val)
	}
	return intConst(val)
}

// floatConst returns val as float64 bits.
func floatConst(val constant.Value) Value {
	f, _ := constant.Float64Val(constant.ToFloat(val))
	return Value{N: int64(math.Float64bits(f))}
}

// intConst returns val as an integer. A uint64 above MaxInt64 keeps its bits.
func intConst(val constant.Value) Value {
	v := constant.ToInt(val)
	if i, ok := constant.Int64Val(v); ok {
		return Value{N: i}
	}
	u, _ := constant.Uint64Val(v)
	return Value{N: int64(u)}
}

func constValue(m *Machine, c *ssa.Const) Value {
	if c.Value == nil {
		return m.zero(c.Type())
	}
	if isComplex(c.Type()) {
		return cplxVal(constComplex(c.Value), c.Type())
	}
	switch c.Value.Kind() {
	case constant.Bool:
		if constant.BoolVal(c.Value) {
			return Value{N: 1}
		}
		return Value{}
	case constant.String:
		return Value{R: constant.StringVal(c.Value)}
	case constant.Int, constant.Float, constant.Complex:
		if b := basic(c.Type()); b != nil && b.Kind() == types.UnsafePointer {
			// unsafe.Pointer(uintptr(k)) with a constant k
			if u, _ := constant.Uint64Val(c.Value); u != 0 {
				return Value{R: badAddr(u)}
			}
			return Value{}
		}
		return numConst(c.Value, c.Type())
	}
	unsupported("constant kind %v", c.Value.Kind())
	return Value{}
}

// ---- per-instruction auxiliary data ----

type callSite struct {
	fn   *Func
	args []int32
	sig  *types.Signature // dynamic calls only
}

type invokeSite struct {
	once sync.Once
	fast func(recv any, regs []Value, a []int32) (Value, bool)
	name string
	args []int32
	sig  *types.Signature
	// hit: the first interpreted dynamic type seen at this site and its
	// method, so the common monomorphic site skips the method map.
	hit atomic.Pointer[invokeHit]
	// gen: the generated direct call of the method for a native value
	// (genFastInvoke), or nil; natHit: the first native dynamic type seen
	// at this site and whether gen may call its method (natInvokeOK).
	genOnce sync.Once
	gen     func(recv any, regs []Value, a []int32) (Value, bool)
	natHit  atomic.Pointer[natInvokeHit]
	// memCheck: a machine with a quota and a method name of
	// nativeMemMethods: a native receiver gets the size check of the call
	// (nativemem.go).
	memCheck bool
}

type natInvokeHit struct {
	t  reflect.Type
	ok bool
}

type invokeHit struct {
	rt *RType
	fn *Func
}

// assertHit is the first dynamic type seen at a type assertion to an
// interface type, with the answer of implements.
type assertHit struct {
	rt *RType
	ok bool
}

type closureSite struct {
	fn    *Func
	binds []int32
}

type sliceSite struct {
	lo, hi, max    int32
	kind           uint8      // sliceOfSlice, sliceOfArray or sliceOfString
	loU, hiU, maxU bool       // the index has an unsigned type
	makeslice      bool       // make([]T, n, constant) built by ssa as new([c]T)[:n]
	esize          uint64     // with makeslice: the size of T
	t              types.Type // result type
}

const (
	sliceOfSlice uint8 = iota
	sliceOfArray       // pointer to an array
	sliceOfString
)

type assertSite struct {
	once     sync.Once
	nat      reflect.Type     // native counterpart of a concrete type (native dynamic values)
	concrete *RType           // asserted concrete type
	iface    *types.Interface // asserted interface type
	t        types.Type
	x        types.Type // static type of the operand (for the error message)
	commaOk  bool
	zero     Value
	hit      atomic.Pointer[assertHit] // interface assertions: the first type seen
}

type lookupSite struct {
	commaOk bool
	zero    Value
	str     bool
	idx     uint8 // index flags of a string index (idxUnsigned)
	ms      *mapSite
}

func (c *compiler) mapSite(t types.Type) *mapSite {
	mt := t.Underlying().(*types.Map)
	ms := &mapSite{key: mt.Key(), elem: mt.Elem(), nan: mayNaN(mt.Key())}
	if pt, ok := mt.Key().Underlying().(*types.Pointer); ok {
		ms.ptr = ptrInfoOf(pt)
	}
	return ms
}

// rangeUnused reports whether the loop over the map of r never reads the
// key or the value: ssa gives such a component the invalid type in the
// tuple of each Next. The range of a native map then skips its conversion.
func rangeUnused(r *ssa.Range) (noKey, noElem bool) {
	noKey, noElem = true, true
	for _, ref := range *r.Referrers() {
		n, ok := ref.(*ssa.Next)
		if !ok {
			return false, false
		}
		tup := n.Type().(*types.Tuple)
		noKey = noKey && tup.At(1).Type() == types.Typ[types.Invalid]
		noElem = noElem && tup.At(2).Type() == types.Typ[types.Invalid]
	}
	return noKey, noElem
}

type printSite struct {
	args  []int32
	types []types.Type
	ln    bool
}

type fixup struct {
	at    int
	field int
	block *ssa.BasicBlock
}

type compiler struct {
	fused    map[ssa.Value]bool // address computations fused into their load/store
	nextOf   map[ssa.Instruction]ssa.Instruction
	next     *ssa.BasicBlock // block compiled after the current one (fall-through target)
	m        *Machine
	f        *Func
	regs     map[ssa.Value]int32
	template []Value
	code     []Instr
	blockPC  map[*ssa.BasicBlock]int
	fixups   []fixup
	// deferStack: the function calls ssa:deferstack (see runDefersAt).
	deferStack bool
	// skip holds the instructions that only compute the operand of
	// unsafe.Sizeof, Alignof or Offsetof: Go never evaluates it.
	skip map[ssa.Instruction]bool
	// appendV: the append calls compiled without their varargs array, with
	// their elements (appendv.go). Their array instructions are in skip.
	appendV map[*ssa.Call][]ssa.Value
	// aggs: the composite literals built in a register, by their Load;
	// spills: the read-only locals aliased to the value stored in them;
	// aliases: the field and element addresses into them (aggv.go).
	aggs map[*ssa.UnOp]*aggLit
	// aggStores: the Stores of a literal of aggs straight into memory
	// (OpStoreAgg), by the Store; their Load emits nothing.
	aggStores map[*ssa.Store]*aggLit
	spills    map[*ssa.Alloc]bool
	aliases   map[ssa.Value]bool
	// aggScan counts the instructions that aggStore looked at: a test
	// checks that it grows with the number of literals, not its square.
	aggScan int
	// shallow: the loads that share the cells they load (loads.go).
	shallow map[*ssa.UnOp]bool
	// unsafeCalls maps the Lparen of these builtin calls to their syntax.
	unsafeCalls map[token.Pos]*ast.CallExpr
	// pos: source positions of the code (Func.pos).
	pos []posRun
	// switches: the OpSwitch chains, whose tables are built at the end
	// (switches.go).
	switches []*switchChain
	inChain  map[*ssa.BasicBlock]bool
}

func (f *Func) ensure() {
	f.once.Do(func() { f.m.compile(f) })
}

func (m *Machine) compile(f *Func) {
	fn := f.ssa
	if len(fn.Blocks) == 0 {
		unsupported("call of bodyless function %s", fn)
	}
	m.noteLocalTypes(fn)
	c := &compiler{m: m, f: f, regs: map[ssa.Value]int32{}, blockPC: map[*ssa.BasicBlock]int{}, fused: map[ssa.Value]bool{}}
	for _, p := range fn.Params {
		c.reg(p)
	}
	for _, fv := range fn.FreeVars {
		c.reg(fv)
	}
	f.nparams = len(fn.Params) + len(fn.FreeVars)
	if optTuple {
		// Tuple results live in consecutive registers; Extract is an alias.
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				if e, ok := ins.(*ssa.Extract); ok {
					c.regs[e] = c.reg(e.Tuple) + int32(e.Index)
				}
			}
		}
	}
	c.skipUnsafeOperands()
	c.skipAppendVarargs()
	c.skipAggregates()
	c.markLoads()
	c.coalescePhis()
	for bi, b := range fn.Blocks {
		c.blockPC[b] = len(c.code)
		c.next = nil
		if bi+1 < len(fn.Blocks) {
			c.next = fn.Blocks[bi+1]
		}
		// The entry block calls ssa:deferstack, so c.deferStack is known
		// when the other blocks are compiled.
		rd := c.runDefersAt(b)
		for i, ins := range b.Instrs {
			if i == rd {
				c.notePos(c.exitPos(b))
				c.emit(Instr{Op: OpRunDefers})
			}
			// One entry per SSA instruction, skipped ones included
			// (gclive.go walks the instructions in the same order).
			f.insStart = append(f.insStart, int32(len(c.code)))
			if c.skip[ins] {
				continue
			}
			c.notePos(c.posOf(ins))
			c.instrLazy(ins)
		}
	}
	f.insReg = make([][2]int32, len(f.insStart))
	i := 0
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			if v, ok := ins.(ssa.Value); ok && !c.fused[v] {
				if r, ok := c.regs[v]; ok {
					n := int32(1)
					if t, ok := v.Type().(*types.Tuple); ok && optTuple && t.Len() > 1 {
						n = int32(t.Len())
					}
					f.insReg[i] = [2]int32{r, n}
				}
			}
			i++
		}
	}
	if fn.Recover != nil {
		f.recoverPC = c.blockPC[fn.Recover]
	}
	for _, fx := range c.fixups {
		pc, ok := c.blockPC[fx.block]
		if !ok {
			panic("bad fixup")
		}
		switch fx.field {
		case 0:
			c.code[fx.at].A = int32(pc)
		case 1:
			c.code[fx.at].B = int32(pc)
		case 2:
			c.code[fx.at].C = int32(pc)
		}
	}
	for _, sw := range c.switches {
		sw.finish(c)
	}
	f.code = c.code
	f.pos = c.pos
	f.template = c.template
	f.nregs = len(c.template)
	f.aggVals = c.aggVals()
}

// instrLazy compiles ins; an unsupported construct becomes an instruction that
// fails when (and only if) it is executed.
func (c *compiler) instrLazy(ins ssa.Instruction) {
	n := len(c.code)
	defer func() {
		if r := recover(); r != nil {
			ce, ok := r.(*CompileError)
			if !ok {
				panic(r)
			}
			c.code = c.code[:n]
			msg := ce.Msg + " (in " + c.f.ssa.String() + ")"
			pos := c.posOf(ins)
			if !pos.IsValid() {
				pos = c.f.ssa.Pos()
			}
			c.f.problems = append(c.f.problems, problem{pos: pos, msg: msg, block: ins.Block()})
			c.emit(Instr{Op: OpFunc, X: func([]Value) Value { panic(&CompileError{Msg: msg}) }})
		}
	}()
	c.instr(ins)
	if v, ok := ins.(ssa.Value); ok && optTuple {
		if t, ok := v.Type().(*types.Tuple); ok && t.Len() > 1 {
			r := c.regs[v]
			for i := len(c.code) - 1; i >= n; i-- {
				if c.code[i].A == r {
					c.code[i].T = int16(t.Len())
					break
				}
			}
		}
	}
}

// nextInstr returns the instruction following ins in its block (skipping
// debug refs).
func (c *compiler) nextInstr(ins ssa.Instruction) ssa.Instruction {
	if c.nextOf == nil {
		c.nextOf = map[ssa.Instruction]ssa.Instruction{}
		for _, b := range c.f.ssa.Blocks {
			var prev ssa.Instruction
			for _, y := range b.Instrs {
				if _, ok := y.(*ssa.DebugRef); ok {
					continue
				}
				if prev != nil {
					c.nextOf[prev] = y
				}
				prev = y
			}
		}
	}
	return c.nextOf[ins]
}

// fusable reports whether an address computation is used only by the
// load/store that immediately follows it.
func (c *compiler) fusable(v ssa.Value) bool {
	if !optSuper {
		return false
	}
	refs := *v.Referrers()
	if len(refs) != 1 {
		return false
	}
	r := refs[0]
	if r != c.nextInstr(v.(ssa.Instruction)) {
		return false
	}
	switch r := r.(type) {
	case *ssa.UnOp:
		return r.Op == token.MUL && r.X == v
	case *ssa.Store:
		return r.Addr == v && r.Val != v
	}
	return false
}

var negJump = map[Op]Op{OpJEqI: OpJNeI, OpJNeI: OpJEqI, OpJLtI: OpJGeI, OpJGeI: OpJLtI, OpJLeI: OpJGtI, OpJGtI: OpJLeI,
	OpJLtU: OpJGeU, OpJGeU: OpJLtU, OpJLeU: OpJGtU, OpJGtU: OpJLeU, OpJEqS: OpJNeS, OpJNeS: OpJEqS}

var jumpOps = map[Op]Op{OpEqI: OpJEqI, OpNeI: OpJNeI, OpLtI: OpJLtI, OpLeI: OpJLeI, OpGtI: OpJGtI, OpGeI: OpJGeI,
	OpLtU: OpJLtU, OpLeU: OpJLeU, OpGtU: OpJGtU, OpGeU: OpJGeU, OpEqS: OpJEqS, OpNeS: OpJNeS}

// fuseCmp reports whether the If's condition is a compare emitted just
// before, used only by this If.
func (c *compiler) fuseCmp(v *ssa.If) (Op, int32, int32, bool) {
	if !optSuper || len(c.code) == 0 {
		return 0, 0, 0, false
	}
	bo, ok := v.Cond.(*ssa.BinOp)
	if !ok || len(*bo.Referrers()) != 1 || c.nextInstr(bo) != v {
		return 0, 0, 0, false
	}
	last := c.code[len(c.code)-1]
	jop, ok := jumpOps[last.Op]
	if !ok || last.A != c.regs[bo] {
		return 0, 0, 0, false
	}
	return jop, last.B, last.C, true
}

// emitGo emits a call to a Go-implemented intrinsic.
func (c *compiler) emitGo(dst int32, f func([]Value) Value, args []int32) {
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		av := make([]Value, len(args))
		for i, r := range args {
			av[i] = regs[r]
		}
		return f(av)
	}})
}

func (c *compiler) newReg() int32 {
	c.template = append(c.template, Value{})
	return int32(len(c.template) - 1)
}

func (c *compiler) reg(v ssa.Value) int32 {
	if r, ok := c.regs[v]; ok {
		return r
	}
	r := c.newReg()
	c.regs[v] = r
	if t, ok := v.Type().(*types.Tuple); ok && optTuple && t.Len() > 1 {
		for i := 1; i < t.Len(); i++ {
			c.newReg()
		}
	}
	switch v := v.(type) {
	case *ssa.Const:
		c.template[r] = constValue(c.m, v)
	case *ssa.Global:
		c.template[r] = c.m.globalPtr(v)
	case *ssa.Function:
		if !c.m.isInterp(v) {
			if vmHookName(v) == "runtime.Goexit" {
				c.template[r] = Value{R: reflect.ValueOf(goexitValue)}
			} else if mv, ok := c.m.nativeMemValue(v); ok {
				c.template[r] = mv // checked calls with a quota (nativemem.go)
			} else if h := c.m.bodylessHook(v); h != nil {
				m := c.m
				c.template[r] = Value{R: &Closure{Go: func(a []Value) Value { return h(m, a) }}}
			} else {
				c.template[r] = Value{R: c.m.nativeFunc(v)}
			}
		} else {
			c.template[r] = Value{R: &Closure{Fn: c.m.funcOf(v)}}
		}
	case *ssa.Builtin:
		unsupported("builtin %s used as value", v.Name())
	}
	return r
}

func (c *compiler) regList(vs []ssa.Value) []int32 {
	out := make([]int32, len(vs))
	for i, v := range vs {
		out[i] = c.reg(v)
	}
	return out
}

func (c *compiler) emit(in Instr) int {
	c.code = append(c.code, in)
	return len(c.code) - 1
}

func basic(t types.Type) *types.Basic {
	b, _ := t.Underlying().(*types.Basic)
	return b
}

func truncKind(t types.Type) int32 {
	b := basic(t)
	if b == nil {
		return 0
	}
	switch b.Kind() {
	case types.Int8, types.Int16, types.Int32, types.Uint8, types.Uint16, types.Uint32, types.Float32:
		// Values live in an int64 (floats as float64 bits): after an
		// operation, cut the result back to the width of its type.
		return int32(b.Kind())
	case types.Int:
		if intIs32 {
			return int32(types.Int32)
		}
	case types.Uint, types.Uintptr:
		if intIs32 {
			return int32(types.Uint32)
		}
	}
	return 0
}

func (c *compiler) trunc(dst int32, t types.Type) {
	if k := truncKind(t); k != 0 {
		c.emit(Instr{Op: OpTrunc, A: dst, B: dst, C: k})
	}
}

func (c *compiler) instr(ins ssa.Instruction) {
	switch v := ins.(type) {
	case *ssa.DebugRef, *ssa.Phi:
		// Phis are resolved by edge moves.
	case *ssa.BinOp:
		c.binop(v)
	case *ssa.UnOp:
		if c.rangeResumeLoad(v) {
			return
		}
		dst, x := c.reg(v), c.reg(v.X)
		b := basic(v.X.Type())
		if b == nil && v.Op != token.MUL && v.Op != token.ARROW {
			unsupported("unop %s on %s", v.Op, v.X.Type())
		}
		if isComplex(v.X.Type()) && v.Op == token.SUB {
			c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return Value{R: -cplx(regs[x])} }})
			break
		}
		switch v.Op {
		case token.NOT:
			c.emit(Instr{Op: OpNot, A: dst, B: x})
		case token.SUB:
			if b.Info()&types.IsFloat != 0 {
				c.emit(Instr{Op: OpNegF, A: dst, B: x})
			} else {
				c.emit(Instr{Op: OpNegI, A: dst, B: x})
				c.trunc(dst, v.Type())
			}
		case token.XOR:
			c.emit(Instr{Op: OpCompl, A: dst, B: x})
			c.trunc(dst, v.Type())
		case token.MUL:
			if lit := c.aggs[v]; lit != nil {
				if st := c.aggStore(lit); st != nil {
					if c.aggStores == nil {
						c.aggStores = map[*ssa.Store]*aggLit{}
					}
					c.aggStores[st] = lit
					break
				}
				c.emit(Instr{Op: OpMakeAgg, A: dst, X: c.aggSiteOf(lit)})
				break
			}
			if base, ok := c.aliasReg(v.X); ok {
				c.emit(Instr{Op: OpMov, A: dst, B: base})
				break
			}
			if fa, ok := v.X.(*ssa.FieldAddr); ok && c.fused[fa] {
				c.emit(Instr{Op: OpFieldLoad, A: dst, B: c.reg(fa.X), C: int32(fa.Field), X: c.shallowSite(v, v.Type())})
				break
			}
			if ia, ok := v.X.(*ssa.IndexAddr); ok && c.fused[ia] {
				c.emit(Instr{Op: OpIndexLoad, F: indexFlags(ia.X.Type(), ia.Index.Type()), A: dst, B: c.reg(ia.X), C: c.reg(ia.Index), X: c.shallowSite(v, v.Type())})
				break
			}
			c.emit(Instr{Op: OpLoad, A: dst, B: x, X: c.shallowSite(v, v.Type())})
		case token.ARROW:
			c.emit(Instr{Op: OpRecv, A: dst, B: x, C: int32(b2i(v.CommaOk)), X: chanElem(v.X.Type())})
		default:
			unsupported("unop %s", v.Op)
		}
	case *ssa.ChangeType:
		dst, x := c.reg(v), c.reg(v.X)
		// A native value changed to another native named type
		// (propagation.HeaderCarrier(req.Header)) must have that type: its
		// methods, and its conversion to an interface, are those of the new type.
		if rt := c.nativeNamedType(v.Type()); rt != nil {
			c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
				val := regs[x]
				if rv, ok := val.R.(reflect.Value); ok && rv.IsValid() && rv.Type() != rt && rv.Type().ConvertibleTo(rt) {
					return Value{N: val.N, R: rv.Convert(rt)}
				}
				return val
			}})
			break
		}
		c.emit(Instr{Op: OpMov, A: dst, B: x})
	case *ssa.ChangeInterface:
		c.emit(Instr{Op: OpMov, A: c.reg(v), B: c.reg(v.X)})
	case *ssa.Convert:
		c.convert(v)
	case *ssa.Call:
		c.call(v)
	case *ssa.Return:
		switch len(v.Results) {
		case 0:
			c.emit(Instr{Op: OpRet, A: -1})
		case 1:
			c.emit(Instr{Op: OpRet, A: c.reg(v.Results[0])})
		default:
			c.emit(Instr{Op: OpRet, A: -2, X: c.regList(v.Results)})
		}
	case *ssa.Jump:
		c.edge(v.Block(), v.Block().Succs[0], true)
	case *ssa.If:
		b := v.Block()
		cond := c.reg(v.Cond)
		if sw := c.switchChain(b); sw != nil {
			if _, _, _, ok := c.fuseCmp(v); ok {
				c.code = c.code[:len(c.code)-1] // drop the compare
			}
			c.emitSwitch(sw)
			break
		}
		if jop, x, y, ok := c.fuseCmp(v); ok {
			c.code = c.code[:len(c.code)-1] // drop the compare
			at := c.emit(Instr{Op: jop, A: x, B: y})
			if hasPhi(b.Succs[0]) || hasPhi(b.Succs[1]) {
				c.edge(b, b.Succs[1], false) // false edge first
				c.code[at].C = int32(len(c.code))
				c.edge(b, b.Succs[0], true)
			} else if b.Succs[0] == c.next {
				// true branch falls through: jump on the negated condition
				c.code[at].Op = negJump[jop]
				c.fixups = append(c.fixups, fixup{at, 2, b.Succs[1]})
			} else {
				c.fixups = append(c.fixups, fixup{at, 2, b.Succs[0]})
				if b.Succs[1] != c.next {
					j := c.emit(Instr{Op: OpJmp})
					c.fixups = append(c.fixups, fixup{j, 0, b.Succs[1]})
				}
			}
			break
		}
		at := c.emit(Instr{Op: OpIf, A: cond})
		if hasPhi(b.Succs[0]) || hasPhi(b.Succs[1]) {
			// Edge stubs.
			c.code[at].B = int32(len(c.code))
			c.edge(b, b.Succs[0], false)
			c.code[at].C = int32(len(c.code))
			c.edge(b, b.Succs[1], true)
		} else {
			c.fixups = append(c.fixups, fixup{at, 1, b.Succs[0]}, fixup{at, 2, b.Succs[1]})
		}
	case *ssa.Panic:
		if c.rangeFuncPanic(v) {
			break
		}
		c.emit(Instr{Op: OpPanic, B: c.reg(v.X)})
	case *ssa.Alloc:
		if c.spills[v] {
			// The register of v gets the stored value (aggv.go). A huge
			// local is still refused at load time (loaddos.go).
			c.m.checkZero(v.Type().(*types.Pointer).Elem())
			break
		}
		if n, ok := byteArrayOnlySliced(v); ok {
			// make([]byte, k) with a constant k: ssa allocates a [k]byte and
			// slices it. The array is never used as an array, so its memory
			// is a Go []byte, as for make([]byte, n) (bytes.go).
			if m := c.m; m.mem != nil && n >= memMin {
				c.emit(Instr{Op: OpFunc, A: c.reg(v), X: func([]Value) Value { return Value{R: &Value{R: m.memBytes(n)}} }})
				break
			}
			c.emit(Instr{Op: OpFunc, A: c.reg(v), X: func([]Value) Value { return Value{R: &Value{R: make([]byte, n)}} }})
			break
		}
		et := v.Type().(*types.Pointer).Elem()
		// The flat path of mem does not build a zero value with m.zero:
		// check the size here (a local [1<<34]byte, new([1<<34]byte)).
		c.m.checkZero(et)
		if c.bytesAlloc(v) {
			// plain data seen as another type through unsafe: real bytes
			// (byteview.go)
			size := gcSizes.Sizeof(et)
			m := c.m
			c.emit(Instr{Op: OpFunc, A: c.reg(v), X: func([]Value) Value {
				if m.mem != nil && size >= memMin {
					return Value{R: &byteView{b: m.memBytes(int(size)), t: et}}
				}
				return Value{R: &byteView{b: make([]byte, size), t: et}}
			}})
			break
		}
		if c.m.frameLocal(v) {
			// The cell stays in the frame (escape.go): a register of its
			// own keeps it from one run of the Alloc to the next.
			cell := c.newReg()
			if c.f.cells == nil {
				c.f.cells = map[int32]int32{}
			}
			c.f.cells[c.reg(v)] = cell
			ms := c.mem(et)
			ms.local = c.m.localSites.Add(1) - 1
			ms.cellVals = 1 + min(c.m.zeroEntries(et), maxZeroElems+1)
			ms.localSize = int32(min(ms.cellVals, maxFreeCells+1))
			c.f.locals = append(c.f.locals, localCell{reg: cell, site: ms})
			c.emit(Instr{Op: OpAllocLocal, A: c.reg(v), B: cell, X: ms})
			break
		}
		c.emit(Instr{Op: OpAlloc, A: c.reg(v), X: c.mem(et)})
	case *ssa.Store:
		if lit := c.aggStores[v]; lit != nil {
			c.emit(Instr{Op: OpStoreAgg, A: c.reg(v.Addr), X: c.aggStoreSiteOf(lit, c.mem(v.Val.Type()))})
			break
		}
		if a, ok := v.Addr.(*ssa.Alloc); ok && c.spills[a] {
			c.emit(Instr{Op: OpMov, A: c.reg(a), B: c.reg(v.Val)})
			break
		}
		if fa, ok := v.Addr.(*ssa.FieldAddr); ok && isBlankField(fa) {
			// T{1, 2, 3} with a blank field: Go ignores the value, the
			// field stays zero.
			break
		}
		if fa, ok := v.Addr.(*ssa.FieldAddr); ok && c.fused[fa] {
			c.emit(Instr{Op: OpFieldStore, A: c.reg(fa.X), B: c.reg(v.Val), C: int32(fa.Field), X: c.mem(v.Val.Type())})
			break
		}
		if ia, ok := v.Addr.(*ssa.IndexAddr); ok && c.fused[ia] {
			c.emit(Instr{Op: OpIndexStore, F: indexFlags(ia.X.Type(), ia.Index.Type()), A: c.reg(ia.X), B: c.reg(v.Val), C: c.reg(ia.Index), X: c.mem(v.Val.Type())})
			break
		}
		c.emit(Instr{Op: OpStore, A: c.reg(v.Addr), B: c.reg(v.Val), X: c.mem(v.Val.Type())})
	case *ssa.FieldAddr:
		if base, ok := c.aliasReg(v.X); ok {
			st := v.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Struct)
			c.emit(Instr{Op: OpField, A: c.reg(v), B: base, C: int32(v.Field), X: st.Field(v.Field).Type()})
			break
		}
		if c.fusable(v) {
			c.fused[v] = true
			break
		}
		c.emit(Instr{Op: OpFieldAddr, A: c.reg(v), B: c.reg(v.X), C: int32(v.Field)})
	case *ssa.Field:
		st := v.X.Type().Underlying().(*types.Struct)
		c.emit(Instr{Op: OpField, A: c.reg(v), B: c.reg(v.X), C: int32(v.Field), X: st.Field(v.Field).Type()})
	case *ssa.IndexAddr:
		if base, ok := c.aliasReg(v.X); ok {
			at := v.X.Type().Underlying().(*types.Pointer).Elem()
			c.emit(Instr{Op: OpIndex, F: indexFlags(at, v.Index.Type()), A: c.reg(v), B: base, C: c.reg(v.Index)})
			break
		}
		if c.fusable(v) {
			c.fused[v] = true
			break
		}
		c.emit(Instr{Op: OpIndexAddr, F: indexFlags(v.X.Type(), v.Index.Type()), A: c.reg(v), B: c.reg(v.X), C: c.reg(v.Index)})
	case *ssa.Index:
		c.emit(Instr{Op: OpIndex, F: indexFlags(v.X.Type(), v.Index.Type()), A: c.reg(v), B: c.reg(v.X), C: c.reg(v.Index)})
	case *ssa.Slice:
		s := &sliceSite{lo: -1, hi: -1, max: -1, t: v.Type()}
		if v.Low != nil {
			s.lo, s.loU = c.reg(v.Low), isUnsigned(v.Low.Type())
		}
		if v.High != nil {
			s.hi, s.hiU = c.reg(v.High), isUnsigned(v.High.Type())
		}
		if v.Max != nil {
			s.max, s.maxU = c.reg(v.Max), isUnsigned(v.Max.Type())
		}
		if b := basic(v.X.Type()); b != nil && b.Info()&types.IsString != 0 {
			s.kind = sliceOfString
		} else if _, ok := v.X.Type().Underlying().(*types.Pointer); ok {
			s.kind = sliceOfArray
			if a, ok := v.X.(*ssa.Alloc); ok && a.Comment == "makeslice" {
				s.makeslice = true
				if at, ok := v.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Array); ok {
					s.esize = uint64(gcSizes.Sizeof(at.Elem()))
				}
			}
		}
		c.emit(Instr{Op: OpSlice, A: c.reg(v), B: c.reg(v.X), X: s})
	case *ssa.SliceToArrayPointer:
		c.sliceToArrayPointer(v)
	case *ssa.MakeSlice:
		elem := v.Type().Underlying().(*types.Slice).Elem()
		s := c.mem(elem)
		s.size = uint64(gcSizes.Sizeof(elem))
		s.entries = c.m.zeroEntries(elem) // checked at run time (checkMakeSlice)
		c.emit(Instr{Op: OpMakeSlice, A: c.reg(v), B: c.reg(v.Len), C: c.reg(v.Cap), X: s})
	case *ssa.MakeInterface:
		rt := c.m.rtypeOf(v.X.Type())
		if s := c.m.boxSiteOf(v.X.Type(), rt); s != nil {
			c.emit(Instr{Op: OpMakeIfaceBox, A: c.reg(v), B: c.reg(v.X), X: s})
			break
		}
		c.emit(Instr{Op: OpMakeInterface, A: c.reg(v), B: c.reg(v.X), X: rt})
	case *ssa.TypeAssert:
		if isMethodValueNilCheck(v) {
			// i.M with a nil interface i panics with a nil dereference in
			// gc, not with the error of the assertion that go/ssa uses
			x := c.reg(v.X)
			c.emit(Instr{Op: OpFunc, A: c.reg(v), X: func(regs []Value) Value {
				if iv, _ := regs[x].R.(*Iface); iv == nil {
					panic(errNilDeref)
				}
				return regs[x]
			}})
			break
		}
		s := &assertSite{t: v.AssertedType, x: v.X.Type(), commaOk: v.CommaOk, zero: c.m.zero(v.AssertedType)}
		if it, ok := v.AssertedType.Underlying().(*types.Interface); ok {
			s.iface = it
		} else {
			s.concrete = c.m.rtypeOf(v.AssertedType)
		}
		c.emit(Instr{Op: OpTypeAssert, A: c.reg(v), B: c.reg(v.X), X: s})
	case *ssa.Extract:
		if optTuple {
			break
		}
		c.emit(Instr{Op: OpExtract, A: c.reg(v), B: c.reg(v.Tuple), C: int32(v.Index)})
	case *ssa.MakeClosure:
		c.emit(Instr{Op: OpMakeClosure, A: c.reg(v), X: &closureSite{fn: c.m.funcOf(v.Fn.(*ssa.Function)), binds: c.regList(v.Bindings)}})
	case *ssa.MakeMap:
		// Maps of native named types (http.Header, url.Values) live in native
		// memory, so that native methods and native code holding them see
		// interpreted updates and vice versa.
		if rt := c.m.nativeMapType(v.Type()); rt != nil {
			c.emit(Instr{Op: OpFunc, A: c.reg(v), X: func([]Value) Value { return Value{R: reflect.MakeMap(rt)} }})
			break
		}
		c.emit(Instr{Op: OpMakeMap, A: c.reg(v)})
	case *ssa.MapUpdate:
		c.emit(Instr{Op: OpMapUpdate, A: c.reg(v.Map), B: c.reg(v.Key), C: c.reg(v.Value), X: c.mapSite(v.Map.Type())})
	case *ssa.Defer:
		d := c.callDesc(&v.Call)
		if v.DeferStack != nil {
			d.stack = c.reg(v.DeferStack)
		}
		c.emit(Instr{Op: OpDefer, X: d})
	case *ssa.Go:
		d := c.callDesc(&v.Call)
		d.goStmt = true
		c.emit(Instr{Op: OpGo, X: d})
	case *ssa.RunDefers:
		c.emit(Instr{Op: OpRunDefers})
	case *ssa.MakeChan:
		in := Instr{Op: OpMakeChan, A: c.reg(v), B: c.reg(v.Size)}
		if rt := nativeChanType(chanElem(v.Type())); rt != nil {
			in.X = rt // nativechan.go
		}
		c.emit(in)
	case *ssa.Send:
		c.emit(Instr{Op: OpSend, A: c.reg(v.Chan), B: c.reg(v.X), X: chanElem(v.Chan.Type())})
	case *ssa.Select:
		c.emit(Instr{Op: OpSelect, A: c.reg(v), X: c.selectSite(v)})
	case *ssa.Range:
		var ms *mapSite
		if _, ok := v.X.Type().Underlying().(*types.Map); ok {
			ms = c.mapSite(v.X.Type())
			ms.noKey, ms.noElem = rangeUnused(v)
		}
		c.emit(Instr{Op: OpRange, A: c.reg(v), B: c.reg(v.X), X: ms})
	case *ssa.Next:
		c.emit(Instr{Op: OpNext, A: c.reg(v), B: c.reg(v.Iter)})
	case *ssa.Lookup:
		s := &lookupSite{commaOk: v.CommaOk}
		if b := basic(v.X.Type()); b != nil && b.Info()&types.IsString != 0 {
			s.str, s.idx = true, indexFlags(v.X.Type(), v.Index.Type())
		} else {
			s.zero = c.m.zero(v.X.Type().Underlying().(*types.Map).Elem())
			s.ms = c.mapSite(v.X.Type())
		}
		c.emit(Instr{Op: OpLookup, A: c.reg(v), B: c.reg(v.X), C: c.reg(v.Index), X: s})
	default:
		unsupported("instruction %T (%s)", ins, ins)
	}
}

type memSite struct {
	t    types.Type
	deep bool // struct/array: copy on load/store
	// shallow: a load through this site may share the cells it loads
	// while no AOT shadow exists (loads.go, copyLoad)
	shallow bool
	zero    Value  // zero value of t
	size    uint64 // OpMakeSlice: size of t with gc
	flat    int    // > 0: t is an array of this length whose zero is all Value{}
	// entries: OpMakeSlice: Value entries of the zero value of one element
	// (zeroEntries), to bound make([]T, n) for arrays and structs
	entries int64
	// zeroSized: t uses no memory (large slices of t are a zslice)
	zeroSized bool
	// bytes: t is byte; make([]byte) is a Go []byte (bytes.go)
	bytes bool
	// local: the index of the free list of an OpAllocLocal site in a ctx,
	// localSize: the Values of one of its cells (escape.go)
	local, localSize int32
	// cellVals: the Values of one cell of an OpAllocLocal site, counted in
	// MaxStack (stacklimit.go)
	cellVals int64
	// word: t is one machine word in Go (or an interface): loads and
	// stores of a *Value cell use loadWord and storeWord, so that a racy
	// read never tears (wordcell.go)
	word bool
}

// newMemSite returns the site of a load or a store of type t made outside
// the compiler (a hook): deep and word set as the compiler sets them.
func newMemSite(t types.Type) *memSite {
	return &memSite{t: t, deep: needsCopy(t), word: wordKind(t)}
}

func (c *compiler) mem(t types.Type) *memSite {
	s := newMemSite(t)
	s.zeroSized, s.bytes = zeroSized(t), optBytes && isByteType(t)
	if a, ok := t.Underlying().(*types.Array); ok && a.Len() > 0 && flatElem(a.Elem()) && !c.m.isNativeNamed(t) &&
		!(a.Len() >= zsliceMin && zeroSized(a.Elem())) && c.m.zero(a.Elem()) == (Value{}) {
		// OpAlloc and OpMakeSlice make a fresh []Value: no zero template.
		// A template would keep a large array alive for the whole run
		// (GOROOT test fixedbugs/issue15277.go).
		s.flat = int(a.Len())
		return s
	}
	s.zero = c.m.zero(t)
	return s
}

// byteArrayOnlySliced reports whether the Alloc v is a [n]byte that is only
// sliced, or written element by element and sliced (the array of
// make([]byte, k) with a constant k, or of a []byte{...} literal), and
// gives n. Such an array is never used as an array, so its memory is a Go
// []byte (bytes.go): real bytes, which unsafe can view (byteview.go).
func byteArrayOnlySliced(v *ssa.Alloc) (int, bool) {
	a, ok := v.Type().(*types.Pointer).Elem().Underlying().(*types.Array)
	if !optBytes || !ok || !isByteType(a.Elem()) || v.Referrers() == nil {
		return 0, false
	}
	for _, r := range *v.Referrers() {
		switch r := r.(type) {
		case *ssa.Slice:
		case *ssa.IndexAddr:
			if r.Referrers() == nil {
				return 0, false
			}
			for _, u := range *r.Referrers() {
				if st, ok := u.(*ssa.Store); !ok || st.Addr != r {
					return 0, false
				}
			}
		default:
			return 0, false
		}
	}
	return int(a.Len()), true
}

// sliceElem returns the element type of the slice type t, or nil when t is
// not a slice.
func sliceElem(t types.Type) types.Type {
	if s, ok := t.Underlying().(*types.Slice); ok {
		return s.Elem()
	}
	return nil
}

func isBlankField(fa *ssa.FieldAddr) bool {
	st, ok := fa.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Struct)
	return ok && st.Field(fa.Field).Name() == "_"
}

func hasPhi(b *ssa.BasicBlock) bool {
	return len(b.Instrs) > 0 && isPhi(b.Instrs[0])
}

func isPhi(i ssa.Instruction) bool { _, ok := i.(*ssa.Phi); return ok }

// edge emits the parallel phi moves for from->to and a jump to `to`.
func (c *compiler) edge(from, to *ssa.BasicBlock, canFall bool) {
	idx := -1
	for i, p := range to.Preds {
		if p == from {
			idx = i
			break
		}
	}
	var dsts, srcs []int32
	for _, ins := range to.Instrs {
		phi, ok := ins.(*ssa.Phi)
		if !ok {
			break
		}
		d, s := c.reg(phi), c.reg(phi.Edges[idx])
		if d == s {
			continue // the value was written into the phi's register (phicoalesce.go)
		}
		dsts = append(dsts, d)
		srcs = append(srcs, s)
	}
	conflict := false
	set := map[int32]bool{}
	for _, d := range dsts {
		set[d] = true
	}
	for _, s := range srcs {
		if set[s] {
			conflict = true
		}
	}
	if conflict {
		tmps := make([]int32, len(srcs))
		for i, s := range srcs {
			tmps[i] = c.newReg()
			c.emit(Instr{Op: OpMov, A: tmps[i], B: s})
		}
		srcs = tmps
	}
	for i := range dsts {
		c.emit(Instr{Op: OpMov, A: dsts[i], B: srcs[i]})
	}
	if optSuper && canFall && to == c.next {
		return // fall through
	}
	if n := len(c.code); optSwitch && len(dsts) > 0 {
		// The last move and the jump in one instruction (OpMovJmp). The
		// loop above just emitted the moves of dsts, so code[n-1] is the
		// last OpMov.
		c.code[n-1].Op = OpMovJmp
		c.fixups = append(c.fixups, fixup{n - 1, 2, to})
		return
	}
	at := c.emit(Instr{Op: OpJmp})
	c.fixups = append(c.fixups, fixup{at, 0, to})
}

func (c *compiler) binop(v *ssa.BinOp) {
	dst, x, y := c.reg(v), c.reg(v.X), c.reg(v.Y)
	t := v.X.Type()
	if isComplex(t) {
		c.complexBinop(v, dst, x, y)
		return
	}
	b := basic(t)
	isF := b != nil && b.Info()&types.IsFloat != 0
	isS := b != nil && b.Info()&types.IsString != 0
	isU := b != nil && b.Info()&types.IsUnsigned != 0
	isI := b != nil && b.Info()&(types.IsInteger|types.IsBoolean) != 0
	var op Op
	pick := func(i, u, f, s Op) Op {
		switch {
		case isS:
			return s
		case isF:
			return f
		case isU:
			return u
		case isI:
			return i
		}
		return 0
	}
	switch v.Op {
	case token.ADD:
		op = pick(OpAddI, OpAddI, OpAddF, OpAddS)
	case token.SUB:
		op = pick(OpSubI, OpSubI, OpSubF, 0)
	case token.MUL:
		op = pick(OpMulI, OpMulI, OpMulF, 0)
	case token.QUO:
		op = pick(OpDivI, OpDivU, OpDivF, 0)
	case token.REM:
		op = pick(OpRemI, OpRemU, 0, 0)
	case token.AND:
		op = OpAndI
	case token.OR:
		op = OpOrI
	case token.XOR:
		op = OpXorI
	case token.AND_NOT:
		op = OpAndNotI
	case token.SHL:
		op = OpShlI
	case token.SHR:
		op = pick(OpShrI, OpShrU, 0, 0)
	case token.EQL:
		op = pick(OpEqI, OpEqI, OpEqF, OpEqS)
		if op == 0 {
			c.emit(Instr{Op: OpEqV, A: dst, B: x, C: y, X: eqSite(t)})
			return
		}
	case token.NEQ:
		op = pick(OpNeI, OpNeI, 0, OpNeS)
		if op == 0 {
			c.emit(Instr{Op: OpNeV, A: dst, B: x, C: y, X: eqSite(t)})
			return
		}
	case token.LSS:
		op = pick(OpLtI, OpLtU, OpLtF, OpLtS)
	case token.LEQ:
		op = pick(OpLeI, OpLeU, OpLeF, 0)
		if isS {
			c.emit(Instr{Op: OpLtS, A: dst, B: y, C: x})
			c.emit(Instr{Op: OpNot, A: dst, B: dst})
			return
		}
	case token.GTR:
		op = pick(OpGtI, OpGtU, OpGtF, 0)
		if isS {
			c.emit(Instr{Op: OpLtS, A: dst, B: y, C: x})
			return
		}
	case token.GEQ:
		op = pick(OpGeI, OpGeU, OpGeF, 0)
		if isS {
			c.emit(Instr{Op: OpLtS, A: dst, B: x, C: y})
			c.emit(Instr{Op: OpNot, A: dst, B: dst})
			return
		}
	}
	if op == 0 {
		unsupported("binop %s on %s", v.Op, t)
	}
	in := Instr{Op: op, A: dst, B: x, C: y}
	if (v.Op == token.SHL || v.Op == token.SHR) && !isUnsigned(v.Y.Type()) {
		if k, ok := v.Y.(*ssa.Const); !ok {
			in.F = shiftSigned // a negative count panics
		} else if k.Value != nil && constant.Sign(k.Value) < 0 {
			// A variable count that ssa turned into a constant
			// (y := -2; x >> y): the shift panics when it runs.
			c.emit(Instr{Op: OpFunc, A: dst, X: func([]Value) Value { panic(errNegShift) }})
			return
		}
	}
	c.emit(in)
	switch v.Op {
	case token.ADD, token.SUB, token.MUL, token.SHL:
		if !isS {
			c.trunc(dst, v.Type()) // int wrap, or float32 rounding
		}
	case token.QUO:
		// Signed overflow: int8(-128) / -1 wraps to -128. float32 results
		// are rounded.
		if !isU {
			c.trunc(dst, v.Type())
		}
	}
}

// numKinds packs the basic kinds of a numeric conversion for OpI2F and OpF2I
// (see convNum). It returns 0 for the common cases that exec does inline: a
// signed integer (or a narrow unsigned one, stored zero-extended) to float64,
// and float64 to int64 (or int when int has 64 bits).
func numKinds(from, to *types.Basic) int32 {
	f, t := from.Kind(), to.Kind()
	switch {
	case t == types.Float64 && from.Info()&types.IsInteger != 0 &&
		f != types.Uint && f != types.Uint64 && f != types.Uintptr:
		return 0
	case f == types.Float64 && (t == types.Int64 || t == types.Int && !intIs32):
		return 0
	}
	return int32(f) | int32(t)<<8
}

func (c *compiler) convert(v *ssa.Convert) {
	dst, x := c.reg(v), c.reg(v.X)
	from, to := v.X.Type().Underlying(), v.Type().Underlying()
	fb, tb := basic(from), basic(to)
	switch {
	case fb != nil && fb.Kind() == types.UnsafePointer && tb != nil && tb.Kind() == types.Uintptr:
		c.unsafeToUintptr(dst, v)
	case fb != nil && fb.Info()&types.IsInteger != 0 && tb != nil && tb.Kind() == types.UnsafePointer:
		c.uintptrToUnsafe(dst, x)
	case fb != nil && fb.Kind() == types.UnsafePointer && isPointer(to):
		c.unsafeView(v)
	case tb != nil && tb.Kind() == types.UnsafePointer && isPointer(from):
		c.toUnsafe(dst, v.X)
	case fb != nil && fb.Kind() == types.UnsafePointer && tb == nil, tb != nil && tb.Kind() == types.UnsafePointer && fb == nil:
		// *T <-> unsafe.Pointer: the cell is shared; scalar punning
		// (float64 <-> uint64 ...) is exact because N holds raw bits.
		c.emit(Instr{Op: OpMov, A: dst, B: x})
	case fb != nil && tb != nil && fb.Info()&types.IsInteger != 0 && tb.Info()&types.IsInteger != 0:
		c.emit(Instr{Op: OpMov, A: dst, B: x})
		c.trunc(dst, to)
	case fb != nil && tb != nil && fb.Info()&types.IsInteger != 0 && tb.Info()&types.IsFloat != 0:
		c.emit(Instr{Op: OpI2F, A: dst, B: x, C: numKinds(fb, tb)})
	case fb != nil && tb != nil && fb.Info()&types.IsFloat != 0 && tb.Info()&types.IsInteger != 0:
		c.emit(Instr{Op: OpF2I, A: dst, B: x, C: numKinds(fb, tb)})
	case fb != nil && tb != nil && fb.Info()&types.IsFloat != 0 && tb.Info()&types.IsFloat != 0:
		c.emit(Instr{Op: OpMov, A: dst, B: x})
		c.trunc(dst, to) // float64 -> float32 rounds
	case fb != nil && tb != nil && fb.Info()&types.IsInteger != 0 && tb.Info()&types.IsString != 0:
		c.emit(Instr{Op: OpRune2Str, A: dst, B: x})
	case fb != nil && fb.Info()&types.IsString != 0 && isRuneSlice(to):
		c.emit(Instr{Op: OpStr2Runes, A: dst, B: x})
	case tb != nil && tb.Info()&types.IsString != 0 && isRuneSlice(from):
		c.emit(Instr{Op: OpRunes2Str, A: dst, B: x})
	case fb != nil && fb.Info()&types.IsString != 0 && tb == nil:
		// C == 1: a constant string. gc gives []byte("...") a capacity
		// equal to its length (a [len]byte array), so cap and the Cap of
		// a reflect.SliceHeader view print as with go run.
		var exact int32
		if _, ok := v.X.(*ssa.Const); ok {
			exact = 1
		}
		c.emit(Instr{Op: OpStr2Bytes, A: dst, B: x, C: exact})
	case tb != nil && tb.Info()&types.IsString != 0 && fb == nil:
		c.emit(Instr{Op: OpBytes2Str, A: dst, B: x})
	case fb != nil && tb != nil && fb.Info()&types.IsString != 0 && tb.Info()&types.IsString != 0:
		c.emit(Instr{Op: OpMov, A: dst, B: x})
	case isComplex(from) && isComplex(to):
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return cplxVal(cplx(regs[x]), to) }})
	default:
		unsupported("conversion %s -> %s", v.X.Type(), v.Type())
	}
}

func (c *compiler) call(v *ssa.Call) {
	cc := v.Call
	dst := c.reg(v)
	if cc.IsInvoke() {
		c.emit(Instr{Op: OpInvoke, A: dst, B: c.reg(cc.Value), X: &invokeSite{
			name: cc.Method.Name(), args: c.regList(cc.Args), sig: cc.Method.Type().(*types.Signature),
			memCheck: c.m.mem != nil && nativeMemMethodNames[cc.Method.Name()],
		}})
		return
	}
	switch callee := cc.Value.(type) {
	case *ssa.Builtin:
		args := cc.Args
		switch callee.Name() {
		case "len":
			c.emit(Instr{Op: OpLen, A: dst, B: c.reg(args[0])})
		case "cap":
			c.emit(Instr{Op: OpCap, A: dst, B: c.reg(args[0])})
		case "append":
			et := sliceElem(args[0].Type())
			if elems, ok := c.appendV[v]; ok && et != nil {
				site := &appendSite{elems: c.regList(elems), et: et, deep: needsCopy(et), bytes: optBytes && isByteType(et), word: wordKind(et)}
				c.emit(Instr{Op: OpAppendV, A: dst, B: c.reg(args[0]), X: site})
				break
			}
			c.emit(Instr{Op: OpAppend, A: dst, B: c.reg(args[0]), C: c.reg(args[1]), X: et})
		case "copy":
			c.emit(Instr{Op: OpCopy, A: dst, B: c.reg(args[0]), C: c.reg(args[1]), X: sliceElem(args[0].Type())})
		case "print", "println":
			ts := make([]types.Type, len(args))
			for i, a := range args {
				ts[i] = a.Type()
			}
			c.emit(Instr{Op: OpPrintln, X: &printSite{args: c.regList(args), types: ts, ln: callee.Name() == "println"}})
		case "min", "max":
			rs, isMax := c.regList(args), callee.Name() == "max"
			b := basic(args[0].Type())
			less := func(x, y Value) bool { return x.N < y.N }
			switch {
			case b.Info()&types.IsString != 0:
				less = func(x, y Value) bool { return x.R.(string) < y.R.(string) }
			case b.Info()&types.IsUnsigned != 0:
				less = func(x, y Value) bool { return uint64(x.N) < uint64(y.N) }
			}
			if b.Info()&types.IsFloat != 0 {
				// The builtins of the host: NaN wins, also against an
				// infinity, and -0 < +0. math.Max and math.Min differ
				// there: math.Max(NaN, +Inf) is +Inf.
				pick := func(x, y float64) float64 { return min(x, y) }
				if isMax {
					pick = func(x, y float64) float64 { return max(x, y) }
				}
				c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
					r := f64(regs[rs[0]])
					for _, i := range rs[1:] {
						r = pick(r, f64(regs[i]))
					}
					return Value{N: fv(r)}
				}})
				break
			}
			c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
				r := regs[rs[0]]
				for _, i := range rs[1:] {
					if !isMax && less(regs[i], r) || isMax && less(r, regs[i]) {
						r = regs[i]
					}
				}
				return r
			}})
		case "clear":
			r, m, t := c.reg(args[0]), c.m, args[0].Type()
			c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
				m.execClear(regs[r].R, t)
				return Value{}
			}})
		case "complex", "real", "imag":
			c.complexBuiltin(callee.Name(), dst, args, v.Type())
		case "recover":
			c.emit(Instr{Op: OpRecover, A: dst})
		case "close":
			c.emit(Instr{Op: OpClose, B: c.reg(args[0])})
		case "delete":
			c.emit(Instr{Op: OpDelete, A: c.reg(args[0]), B: c.reg(args[1]), X: c.mapSite(args[0].Type())})
		case "ssa:deferstack":
			c.deferStack = true
			c.emit(Instr{Op: OpDeferStack, A: dst})
		case "Sizeof", "Alignof", "Offsetof":
			c.unsafeSize(callee.Name(), dst, v)
		case "ssa:wrapnilchk":
			// The wrapper of a value method called through a pointer:
			// a nil pointer panics with the message of the gc runtime.
			c.emit(Instr{Op: OpWrapNilChk, A: dst, B: c.reg(args[0]), X: panicWrapError(args[0].Type(), args[2])})
		case "Add":
			c.unsafeAdd(dst, args[0], args[1])
		case "StringData", "SliceData":
			c.unsafeData(callee.Name(), dst, args[0])
		case "String":
			c.unsafeSlice("String", dst, v, args[0], args[1], types.Typ[types.Byte])
		case "Slice":
			c.unsafeSlice("Slice", dst, v, args[0], args[1], sliceElem(v.Type()))
		default:
			unsupported("builtin %s", callee.Name())
		}
	case *ssa.Function:
		if callee.Name() == "init" && callee.Signature.Recv() == nil && !c.m.isInterp(callee) {
			return // dependency init: already run natively
		}
		if msg := c.m.lostProblem(callee, cc.Args); msg != "" {
			panic(&CompileError{Msg: msg}) // lostmethods.go
		}
		// Native calls that allocate from their arguments, with a quota
		// (nativemem.go).
		if h := c.m.nativeMemHook(callee); h != nil {
			m := c.m
			c.emitGo(dst, func(a []Value) Value { return h(m, a) }, c.regList(cc.Args))
			return
		}
		if size := c.m.nativeMemCheck(callee); size != nil {
			c.emitNativeMemCheck(size, c.regList(cc.Args))
			defer c.emitNativeMemCount(dst)
		}
		if c.m.progVR {
			o := callee
			if callee.Origin() != nil {
				o = callee.Origin()
			}
			if o.Pkg != nil && isReflectPkg(o.Pkg.Pkg) {
				c.emit(Instr{Op: OpFunc, A: dst, X: c.m.vr.intrinsic(callee, c.regList(cc.Args))})
				return
			}
		}
		if h := c.m.storageHook(callee); h != nil {
			m, f := c.m, callee
			c.emitGo(dst, func(a []Value) Value { return h(m, f, a) }, c.regList(cc.Args))
			return
		}
		if callee.Pkg != nil && !c.m.isInterp(callee) {
			if h := c.m.hookFor(callee); h != nil {
				m, deep := c.m, c.m.deepHook(callee)
				c.emitGo(dst, func(a []Value) Value {
					if deep != nil {
						if r, ok := deep(a); ok {
							return r // a value nested too deep: deepprint.go
						}
					}
					r := h(m, a)
					m.flushShadows()
					return r
				}, c.regList(cc.Args))
				return
			}
		}
		// Native functions: bodyless, or with a body built from source but
		// not interpreted (full-syntax loads).
		if callee.Pkg != nil && !c.m.isInterp(callee) && callee.Signature.Recv() == nil {
			if h := ctxHooks[callee.Pkg.Pkg.Path()+"."+callee.Name()]; h != nil {
				args := c.regList(cc.Args)
				c.emit(Instr{Op: OpCtxFunc, A: dst, X: func(cx *ctx, regs []Value) Value {
					av := make([]Value, len(args))
					for i, r := range args {
						av[i] = regs[r]
					}
					return h(cx, av)
				}})
				return
			}
			if callee.Pkg.Pkg.Path() == "runtime" && callee.Name() == "GC" {
				c.emit(Instr{Op: OpGC})
				return
			}
			if callee.Pkg.Pkg.Path() == "runtime" && callee.Name() == "KeepAlive" {
				// No code: the SSA liveness of gclive.go already keeps
				// the argument alive up to here. A native call would copy
				// the object and could keep the copy.
				return
			}
			if h := hookOf(callee); h != nil {
				m := c.m
				c.emitGo(dst, func(a []Value) Value { return h(m, a) }, c.regList(cc.Args))
				return
			}
			if g := goIntrinsics[callee.Pkg.Pkg.Path()+"."+callee.Name()]; g != nil {
				c.emitGo(dst, g, c.regList(cc.Args))
				return
			}
			if h := c.m.bodylessHook(callee); h != nil {
				m := c.m
				if t := c.typedMemmove(callee, cc.Args); t != nil {
					// a copy of a whole value of the type the SSA shows
					// on both sides (linkname.go)
					c.emitGo(dst, func(a []Value) Value { return m.memmoveTyped(a, t, h) }, c.regList(cc.Args))
					return
				}
				c.emitGo(dst, func(a []Value) Value { return h(m, a) }, c.regList(cc.Args))
				return
			}
		}
		if isVMGeneric(callee) {
			c.emit(Instr{Op: OpFunc, A: dst, X: c.m.genericIntrinsic(callee, c.regList(cc.Args))})
			return
		}
		// The escape summaries (escape.go) trust staticCallRunsBytecode:
		// a route above that runs other code for an interpreted function
		// must also make it return false.
		if c.m.staticCallRunsBytecode(callee) {
			c.emit(Instr{Op: OpCall, A: dst, X: &callSite{fn: c.m.funcOf(callee), args: c.regList(cc.Args)}})
			return
		}
		if callee.Name() == "init" && callee.Signature.Recv() == nil {
			return // dependency init: already run natively
		}
		if f := c.m.aotRouteCall(callee, c.regList(cc.Args)); f != nil {
			c.emit(Instr{Op: OpFunc, A: dst, X: f})
			return
		}
		if f := c.m.logRouteCall(callee, c.regList(cc.Args)); f != nil {
			c.emit(Instr{Op: OpFunc, A: dst, X: f}) // aotroutelog.go
			return
		}
		if f := c.m.jsonStreamCall(callee, c.regList(cc.Args)); f != nil {
			c.emit(Instr{Op: OpFunc, A: dst, X: f}) // aotroutejson.go
			return
		}
		if f := c.m.lostCall(callee, c.regList(cc.Args)); f != nil {
			c.emit(Instr{Op: OpFunc, A: dst, X: f}) // lostmethods.go
			return
		}
		c.emit(Instr{Op: OpCallNative, A: dst, X: c.m.nativeSite(callee, c.regList(cc.Args))})
	default:
		c.emit(Instr{Op: OpCallClosure, A: dst, B: c.reg(cc.Value), X: &callSite{args: c.regList(cc.Args), sig: cc.Signature()}})
	}
}

// isMethodWrapper reports whether f is a wrapper that ssa made for a method:
// a bound method (x.M), a thunk (T.M), or a promoted or pointer receiver
// method. gc hides these frames from recover.
func isMethodWrapper(f *ssa.Function) bool {
	if f.Synthetic == "" {
		return false
	}
	_, ok := f.Object().(*types.Func)
	return ok
}

// nativeNamedType returns the native type of a named non-interface type of a
// native package, or nil.
func (c *compiler) nativeNamedType(t types.Type) (rt reflect.Type) {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || c.m.interpPkg(named.Obj().Pkg()) || named.TypeArgs().Len() > 0 {
		return nil
	}
	if _, iface := named.Underlying().(*types.Interface); iface {
		return nil
	}
	defer func() {
		if recover() != nil {
			rt = nil
		}
	}()
	return c.m.reflectType(t)
}
