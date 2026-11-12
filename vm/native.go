package vm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/types"
	"io"
	"log"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"

	"golang.org/x/tools/go/ssa"

	"github.com/traefik/yaegi/v2/stdsyms"
)

// stdSym falls back to the generated stdlib symbol table of the Go release
// yaegi v2 is built with (package stdsyms, restricted as yaegi's stdlib, see
// stdTable).
func (m *Machine) stdSym(pkg *types.Package, name string) (reflect.Value, bool) {
	v, ok := m.stdTable(pkg)[name]
	return v, ok
}

// stdTable returns the symbols of the standard library package pkg. A
// machine with the standard library as in Go (m.realStd) gets the real
// os.Exit and log.Fatal, and the tables of os/exec and syscall
// (stdsyms.Unrestricted).
func (m *Machine) stdTable(pkg *types.Package) map[string]reflect.Value {
	return m.stdTableKey(pkg.Path() + "/" + pkg.Name())
}

// stdTableKey is stdTable for the key "path/name" of a package.
func (m *Machine) stdTableKey(key string) map[string]reflect.Value {
	if m.realStd {
		if syms, ok := unrestrictedSyms()[key]; ok {
			return syms
		}
	} else if m.unsafeSyscall && key == "syscall/syscall" {
		return m.syscallSubset()
	} else if key == "os/exec/exec" && m.execMode != ExecRefused {
		return m.execTable()
	}
	return stdsyms.Symbols[key]
}

// unrestrictedSyms holds, for each package of stdsyms.Unrestricted, its
// entries of stdsyms.Symbols with those of stdsyms.Unrestricted added or
// replaced. It is built once.
var unrestrictedSyms = sync.OnceValue(func() map[string]map[string]reflect.Value {
	all := make(map[string]map[string]reflect.Value, len(stdsyms.Unrestricted))
	for key, extra := range stdsyms.Unrestricted {
		syms := maps.Clone(stdsyms.Symbols[key])
		if syms == nil {
			syms = map[string]reflect.Value{}
		}
		maps.Copy(syms, extra)
		all[key] = syms
	}
	return all
})

// ---- symbol registry (hand-written entries, before stdsyms) ----

var nativeFuncs = map[string]reflect.Value{
	"fmt.Println":                   reflect.ValueOf(fmt.Println),
	"fmt.Printf":                    reflect.ValueOf(fmt.Printf),
	"fmt.Sprintf":                   reflect.ValueOf(fmt.Sprintf),
	"fmt.Sprint":                    reflect.ValueOf(fmt.Sprint),
	"fmt.Print":                     reflect.ValueOf(fmt.Print),
	"fmt.Errorf":                    reflect.ValueOf(fmt.Errorf),
	"strconv.Itoa":                  reflect.ValueOf(strconv.Itoa),
	"strconv.Atoi":                  reflect.ValueOf(strconv.Atoi),
	"strings.Repeat":                reflect.ValueOf(strings.Repeat),
	"strings.ToUpper":               reflect.ValueOf(strings.ToUpper),
	"strings.NewReader":             reflect.ValueOf(strings.NewReader),
	"strings.Join":                  reflect.ValueOf(strings.Join),
	"errors.New":                    reflect.ValueOf(errors.New),
	"sort.Ints":                     reflect.ValueOf(sort.Ints),
	"sort.Slice":                    reflect.ValueOf(sort.Slice),
	"io.ReadAll":                    reflect.ValueOf(io.ReadAll),
	"net/http.NewServeMux":          reflect.ValueOf(http.NewServeMux),
	"net/http/httptest.NewRecorder": reflect.ValueOf(httptest.NewRecorder),
	"net/http/httptest.NewRequest":  reflect.ValueOf(httptest.NewRequest),
}

var nativeTypes = map[string]reflect.Type{
	"net/http.Request":                   reflect.TypeOf(http.Request{}),
	"net/http.ResponseWriter":            reflect.TypeOf((*http.ResponseWriter)(nil)).Elem(),
	"net/http.Handler":                   reflect.TypeOf((*http.Handler)(nil)).Elem(),
	"net/http.HandlerFunc":               reflect.TypeOf(http.HandlerFunc(nil)),
	"net/http.ServeMux":                  reflect.TypeOf(http.ServeMux{}),
	"net/http.Header":                    reflect.TypeOf(http.Header{}),
	"net/http/httptest.ResponseRecorder": reflect.TypeOf(httptest.ResponseRecorder{}),
	"net/url.URL":                        reflect.TypeOf(url.URL{}),
	"bytes.Buffer":                       reflect.TypeOf(bytes.Buffer{}),
	"strings.Reader":                     reflect.TypeOf(strings.Reader{}),
	"io.Reader":                          reflect.TypeOf((*io.Reader)(nil)).Elem(),
	"fmt.Stringer":                       reflect.TypeOf((*fmt.Stringer)(nil)).Elem(),
}

var nativeVars = map[string]reflect.Value{
	"os.Stdout": reflect.ValueOf(&os.Stdout),
}

var (
	anyType   = reflect.TypeOf((*any)(nil)).Elem()
	errorType = reflect.TypeOf((*error)(nil)).Elem()
)

func (m *Machine) nativeFunc(f *ssa.Function) reflect.Value {
	if f.Pkg == nil {
		unsupported("native call of package-less function %s (synthetic %q, origin %v)", f, f.Synthetic, f.Origin())
	}
	key := f.Pkg.Pkg.Path() + "." + f.Name()
	if rv, ok := m.fatalFunc(key); ok && f.Signature.Recv() == nil {
		return rv
	}
	if rv, ok := m.overrides[key]; ok && f.Signature.Recv() == nil {
		return rv
	}
	if f.Pkg.Pkg.Path() == "flag" && f.Signature.Recv() == nil {
		if rv, ok := m.flagFunc(f.Name()); ok {
			return rv
		}
	}
	if f.Signature.Recv() == nil {
		if rv, ok := m.hostFunc(key); ok {
			return rv
		}
	}
	if t := m.table(f.Pkg.Pkg.Path()); t != nil {
		if rv, ok := t.Funcs[f.Name()]; ok {
			return rv
		}
	}
	rv, ok := nativeFuncs[key]
	if !ok {
		rv, ok = m.stdSym(f.Pkg.Pkg, f.Name())
	}
	if !ok {
		rv, ok = m.syscallStub(f)
	}
	if !ok {
		rv, ok = m.xsysStub(f)
	}
	if !ok {
		unsupported("native func %s not in registry%s", key, m.whyMissing(f.Pkg.Pkg))
	}
	return rv
}

// syscallStub returns, for a restricted machine with the syscall subset and
// Options.StubAssembly, a stub of the function f of syscall that is outside
// the subset: the plugin loads, and a call panics with a *RefusedError
// without calling the function. gRPC on linux reaches
// syscall.SetsockoptInt (TCP_USER_TIMEOUT): with the stub it loads, but a
// gRPC server calls it for each connection, so the connection fails there.
func (m *Machine) syscallStub(f *ssa.Function) (reflect.Value, bool) {
	if !m.stubAsm || !m.unsafeSyscall || m.realStd || f.Pkg.Pkg.Path() != "syscall" || f.Signature.Recv() != nil {
		return reflect.Value{}, false
	}
	real, ok := stdsyms.Unrestricted["syscall/syscall"][f.Name()]
	if !ok || real.Kind() != reflect.Func {
		return reflect.Value{}, false
	}
	what := "syscall." + f.Name()
	return reflect.MakeFunc(real.Type(), func([]reflect.Value) []reflect.Value {
		panic(&RefusedError{What: what + " (outside the read-only syscall subset; Options.StubAssembly loaded the plugin)"})
	}), true
}

// whyMissing explains, when it can, why a function or variable of pkg has
// no native implementation.
func (m *Machine) whyMissing(pkg *types.Package) string {
	switch {
	case m.interpPkg(pkg):
		// A package run from source: the declaration has no body.
		return ": the function has no Go body (it is written in assembly), and the VM cannot run assembly" + stubAsmHint
	case pkg.Path() == "syscall" && m.unsafeSyscall && !m.realStd:
		return ": it is not in the read-only syscall subset of a plugin with Options.UseUnsafe (docs/SANDBOX.md); yaegi v2 gives the syscall symbols only with Options.Unrestricted and RealProcess" + stubAsmHint
	case pkg.Path() == "syscall" && !m.realStd:
		return ": yaegi v2 gives the syscall symbols only with Options.Unrestricted and RealProcess"
	case pkg.Path() == xsysUnixPath && m.unsafeSyscall && !m.realStd:
		return ": it is not in the read-only golang.org/x/sys/unix subset of a plugin with Options.UseUnsafe (docs/SANDBOX.md); yaegi v2 gives the whole package only with Options.Unrestricted and RealProcess" + stubAsmHint
	case pkg.Path() == xsysUnixPath && !m.realStd:
		return ": yaegi v2 gives golang.org/x/sys/unix only with Options.UseUnsafe (a read-only subset), or Options.Unrestricted and RealProcess"
	case pkg.Path() == "os/exec" && !m.realStd:
		return ": yaegi v2 gives the os/exec symbols only with Options.Exec, or Options.Unrestricted and RealProcess"
	}
	return ""
}

type nativeSite struct {
	once    sync.Once
	fast    fastFn
	mfn     atomic.Pointer[methodFunc] // method of the last receiver type
	fn      reflect.Value
	method  string
	params  []types.Type
	results []types.Type
	vari    bool
	args    []int32
	dst     decodeDst   // the argument a native decoder fills, if any
	prints  bool        // a native printer (printFuncs)
	stores  int         // the argument a native container only stores (storeFuncs), or -1
	deep    *nativeSite // native fmt for values nested too deep (deepprint.go)
	plain   bool        // native copies, no opaque handles (convGraph.plain)
}

func tupleTypes(t *types.Tuple) []types.Type {
	out := make([]types.Type, t.Len())
	for i := range out {
		out[i] = t.At(i).Type()
	}
	return out
}

func (m *Machine) nativeSite(f *ssa.Function, args []int32) *nativeSite {
	sig := f.Signature
	s := &nativeSite{args: args, results: tupleTypes(sig.Results()), vari: sig.Variadic(), dst: decodeDstOf(f), prints: printFuncs[f.String()], plain: plainFuncs[f.String()], stores: -1}
	if i, ok := storeFuncs[f.String()]; ok {
		s.stores = i
	}
	if recv := sig.Recv(); recv != nil {
		s.method = f.Name()
		s.params = append([]types.Type{recv.Type()}, tupleTypes(sig.Params())...)
	} else {
		s.fn = m.nativeFunc(f)
		s.params = tupleTypes(sig.Params())
		s.deep = m.deepPrinter(f, s)
	}
	return s
}

func callRV(fn reflect.Value, in []reflect.Value, variadic bool) []reflect.Value {
	if variadic {
		return fn.CallSlice(in)
	}
	return fn.Call(in)
}

func (m *Machine) pack(out []reflect.Value, ts []types.Type) Value {
	switch len(out) {
	case 0:
		return Value{}
	case 1:
		return m.fromNative(out[0], ts[0])
	}
	t := make([]Value, len(out))
	for i := range out {
		t[i] = m.fromNative(out[i], ts[i])
	}
	return Value{R: t}
}

func writeBack(wb []func()) {
	for _, f := range wb {
		f()
	}
}

func (m *Machine) callNativeSite(s *nativeSite, regs []Value) Value {
	if s.deep != nil && m.printTooDeep(s, regs) {
		return m.callNativeSlow(s.deep, regs)
	}
	if optFastNative {
		s.once.Do(func() { s.fast = m.fastSite(s) })
		// Method trampolines expect a native receiver, not an interpreted
		// representation of the native type (http.Header{} is a *MapV).
		if s.fast != nil && (s.method == "" || !isMapV(regs[s.args[0]])) {
			return s.fast(regs, s.args)
		}
	}
	return m.callNativeSlow(s, regs)
}

// callNativeSlow calls a native function through reflect, converting each
// argument.
func (m *Machine) callNativeSlow(s *nativeSite, regs []Value) Value {
	if len(s.args) <= maxDirectArgs {
		var buf [maxDirectArgs]reflect.Value
		if in, ok := m.directArgs(s, regs, buf[:len(s.args)]); ok {
			if s.method == "" {
				defer m.flushShadows()
				return m.pack(callRV(s.fn, in, s.vari), s.results)
			}
			if t := in[0].Type(); !readsChans(t, s.method) && in[0].CanInterface() {
				mf := s.mfn.Load()
				if mf == nil || mf.t != t {
					mf = m.lookupMethod(t, s.method)
					s.mfn.Store(mf)
				}
				if mf.fn.IsValid() {
					defer m.flushShadows()
					return m.pack(callRV(mf.fn, in, s.vari), s.results)
				}
			}
		}
	}
	var wb []func()
	c := newConv(&wb)
	c.g.prints, c.g.plain = s.prints, s.plain
	in := make([]reflect.Value, len(s.args))
	for i, a := range s.args {
		if i == s.dst.arg {
			c.dst = s.dst.kind
		}
		c.stores = i == s.stores
		in[i] = m.toNativeW(regs[a], s.params[i], c)
		c.dst, c.stores = decodeNone, false
		if i == 0 && s.method != "" && readsChans(in[0].Type(), s.method) {
			defer enterChanReader()() // before the data: see readsChans
		}
	}
	defer writeBack(wb)
	defer m.flushShadows()
	if s.method != "" {
		t := in[0].Type()
		mf := s.mfn.Load()
		if mf == nil || mf.t != t {
			mf = m.lookupMethod(t, s.method)
			s.mfn.Store(mf)
		}
		if mf.fn.IsValid() && in[0].CanInterface() {
			// mf.fn takes the receiver first: in lines up with its parameters.
			m.realReflectArgs(mf.fn.Type(), in)
			return m.pack(callRV(mf.fn, in, s.vari), s.results)
		}
		mv := in[0].MethodByName(s.method)
		m.realReflectArgs(mv.Type(), in[1:])
		return m.pack(callRV(mv, in[1:], s.vari), s.results)
	}
	if s.prints {
		m.topPrintArgs(in, s.vari)
	}
	m.realReflectArgs(s.fn.Type(), in)
	return m.pack(callRV(s.fn, in, s.vari), s.results)
}

// maxDirectArgs bounds the arguments of a native call made by directArgs.
const maxDirectArgs = 4

// directArgs fills in with the arguments of the native call s when each of
// them is native data that the call shares as it is: a native value that
// the VM holds (a reflect.Value of the type of the parameter), or a pointer
// to a native variable (new(runtime.MemStats), a field of type sync.Mutex).
// toNativeW gives the same values, but the call then needs no conversion
// state and no write-back, so it allocates nothing of its own:
// runtime.ReadMemStats(ms) in a measured loop counted 2 allocations per
// call (GOROOT test closure.go). ok is false when an argument needs the
// general conversion.
func (m *Machine) directArgs(s *nativeSite, regs []Value, in []reflect.Value) ([]reflect.Value, bool) {
	for i, a := range s.args {
		t := s.params[i]
		if m.twinNamed(t) != nil {
			return nil, false
		}
		v := regs[a]
		if rv, ok := v.R.(reflect.Value); ok {
			if rv.Kind() == reflect.Slice || !rv.IsValid() || rv.Type() != m.reflectType(t) {
				return nil, false
			}
			in[i] = rv
			continue
		}
		if _, ok := t.Underlying().(*types.Pointer); !ok {
			return nil, false
		}
		cell, ok := v.R.(*Value)
		if !ok || cell == nil {
			return nil, false
		}
		rv, ok := cell.R.(reflect.Value)
		if !ok || !rv.CanAddr() {
			return nil, false
		}
		in[i] = rv.Addr() // native variable living in native memory: share it, as toNativeW
	}
	return in, true
}

// methodFunc is the method of a native type as a function that takes the
// receiver first. Value.MethodByName builds the method type with
// reflect.FuncOf at each call: a sync.RWMutex method called in a loop
// spent most of its time there (GOROOT test fixedbugs/issue79186.go).
type methodFunc struct {
	t  reflect.Type
	fn reflect.Value // invalid: use Value.MethodByName (interface types)
	// replaced: fn is not the method of t but the replacement of a
	// restricted machine (Machine.lookupMethod).
	replaced bool
}

var methodFuncs sync.Map // methodKey -> *methodFunc

type methodKey struct {
	t    reflect.Type
	name string
}

func lookupMethod(t reflect.Type, name string) *methodFunc {
	k := methodKey{t, name}
	if mf, ok := methodFuncs.Load(k); ok {
		return mf.(*methodFunc)
	}
	mf := &methodFunc{t: t}
	if t.Kind() != reflect.Interface {
		if meth, ok := t.MethodByName(name); ok {
			mf.fn = meth.Func
		}
	}
	methodFuncs.Store(k, mf)
	return mf
}

func isMapV(v Value) bool { _, ok := v.R.(*MapV); return ok }

func (m *Machine) callNativeMethod(rv reflect.Value, name string, sig *types.Signature, regs []Value, args []int32) Value {
	ps := tupleTypes(sig.Params())
	in := make([]reflect.Value, len(args))
	var wb []func()
	c := newConv(&wb)
	if readsChans(rv.Type(), name) {
		defer enterChanReader()() // before the data: see readsChans
	}
	c.g.prints = printMethods[methodKey{rv.Type(), name}]
	dst := decodeDstOfMethod(rv.Type(), name)
	for i, a := range args {
		if i+1 == dst.arg { // dst.arg counts the receiver
			c.dst = dst.kind
		}
		in[i] = m.toNativeW(regs[a], ps[i], c)
		c.dst = decodeNone
	}
	defer writeBack(wb)
	defer m.flushShadows()
	if c.g.prints {
		m.topPrintArgs(in, sig.Variadic())
	}
	if mf := m.lookupMethod(rv.Type(), name); mf.fn.IsValid() && rv.CanInterface() {
		all := append([]reflect.Value{rv}, in...)
		m.realReflectArgs(mf.fn.Type(), all)
		return m.pack(callRV(mf.fn, all, sig.Variadic()), tupleTypes(sig.Results()))
	}
	mv := rv.MethodByName(name)
	m.realReflectArgs(mv.Type(), in)
	return m.pack(callRV(mv, in, sig.Variadic()), tupleTypes(sig.Results()))
}

// callBodyless invokes a native method on an interpreted representation of a
// native named type (e.g. http.HandlerFunc(closure).ServeHTTP).
func (m *Machine) callBodyless(fn *ssa.Function, recv Value, regs []Value, args []int32) Value {
	if isVMGeneric(fn) {
		return m.callGenericMethod(fn, recv, regs, args)
	}
	sig := fn.Signature
	// The receiver keeps its write-backs, as an argument: a pointer method
	// called through an interface (var u json.Unmarshaler = &raw) writes
	// a new slice header or new fields into the native copy, which must
	// reach the interpreted variable.
	var wb []func()
	rv := m.toNativeW(recv, sig.Recv().Type(), newConv(&wb))
	defer writeBack(wb)
	return m.callNativeMethod(rv, fn.Name(), sig, regs, args)
}

func (m *Machine) callNativeValue(fn reflect.Value, sig *types.Signature, regs []Value, args []int32) Value {
	if fn.Pointer() == goexitPtr {
		vmGoexit() // see goexitPtr
	}
	ps := tupleTypes(sig.Params())
	in := make([]reflect.Value, len(args))
	var wb []func()
	c := newConv(&wb)
	if isChanReaderFunc(fn) {
		defer enterChanReader()() // before the data: see readsChans
	}
	c.g.prints = printFuncPtrs[fn.Pointer()]
	dst := decodeDstOfValue(fn) // a decoder held in a field, a map...
	for i, a := range args {
		if i == dst.arg {
			c.dst = dst.kind
		}
		in[i] = m.toNativeW(regs[a], ps[i], c)
		c.dst = decodeNone
	}
	defer writeBack(wb)
	defer m.flushShadows()
	if c.g.prints {
		m.topPrintArgs(in, sig.Variadic())
	}
	m.realReflectArgs(fn.Type(), in)
	return m.pack(callRV(fn, in, sig.Variadic()), tupleTypes(sig.Results()))
}

// ---- type mapping ----

var basicRT = map[types.BasicKind]reflect.Type{
	types.Bool: reflect.TypeOf(false), types.Int: reflect.TypeOf(0), types.Int8: reflect.TypeOf(int8(0)),
	types.Int16: reflect.TypeOf(int16(0)), types.Int32: reflect.TypeOf(int32(0)), types.Int64: reflect.TypeOf(int64(0)),
	types.Uint: reflect.TypeOf(uint(0)), types.Uint8: reflect.TypeOf(uint8(0)), types.Uint16: reflect.TypeOf(uint16(0)),
	types.Uint32: reflect.TypeOf(uint32(0)), types.Uint64: reflect.TypeOf(uint64(0)), types.Uintptr: reflect.TypeOf(uintptr(0)),
	types.Float32: reflect.TypeOf(float32(0)), types.Float64: reflect.TypeOf(float64(0)), types.String: reflect.TypeOf(""),
	types.Complex64: reflect.TypeOf(complex64(0)), types.Complex128: reflect.TypeOf(complex128(0)),
	types.UntypedNil: anyType, types.UnsafePointer: reflect.TypeOf(unsafe.Pointer(nil)),
}

// unexportedNative finds the native type of an unexported type of a native
// package, which the symbol tables do not list (net.conn, embedded in
// net.TCPConn and net.UDPConn, whose promoted methods have it as receiver):
// it walks the exported types of the package in the table, their go/types
// and reflect structures side by side, until it meets the type.
//
// A type of an internal package has no table of its own (Go code outside
// the tree cannot import it): the exported types of the packages that
// import it are walked too. encoding/json/v2 and jsontext declare
// type Options = jsonopts.Options.
func (m *Machine) unexportedNative(target *types.TypeName) reflect.Type {
	if rt := m.nativeIn(target, target.Pkg()); rt != nil {
		return rt
	}
	if _, ok := internalRoot(target.Pkg().Path()); !ok {
		return nil
	}
	var users []*types.Package
	for _, p := range m.prog.AllPackages() {
		if slices.Contains(p.Pkg.Imports(), target.Pkg()) {
			users = append(users, p.Pkg)
		}
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Path() < users[j].Path() })
	for _, p := range users {
		if rt := m.nativeIn(target, p); rt != nil {
			return rt
		}
	}
	return nil
}

// nativeIn is unexportedNative for the exported types of pkg.
func (m *Machine) nativeIn(target *types.TypeName, pkg *types.Package) reflect.Type {
	syms := m.stdTable(pkg)
	names := make([]string, 0, len(syms))
	for name := range syms {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		v := syms[name]
		tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || v.Kind() != reflect.Pointer || !v.IsNil() {
			continue
		}
		if rt := findNative(target, tn.Type(), v.Type().Elem(), 0, map[types.Type]bool{}); rt != nil {
			return rt
		}
	}
	// The exported types of a package registered with RegisterNative reach its
	// unexported types (otel/trace.TraceState holds []member).
	if tab := m.table(pkg.Path()); tab != nil && pkg.Path() == target.Pkg().Path() {
		names = names[:0]
		for name := range tab.Types {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
			if !ok {
				continue
			}
			if rt := findNative(target, tn.Type(), tab.Types[name], 0, map[types.Type]bool{}); rt != nil {
				return rt
			}
		}
	}
	return nil
}

func findNative(target *types.TypeName, gt types.Type, rt reflect.Type, depth int, seen map[types.Type]bool) reflect.Type {
	if depth > 6 || seen[gt] {
		return nil
	}
	seen[gt] = true
	if n, ok := types.Unalias(gt).(*types.Named); ok && n.Obj() == target {
		return rt
	}
	switch u := gt.Underlying().(type) {
	case *types.Pointer:
		if rt.Kind() == reflect.Pointer {
			return findNative(target, u.Elem(), rt.Elem(), depth+1, seen)
		}
	case *types.Slice:
		if rt.Kind() == reflect.Slice {
			return findNative(target, u.Elem(), rt.Elem(), depth+1, seen)
		}
	case *types.Array:
		if rt.Kind() == reflect.Array {
			return findNative(target, u.Elem(), rt.Elem(), depth+1, seen)
		}
	case *types.Struct:
		if rt.Kind() != reflect.Struct || rt.NumField() != u.NumFields() {
			return nil
		}
		for i := 0; i < u.NumFields(); i++ {
			if r := findNative(target, u.Field(i).Type(), rt.Field(i).Type, depth+1, seen); r != nil {
				return r
			}
		}
	}
	return nil
}

// reflectType maps a go/types type to a native reflect.Type. For interpreted
// named types this is the "reflect wall": reflect cannot create named types
// with methods, so only the underlying type survives.
func (m *Machine) reflectType(t types.Type) reflect.Type {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() == nil && obj.Name() == "error" {
			return errorType
		}
		if r := m.twinNamed(t); r != nil {
			return r
		}
		// An instance of a generic type of a native package has no native
		// instance: it uses the VM layout, as in isNativeNamed.
		if obj.Pkg() != nil && !m.interpPkg(obj.Pkg()) && t.TypeArgs().Len() == 0 {
			key := obj.Pkg().Path() + "." + obj.Name()
			if tab := m.table(obj.Pkg().Path()); tab != nil {
				if rt, ok := tab.Types[obj.Name()]; ok {
					return rt
				}
			}
			rt, ok := nativeTypes[key]
			if !ok {
				if v, found := m.stdSym(obj.Pkg(), obj.Name()); found {
					rt, ok = v.Type().Elem(), true
				} else if v, found := m.learned.Load(key); found {
					rt, ok = v.(reflect.Type), true
				}
			}
			if !ok {
				if rt = m.unexportedNative(obj); rt != nil {
					m.learned.Store(key, rt)
					ok = true
				}
			}
			if !ok {
				unsupported("native type %s not in registry", key)
			}
			return rt
		}
		if m.selfRecursive(t) {
			return m.cutNamed(t)
		}
		return m.reflectType(t.Underlying())
	case *types.Basic:
		return basicRT[t.Kind()]
	case *types.Pointer:
		return reflect.PointerTo(m.reflectType(t.Elem()))
	case *types.Slice:
		return reflect.SliceOf(m.reflectType(t.Elem()))
	case *types.Array:
		return reflect.ArrayOf(int(t.Len()), m.reflectType(t.Elem()))
	case *types.Map:
		return reflect.MapOf(m.reflectType(t.Key()), m.reflectType(t.Elem()))
	case *types.Chan:
		return reflect.ChanOf(chanDir(t.Dir()), m.reflectType(t.Elem()))
	case *types.Interface:
		// reflect cannot build interface types with methods: native code sees
		// them as any (dynamic values are boxed in wrappers, see box).
		return anyType
	case *types.Signature:
		in := make([]reflect.Type, t.Params().Len())
		for i := range in {
			in[i] = m.reflectType(t.Params().At(i).Type())
		}
		out := make([]reflect.Type, t.Results().Len())
		for i := range out {
			out[i] = m.reflectType(t.Results().At(i).Type())
		}
		return reflect.FuncOf(in, out, t.Variadic())
	case *types.Struct:
		// reflect cannot build recursive types: in fields whose type refers
		// back to the struct, the references are cut (see cutType). Other
		// fields are built independently of this struct, so the native type of
		// a type does not depend on where it is built.
		cut := m.recursiveFields(t)
		fs := make([]reflect.StructField, t.NumFields())
		for i := range fs {
			f := t.Field(i)
			fs[i] = reflect.StructField{Name: f.Name(), Tag: reflect.StructTag(t.Tag(i)), Anonymous: f.Embedded()}
			if cut[i] {
				fs[i].Type = m.cutType(f.Type(), t)
				fs[i].Anonymous = fs[i].Anonymous && fs[i].Type.Kind() != reflect.Interface
			} else {
				fs[i].Type = m.reflectType(f.Type())
			}
			if !f.Exported() {
				fs[i].PkgPath = "main"
			}
			if fs[i].Anonymous && fs[i].Type.Kind() != reflect.Interface && fs[i].Type.NumMethod() > 0 {
				// reflect.StructOf promotes the methods of an embedded
				// field into method tables that the runtime cannot use
				// (Go issue 15924): the first type assertion of native
				// code to an interface (testing/quick's Generator on a
				// struct{ *big.Int }) ended the whole process with "fatal
				// error: runtime: type offset base pointer out of range".
				// Native code gets a plain field instead: it sees no
				// promoted method, never a broken one.
				fs[i].Anonymous = false
			}
		}
		return structOf(fs)
	}
	unsupported("reflectType %s", t)
	return nil
}

// structOf is reflect.StructOf for the native counterpart of an interpreted
// struct. reflect cannot build a struct with an embedded unexported field
// (it panics: "field is anonymous but has PkgPath set"): such a struct has
// no native counterpart, and structOf says so with unsupported. Then box
// (tryNative) gives native code an opaque handle, which sync.Pool,
// sync.Map or a context give back intact, and the other conversions fail
// with a clear error. Before, the panic of reflect reached the program
// (the Encoder of encoding/json/jsontext in Go 1.27 embeds its state).
func structOf(fs []reflect.StructField) reflect.Type {
	for _, f := range fs {
		if f.Anonymous && f.PkgPath != "" {
			unsupported("vm: no native type for a struct with the embedded unexported field %s", f.Name)
		}
	}
	return reflect.StructOf(fs)
}

// selfRecursive reports whether the interpreted named type n refers to
// itself without a struct on the way: type M map[string]M, type T []T,
// type A [1]*A, type F func() F, type C chan C, type P *P. reflect cannot
// build such a type, and reflectType would never end (the Go stack grew
// until gc killed the whole host). A cycle through a struct is cut by the
// struct (recursiveFields).
func (m *Machine) selfRecursive(n *types.Named) bool {
	if _, ok := n.Underlying().(*types.Struct); ok {
		return false
	}
	if c, ok := m.recNamed.Load(n); ok {
		return c.(bool)
	}
	r := m.reachesNamed(n.Underlying(), n, map[types.Type]bool{})
	m.recNamed.Store(n, r)
	return r
}

// reachesNamed reports whether the native type of t would contain the named
// type n without a struct on the way.
func (m *Machine) reachesNamed(t types.Type, n *types.Named, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if t == n {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.Named:
		if m.isNativeNamed(u) || m.twinNamed(u) != nil {
			return false
		}
		return m.reachesNamed(u.Underlying(), n, seen)
	case *types.Pointer:
		return m.reachesNamed(u.Elem(), n, seen)
	case *types.Slice:
		return m.reachesNamed(u.Elem(), n, seen)
	case *types.Array:
		return m.reachesNamed(u.Elem(), n, seen)
	case *types.Chan:
		return m.reachesNamed(u.Elem(), n, seen)
	case *types.Map:
		return m.reachesNamed(u.Key(), n, seen) || m.reachesNamed(u.Elem(), n, seen)
	case *types.Signature:
		for _, tu := range []*types.Tuple{u.Params(), u.Results()} {
			for i := 0; i < tu.Len(); i++ {
				if m.reachesNamed(tu.At(i).Type(), n, seen) {
					return true
				}
			}
		}
	}
	return false
}

// cutNamed is the native type of n, a selfRecursive type: each direct part
// of its underlying type (element, key, parameter, result) whose type refers
// back to n becomes any, which holds the native value of the part's static
// type. type M map[string]M becomes map[string]any, type T []T []any,
// type A [1]*A [1]any (the any holds a *[1]any), type F func() F
// func() any, type P *P *any. The conversions build the parts with their
// own native type and store them in the any (reflect.Value.Set accepts
// it); the conversions back take the value out of the any (unwrapCut).
func (m *Machine) cutNamed(n *types.Named) reflect.Type {
	part := func(t types.Type) reflect.Type {
		if m.reachesNamed(t, n, map[types.Type]bool{}) {
			return anyType
		}
		return m.reflectType(t)
	}
	switch u := n.Underlying().(type) {
	case *types.Pointer:
		return reflect.PointerTo(part(u.Elem()))
	case *types.Slice:
		return reflect.SliceOf(part(u.Elem()))
	case *types.Array:
		return reflect.ArrayOf(int(u.Len()), part(u.Elem()))
	case *types.Map:
		return reflect.MapOf(part(u.Key()), part(u.Elem()))
	case *types.Chan:
		return reflect.ChanOf(chanDir(u.Dir()), part(u.Elem()))
	case *types.Signature:
		in := make([]reflect.Type, u.Params().Len())
		for i := range in {
			pt := u.Params().At(i).Type()
			if u.Variadic() && i == len(in)-1 {
				// reflect.FuncOf needs a slice for the last parameter of
				// a variadic func: type V func(...V) V becomes
				// func(...any) any, not func(any) any.
				in[i] = reflect.SliceOf(part(pt.Underlying().(*types.Slice).Elem()))
				continue
			}
			in[i] = part(pt)
		}
		out := make([]reflect.Type, u.Results().Len())
		for i := range out {
			out[i] = part(u.Results().At(i).Type())
		}
		return reflect.FuncOf(in, out, u.Variadic())
	}
	unsupported("reflectType %s", n)
	return nil
}

// recursiveFields reports, for each field of st, whether its type refers
// back to st.
func (m *Machine) recursiveFields(st *types.Struct) []bool {
	if c, ok := m.recFields.Load(st); ok {
		return c.([]bool)
	}
	cut := make([]bool, st.NumFields())
	for i := range cut {
		cut[i] = m.reaches(st.Field(i).Type(), st, map[types.Type]bool{})
	}
	m.recFields.Store(st, cut)
	return cut
}

// cutType is the native type of t, a type referring to the struct st, with
// pointers and structs referring to st replaced by any (holding the native
// value of their static type): []*T becomes []any, map[string]T
// map[string]any, *T any. Slices and maps stay slices and maps, so nil
// ones print as [] and map[], and omitempty works.
func (m *Machine) cutType(t types.Type, st *types.Struct) reflect.Type {
	if !m.reaches(t, st, map[types.Type]bool{}) {
		return m.reflectType(t)
	}
	switch u := types.Unalias(t).(type) {
	case *types.Named:
		if _, ok := u.Underlying().(*types.Struct); ok {
			return anyType
		}
		if !m.isNativeNamed(u) && m.selfRecursive(u) {
			return anyType // cutType(u.Underlying()) would never end
		}
		return m.cutType(u.Underlying(), st)
	case *types.Slice:
		return reflect.SliceOf(m.cutType(u.Elem(), st))
	case *types.Array:
		return reflect.ArrayOf(int(u.Len()), m.cutType(u.Elem(), st))
	case *types.Map:
		return reflect.MapOf(m.cutType(u.Key(), st), m.cutType(u.Elem(), st))
	case *types.Chan:
		return reflect.ChanOf(reflect.BothDir, m.cutType(u.Elem(), st))
	case *types.Signature:
		in := make([]reflect.Type, u.Params().Len())
		for i := range in {
			in[i] = m.cutType(u.Params().At(i).Type(), st)
		}
		out := make([]reflect.Type, u.Results().Len())
		for i := range out {
			out[i] = m.cutType(u.Results().At(i).Type(), st)
		}
		return reflect.FuncOf(in, out, u.Variadic())
	}
	return anyType // pointers, structs
}

// toNativeAs converts v, of type t, to the native type rt, a cut type of t
// (see cutType).
func (m *Machine) toNativeAs(v Value, t types.Type, rt reflect.Type, c *natConv) reflect.Value {
	if rv, ok := v.R.(reflect.Value); ok {
		if rv.Type().AssignableTo(rt) {
			return rv
		}
		if rv.Kind() == reflect.Slice && rv.Type().ConvertibleTo(rt) {
			return rv.Convert(rt) // a native slice view (natslice.go)
		}
	}
	if rt.Kind() == reflect.Interface && !types.IsInterface(t) {
		switch t.Underlying().(type) {
		case *types.Slice, *types.Map, *types.Array, *types.Signature, *types.Chan:
			// a cut of a selfRecursive type (cutNamed): the any holds the
			// native value of t, a nil slice or map included
			return m.toNativeW(v, t, c)
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		// scalars carry no R: the check of v.R below is for pointers
		r := m.toNativeW(v, t, c)
		if r.Type() != rt && r.Type().ConvertibleTo(rt) {
			r = r.Convert(rt)
		}
		return r
	case *types.Interface:
		return m.toNativeW(v, t, c)
	case *types.Slice:
		if z, ok := v.R.(*zslice); ok {
			return z.native(rt)
		}
		s, _ := v.R.([]Value)
		if s == nil {
			return reflect.Zero(rt)
		}
		c.g.use(len(s)) // before the native slice is allocated
		out := reflect.MakeSlice(rt, len(s), len(s))
		if len(s) > 0 {
			if o, ok := c.g.enterComp(compKey{unsafe.Pointer(&s[0]), len(s), rt}, out); !ok {
				return o
			}
		}
		for i := range s {
			out.Index(i).Set(m.toNativeAs(s[i], u.Elem(), rt.Elem(), c))
		}
		return out
	case *types.Array:
		out := reflect.New(rt).Elem()
		es, _ := v.R.([]Value)
		for i := range es {
			out.Index(i).Set(m.toNativeAs(es[i], u.Elem(), rt.Elem(), c))
		}
		return out
	case *types.Map:
		mp, _ := v.R.(*MapV)
		if mp == nil {
			return reflect.Zero(rt)
		}
		c.g.use(len(mp.M))
		out := reflect.MakeMapWithSize(rt, len(mp.M))
		if o, ok := c.g.enterComp(compKey{unsafe.Pointer(mp), 0, rt}, out); !ok {
			return o
		}
		mp.each(func(k, val Value) {
			out.SetMapIndex(m.toNativeAs(k, u.Key(), rt.Key(), c), m.toNativeAs(val, u.Elem(), rt.Elem(), c))
		})
		return out
	case *types.Signature:
		cl, _ := v.R.(*Closure)
		if cl == nil {
			return reflect.Zero(rt)
		}
		origin := m.originAcct()
		return reflect.MakeFunc(rt, func(in []reflect.Value) []reflect.Value {
			defer m.nativeCallback(origin)()
			args := make([]Value, len(in))
			for i := range in {
				args[i] = m.fromNative(in[i], u.Params().At(i).Type())
			}
			res := m.callClosure(cl, args, u)
			if u.Results().Len() == 1 {
				res = Value{R: []Value{res}}
			}
			out := make([]reflect.Value, u.Results().Len())
			for i := range out {
				out[i] = m.toNativeAs(res.R.([]Value)[i], u.Results().At(i).Type(), rt.Out(i), newConv(nil))
			}
			return out
		})
	}
	if v.R == nil {
		return reflect.Zero(rt) // nil pointer or chan (a nil any, for omitempty)
	}
	return m.toNativeW(v, t, c)
}

// reaches reports whether the native type of t would contain the struct st.
func (m *Machine) reaches(t types.Type, st *types.Struct, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.Named:
		if m.isNativeNamed(u) {
			return false
		}
		return m.reaches(u.Underlying(), st, seen)
	case *types.Struct:
		if u == st {
			return true
		}
		for i := 0; i < u.NumFields(); i++ {
			if m.reaches(u.Field(i).Type(), st, seen) {
				return true
			}
		}
	case *types.Pointer:
		return m.reaches(u.Elem(), st, seen)
	case *types.Slice:
		return m.reaches(u.Elem(), st, seen)
	case *types.Array:
		return m.reaches(u.Elem(), st, seen)
	case *types.Chan:
		return m.reaches(u.Elem(), st, seen)
	case *types.Map:
		return m.reaches(u.Key(), st, seen) || m.reaches(u.Elem(), st, seen)
	case *types.Signature:
		for _, tu := range []*types.Tuple{u.Params(), u.Results()} {
			for i := 0; i < tu.Len(); i++ {
				if m.reaches(tu.At(i).Type(), st, seen) {
					return true
				}
			}
		}
	}
	return false
}

// tryReflectType is reflectType returning nil when t has no native equivalent.
func (m *Machine) tryReflectType(t types.Type) (rt reflect.Type) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*CompileError); !ok {
				panic(r)
			}
			rt = nil
		}
	}()
	return m.reflectType(t)
}

// mentionsInterpNamed reports whether t refers to a named type declared in
// interpreted code (such types lose their identity in native code).
func (m *Machine) mentionsInterpNamed(t types.Type) bool {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		return t.Obj().Pkg() != nil && m.interpPkg(t.Obj().Pkg())
	case *types.Pointer:
		return m.mentionsInterpNamed(t.Elem())
	case *types.Slice:
		return m.mentionsInterpNamed(t.Elem())
	case *types.Array:
		return m.mentionsInterpNamed(t.Elem())
	case *types.Chan:
		return m.mentionsInterpNamed(t.Elem())
	case *types.Map:
		return m.mentionsInterpNamed(t.Key()) || m.mentionsInterpNamed(t.Elem())
	case *types.Signature:
		for _, tu := range []*types.Tuple{t.Params(), t.Results()} {
			for i := 0; i < tu.Len(); i++ {
				if m.mentionsInterpNamed(tu.At(i).Type()) {
					return true
				}
			}
		}
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if m.mentionsInterpNamed(t.Field(i).Type()) {
				return true
			}
		}
	}
	return false
}

// ---- boundary conversions ----

func (m *Machine) toNative(v Value, t types.Type) reflect.Value {
	return m.toNativeW(v, t, newConv(nil))
}

// setNative sets the native place dst to rv, the native value of an
// interpreted value. A plugin logger that goes into a place of type
// *log.Logger (httputil.ReverseProxy.ErrorLog) becomes the native logger it
// wraps: the standard library of plugins makes log.Logger a wrapper type
// (stdsyms logLogger), whose Fatal methods panic. fromNativeS gives the
// plugin logger back.
func setNative(dst, rv reflect.Value) {
	// The type of the place first: the common store looks at one type.
	switch dt := dst.Type(); {
	case dt == stdsyms.LoggerType && rv.Type() != dt:
		rv, _ = stdsyms.NativeLogger(rv)
	case dt == handlerType && rv.IsNil() && dst.CanAddr():
		// nil in the Handler of a running server of a plugin: the plugin's
		// DefaultServeMux, not the host's (hostregs.go)
		if h := lateNilHandler(dst.Addr().UnsafePointer()); h != nil {
			rv = reflect.ValueOf(&h).Elem()
		}
	case dt == serverType && dst.CanAddr():
		// The whole struct of a running server: the same for its Handler.
		rv = lateNilServer(dst, rv)
	case dt == reflectTypeT && rv.IsValid() && rv.Type() == rtNative:
		// A reflect.Type of the program (the twin's *vtype) in a field of
		// type reflect.Type: json.UnsupportedTypeError{Type: reflect.TypeOf(x)}.
		t := nativeType(rv.Interface().(*vtype))
		rv = reflect.ValueOf(&t).Elem()
	case dt == structFieldNative && rv.IsValid() && rv.Type() == reflectStructFieldT:
		// A native reflect.StructField in a field of the AOT copy, whose
		// reflect is the twin's: json.UnmarshalFieldError{Field: ...} with -aot.
		sf := rv.Interface().(reflect.StructField)
		f := natField(sf)
		f.Type = twinType(sf.Type) // *Inner, not *struct { X int }
		rv = reflect.ValueOf(f)
	case rv.IsValid() && rv.Type() != dt && aotTwinTypes(rv.Type(), dt):
		// A value of the AOT copy of a package in a place of the native
		// package, or the reverse: an asn1.ObjectIdentifier of the copy of
		// encoding/asn1 in the field of a native pkix.AttributeTypeAndValue,
		// or a []asn1.ObjectIdentifier in x509.Certificate.PolicyIdentifiers.
		if cv, ok := convertTwin(rv, dt); ok {
			rv = cv
		}
	}
	dst.Set(rv)
}

// aotTwinTypes reports whether a and b are the same named type of a
// package and of its AOT copy, or a slice, an array or a map of such types.
func aotTwinTypes(a, b reflect.Type) bool {
	if a.Name() == "" && b.Name() == "" && a.Kind() == b.Kind() {
		switch a.Kind() {
		case reflect.Slice:
			return twinOrSame(a.Elem(), b.Elem())
		case reflect.Array:
			return a.Len() == b.Len() && twinOrSame(a.Elem(), b.Elem())
		case reflect.Map:
			return twinOrSame(a.Key(), b.Key()) && twinOrSame(a.Elem(), b.Elem()) && a != b
		}
	}
	if a.Name() == "" || a.Name() != b.Name() {
		return false
	}
	pa, pb := a.PkgPath(), b.PkgPath()
	if strings.HasPrefix(pb, aotModPrefix) {
		pa, pb = pb, pa
	}
	return strings.HasPrefix(pa, aotModPrefix) && origPath(pa) == pb
}

func twinOrSame(a, b reflect.Type) bool { return a == b || aotTwinTypes(a, b) }

// convertTwin converts rv to dt, its twin type of the native package or of
// the AOT copy (aotTwinTypes). A slice, an array or a map whose elements are
// twin types is converted element by element, into a new value. ok is
// false when rv cannot be converted (a struct with fields of twin types).
func convertTwin(rv reflect.Value, dt reflect.Type) (reflect.Value, bool) {
	st := rv.Type()
	if st == dt {
		return rv, true
	}
	if st.ConvertibleTo(dt) {
		return rv.Convert(dt), true
	}
	if st.Kind() != dt.Kind() {
		return rv, false
	}
	switch st.Kind() {
	case reflect.Slice:
		if rv.IsNil() {
			return reflect.Zero(dt), true
		}
		out := reflect.MakeSlice(dt, rv.Len(), rv.Len())
		for i := range rv.Len() {
			e, ok := convertTwin(rv.Index(i), dt.Elem())
			if !ok {
				return rv, false
			}
			out.Index(i).Set(e)
		}
		return out, true
	case reflect.Array:
		out := reflect.New(dt).Elem()
		for i := range rv.Len() {
			e, ok := convertTwin(rv.Index(i), dt.Elem())
			if !ok {
				return rv, false
			}
			out.Index(i).Set(e)
		}
		return out, true
	case reflect.Map:
		if rv.IsNil() {
			return reflect.Zero(dt), true
		}
		out := reflect.MakeMapWithSize(dt, rv.Len())
		for it := rv.MapRange(); it.Next(); {
			k, ok := convertTwin(it.Key(), dt.Key())
			v, ok2 := convertTwin(it.Value(), dt.Elem())
			if !ok || !ok2 {
				return rv, false
			}
			out.SetMapIndex(k, v)
		}
		return out, true
	}
	return rv, false
}

// natConv is the state of the conversions of the arguments of one native
// call: the write-backs to run after the call (nil: none), and the
// interpreted pointers converted so far, shared by nested conversions.
type natConv struct {
	wb    *[]func()
	g     *convGraph
	inAny bool // converting the dynamic value of an interface (see box)
	// inBox: converting a value that native code gets in an interface, or
	// a part of it (see boxCopy): an interpreted channel there becomes a
	// native stand-in (chanstandin.go).
	inBox bool
	// track: the maps and slices met by this conversion keep their identity
	// (see mapCopy and sliceCopy). It is set for the arguments of a call
	// with write-backs and for what their pointers reach, not for the
	// values that native code gets by value in an interface (fmt.Println,
	// json.Marshal), which it only reads.
	track bool
	// dst: the value being converted is the pointer that a native decoder
	// fills (json.Unmarshal(data, v)), see decodeDsts. box uses it once;
	// on a slice (the ...any of fmt.Sscan), each element gets it.
	dst decodeKind
	// stores: the value being converted is the argument that a native
	// container only stores and gives back (storeFuncs): box hands out an
	// opaque handle, as with OpaqueAny. Used once, like dst.
	stores bool
	own    convGraph
}

// sharedKey is a native pointer: its address and its type (a pointer to
// the first field of a struct has the address of the struct).
type sharedKey struct {
	addr uintptr
	t    reflect.Type
}

type convGraph struct {
	shadows map[*Value]reflect.Value // interpreted cell -> native shadow
	cells   map[uintptr]*Value       // shadow address -> interpreted cell
	back    map[uintptr]*Value       // write-back: native address -> cell
	// shared: the cells whose value lives in native memory (a native
	// struct made by the program), by the native pointer that the
	// conversion gave for them. That pointer converts back to the cell, so
	// the program keeps the identity of its pointer.
	shared map[sharedKey]*Value
	// unrolled shadows (toNativeU): one cell can have one shadow per
	// unrolled type, so they are not in shadows.
	ushadows map[ushadowKey]reflect.Value
	// open holds the maps and slices converted, or being converted (see
	// enterComp).
	open map[compKey]reflect.Value
	// nopen holds the native maps and slices that fromNativeDeepC has
	// converted back to VM values, or is converting (see natSeen).
	nopen natSeen

	wb      *[]func()                    // write-backs of the call (nil: none)
	maps    map[*MapV]*mapCopy           // interpreted map -> its native copy
	nmaps   map[uintptr]*mapCopy         // native copy address -> record
	slices  map[uintptr]*sliceCopy       // address of the native copy's array -> record
	funcs   map[unsafe.Pointer]*funcCopy // funcWord of a func made for a closure -> record
	ptrs    map[uintptr]*ptrCopy         // shadow address -> pointer boxed in an interface
	snaps   map[uintptr]reflect.Value    // shadow address -> its content after the conversion (shadowSame)
	flushOn bool                         // flushCopies is in the write-backs
	// prints: the call is a native printer (printFuncs), which only reads
	// its `any` arguments: box gives no opaque handle for a value without
	// methods, so that %T sees its type.
	prints bool
	// depth counts the boxCopy calls in progress: a value nested in
	// interfaces ([]any{[]any{...}}). The copy recurses on the host stack,
	// so over maxBoxDepth box gives an opaque handle (see boxCopy).
	depth int
	// seg is the number of maxBoxDepth bounds above the converted value:
	// the conversion of a handle of the bound continues its count (see
	// deepSegs).
	seg int
	// plain: box gives native copies, not opaque handles, also with
	// OpaqueAny: native fmt walks a value nested too deep in one copy per
	// maxBoxDepth bound, not one handle (and one Format call) per level
	// (deepprint.go).
	plain bool
	// aot: the call goes to an AOT copy in default mode (aotroute.go). box
	// gives opaque handles, as with OpaqueAny, at every level, and a value
	// of a native type that holds interfaces ([]any, map[string]any) is a
	// native copy whose interfaces hold handles, as in AOT mode: a handle
	// of it was unboxed by the copy into a native copy of its elements,
	// which lost the names of the program's types.
	aot bool
	// left is the number of values that the native copy for a printer may
	// still convert, when bounded is set (see printCopyMax).
	left    int
	bounded bool
}

// maxBoxDepth bounds the nesting of interfaces that box copies for native
// code. A []any nested one million times killed the host with a fatal Go
// stack overflow, which tryNative cannot recover (fmt.Printf("%T", v) does
// not walk v in Go). Deeper values get an opaque handle. The bound is high:
// values up to it are copied as before. The twin converts a handle of the
// bound again in one native copy (fromBoxed).
//
// The host stack of a goroutine is at most 1 GB on 64-bit platforms and
// 250 MB on 32-bit ones (runtime/debug.SetMaxStack): there, one copy of
// 100000 levels killed the host with a fatal stack overflow. The bound is
// four times smaller on 32-bit (see deepSegs for the levels that native
// code may walk through).
const maxBoxDepth = 25000 * (1 + 3*(strconv.IntSize/64))

// printCopyMax bounds the native copy that a printer gets with OpaqueAny
// (convGraph.prints): the number of values converted, and the entries of the
// graph cloned by tryNativeConv. A larger value gets the opaque handle, as
// without the printer case: %v is right (Format), %T prints vm.vmOpaque. It
// keeps the cost of fmt.Printf("%T", make([]int, 1e7)) small: Go does not
// read the elements for %T, and the copy is not charged to the budget.
const printCopyMax = 1 << 16

// use counts n values converted by a bounded copy. Over the bound, it
// makes the conversion fail: tryNativeConv in boxCopy then gives the
// opaque handle. Outside boxCopy (depth 0) nothing would recover it, so
// it only counts there.
func (g *convGraph) use(n int) {
	if g.bounded && g.depth > 0 {
		if g.left -= n; g.left < 0 {
			unsupported("value too large for the native copy of a printer")
		}
	}
}

// ptrCopy is the native shadow of an interpreted pointer that was converted
// for the interface value iv, in a call with write-backs. An interface
// reached through a pointer (json.Unmarshal(data, &v) with v of type any)
// has no write-back of its own: when native code keeps the pointer in the
// interface (encoding/json, xml and gob decode into the pointed value), the
// write-back of the outer value gives back iv and writes the shadow into
// the interpreted cell (boxedCopy). Without it, the interface got a new
// pointer to the old content of the cell (default mode), or a new pointer
// to the cell content after the call (-aot, an AOT shadow of the cell).
type ptrCopy struct {
	iv     *Iface
	t      types.Type   // the pointer type of the conversion
	rt     reflect.Type // the native type of the pointer
	shared bool         // the pointer is native memory of the cell: nothing to write
}

// funcCopy is a native func made by reflect.MakeFunc for an interpreted
// func v, for a native call with write-backs. The write-backs give back v
// (and iv, when the func was converted for the interface value iv): a read
// only call does not replace the func (fromNativeDeepC, boxedCopy).
type funcCopy struct {
	v  Value
	iv *Iface
}

// closureFunc is the interpreted closure behind a native func made by
// reflect.MakeFunc (toNativeW), with its signature.
type closureFunc struct {
	cl  *Closure
	sig *types.Signature
}

// noteClosureFunc records that the native func fn calls the interpreted
// closure cl of signature sig. When fn comes back to the VM, even after the
// native call that made it (a struct with a func field stored in an
// atomic.Value, as the produceHolder of the OpenTelemetry metric SDK), it is
// cl again (closureOfFunc), as in Go: the arguments of a call are not
// converted to native values and back, and the writes through a pointer
// argument reach the caller. The key is a weak pointer to the func, as for
// chanReaderFuncs: the entry goes when the func is freed.
func (m *Machine) noteClosureFunc(fn reflect.Value, cl *Closure, sig *types.Signature) {
	p, ok := funcWord(fn)
	if !ok || p == nil {
		return
	}
	k := weak.Make((*byte)(p))
	funcs := m.closureFuncs
	funcs.Store(k, closureFunc{cl: cl, sig: sig})
	runtime.AddCleanup((*byte)(p), func(k weak.Pointer[byte]) { funcs.Delete(k) }, k)
}

// closureOfFunc returns the interpreted closure behind the native func rv
// (noteClosureFunc), when the func type t has its signature; nil otherwise.
func (m *Machine) closureOfFunc(rv reflect.Value, t types.Type) *Closure {
	if rv.Kind() != reflect.Func || rv.IsNil() {
		return nil
	}
	if pc := rv.Pointer(); pc != makeFuncPC && !callbackPCs[pc] {
		return nil
	}
	sig, ok := t.Underlying().(*types.Signature)
	if !ok {
		return nil
	}
	p, ok := funcWord(rv)
	if !ok || p == nil {
		return nil
	}
	r, ok := m.closureFuncs.Load(weak.Make((*byte)(p)))
	if !ok {
		return nil
	}
	cf := r.(closureFunc)
	if !types.Identical(cf.sig, sig) {
		return nil // another func type with the same native type: keep the native func
	}
	return cf.cl
}

// mapCopy is the native copy of an interpreted map, made for a native
// call. In Go a map is a reference: native code that fills the copy
// (json.Unmarshal into a non nil map, url.Values.Add...) must fill the
// interpreted map itself, and every interpreted alias must see it. After
// the call, the content of the copy is written back into mp (mapBack), and
// the native copy converts back to mp (fromNativeDeepC), never to a new
// map.
type mapCopy struct {
	mp   *MapV
	out  reflect.Value
	t    *types.Map
	orig map[any]origEntry // key of mp.M -> value converted in
	refs map[any]Value     // refKey of the native values -> interpreted value (moved values)
	done bool
	iv   *Iface // the interface value the map was converted for, if any
}

// origEntry is an entry of an interpreted map, with the native value that
// it was converted to.
type origEntry struct {
	nat reflect.Value
	v   Value
}

// sliceCopy is the native copy of an interpreted slice, made for a native
// call. The copy has the capacity of the interpreted slice: native code
// that writes into the elements or appends in place (json.Unmarshal into
// s[:0], sort.Strings, io.Reader.Read) writes into the interpreted array.
type sliceCopy struct {
	full []Value       // the interpreted array: s[:cap(s)]
	n    int           // len(s): the elements converted in
	out  reflect.Value // the native copy, of length cap(s)
	et   types.Type
	keys map[any]int // refKey of the native elements -> index (moved elements)
	orig []Value
	nat  []reflect.Value // the native elements converted in
	done bool
	ext  int    // elements after n already written back from a longer view
	tail bool   // the elements after n (basic type) are in out and written back
	iv   *Iface // the interface value the slice was converted for, if any
}

// compKey names an interpreted map or slice being converted to the native
// type t: the map, or the first element and the length of the slice.
type compKey struct {
	p unsafe.Pointer
	n int
	t reflect.Type
}

// enterComp starts the conversion of the map or slice k to out. When k was
// met before by this conversion, enterComp returns its native value and ok
// is false: in the middle of its conversion, the value contains itself
// (m["self"] = m, or s[0] = s through an interface), and the native copy
// gets the same cycle (without this, the conversion never ended and the Go
// stack grew until gc killed the whole host); after its conversion, the
// value is shared (a DAG: the same map or slice reached by several paths),
// and the native copy shares it too, as in Go. Each map and slice is so
// converted once: the conversion was exponential in the depth of a DAG.
// Otherwise the caller fills out.
func (g *convGraph) enterComp(k compKey, out reflect.Value) (reflect.Value, bool) {
	if o, ok := g.open[k]; ok {
		return o, false
	}
	if g.open == nil {
		g.open = map[compKey]reflect.Value{}
	}
	g.open[k] = out
	return out, true
}

func newConv(wb *[]func()) *natConv {
	c := &natConv{wb: wb, track: wb != nil}
	c.g = &c.own
	c.g.wb = wb
	return c
}

// nested returns the state for values nested in a converted value, which
// have no write-back of their own.
func (c *natConv) nested() *natConv {
	if c.wb == nil {
		return c
	}
	return &natConv{g: c.g, track: c.track, inBox: c.inBox}
}

// boxed returns c for the value of an interface (see natConv.inBox).
func (c *natConv) boxed() *natConv {
	if c.inBox {
		return c
	}
	return &natConv{wb: c.wb, g: c.g, inAny: c.inAny, inBox: true, track: c.track, dst: c.dst}
}

// keeps reports whether the maps and slices of this conversion keep their
// identity across the native call.
func (c *natConv) keeps() bool { return c.track && c.g.wb != nil }

// queueFlush makes sure the copies of the graph are written back after the
// call (flushCopies).
func (m *Machine) queueFlush(c *natConv) {
	if c.g.flushOn {
		return
	}
	c.g.flushOn = true
	g := c.g
	*g.wb = append(*g.wb, func() { m.flushCopies(&natConv{g: g}) })
}

// flushCopies writes back every map and slice copy of the call that no
// pointer write-back reached: native code may have changed a map that it
// got inside a struct or a slice.
func (m *Machine) flushCopies(c *natConv) {
	for _, r := range c.g.nmaps {
		m.mapBack(r, c)
	}
	for _, r := range c.g.slices {
		m.sliceBack(r, c)
	}
}

// reopen makes the graph ready to write back again: a shadow that keeps its
// native value across a callback (resumeShadows) is detached again later,
// and native code may have changed its maps and slices since the last
// write-back. The copies stay recorded, so they still convert back to the
// same interpreted maps and arrays.
func (g *convGraph) reopen() {
	for _, r := range g.nmaps {
		r.done = false
	}
	for _, r := range g.slices {
		r.done, r.ext = false, 0
	}
	g.nopen = natSeen{}
}

// unexportedIfaceError stops with a clear error when the value iv of the
// program cannot be a value of the native interface rt: rt has unexported
// methods (ast.Expr's exprNode, promoted from an embedded *ast.Ident), and
// Go cannot make a native type with them at run time. The wrapper of the
// VM made reflect.Set panic with "value of type vm.stringerW is not
// assignable to type ast.Expr" (docs/ARCHITECTURE.md).
func unexportedIfaceError(iv *Iface, rt reflect.Type) {
	for i := 0; i < rt.NumMethod(); i++ {
		if mt := rt.Method(i); !mt.IsExported() {
			unsupported("a value of type %s cannot be given to native code as %s: the interface has the unexported method %s, which a type of the program cannot have natively",
				typeString(iv.T.T), rt, mt.Name)
		}
	}
}

// toNativeW converts an interpreted value for native code. Interpreted memory
// (pointers) cannot be shared with native code: a native shadow is allocated,
// filled, and copied back after the call when c.wb != nil (the "copy-in /
// copy-out" hack; breaks if native code retains the pointer). Cycles of
// pointers map to cycles of shadows.
func (m *Machine) toNativeW(v Value, t types.Type, c *natConv) reflect.Value {
	if rt := m.twinNamed(t); rt != nil {
		if r, ok := m.twinFromVM(v, rt); ok {
			return r
		}
	}
	wb := c.wb
	c.g.use(1)
	if rv, ok := v.R.(reflect.Value); ok {
		if rv.Kind() == reflect.Slice {
			// a native slice view: the native slice itself, converted
			// when the static type is another slice type with the same
			// elements (sort.StringSlice(vs), a named type of the plugin)
			if rt := m.reflectType(t); rv.Type() != rt && rv.Type().ConvertibleTo(rt) {
				return rv.Convert(rt)
			}
		}
		return rv
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		rt := m.reflectType(t)
		out := reflect.New(rt).Elem()
		switch {
		case u.Kind() == types.UnsafePointer:
			out.SetPointer(ptrOf(v))
		case u.Info()&types.IsBoolean != 0:
			out.SetBool(v.N != 0)
		case u.Info()&types.IsString != 0:
			s, _ := v.R.(string)
			out.SetString(s)
		case u.Info()&types.IsComplex != 0:
			out.SetComplex(cplx(v))
		case u.Info()&types.IsFloat != 0:
			out.SetFloat(math.Float64frombits(uint64(v.N)))
		case u.Info()&types.IsUnsigned != 0:
			out.SetUint(uint64(v.N))
		default:
			out.SetInt(v.N)
		}
		return out
	case *types.Interface:
		rt := m.reflectType(t)
		iv, _ := v.R.(*Iface)
		if iv == nil {
			return reflect.Zero(rt)
		}
		var nv reflect.Value
		if iv.T.Native != nil {
			nv = iv.V.R.(reflect.Value)
		} else if ow, ok := m.optWrapper(iv, rt); ok {
			nv = ow
		} else if w := m.ifaceWrapper(iv, rt); w != nil {
			if rw, ok := m.rwWrapper(iv, w, rt); ok {
				nv = rw
			} else {
				nv = m.wrap(iv, w)
			}
		} else {
			nv = m.boxFor(iv, rt, c)
		}
		if nv.IsValid() && !nv.Type().AssignableTo(rt) {
			unexportedIfaceError(iv, rt)
		}
		out := reflect.New(rt).Elem()
		out.Set(nv)
		return out
	case *types.Slice:
		rt := m.reflectType(t)
		if b, ok := v.R.([]byte); ok {
			rv := reflect.ValueOf(b)
			if rv.Type() != rt {
				rv = rv.Convert(rt)
			}
			return rv
		}
		if z, ok := v.R.(*zslice); ok {
			return z.native(rt)
		}
		s, _ := v.R.([]Value)
		if s == nil {
			c.dst = decodeNone
			return reflect.Zero(rt)
		}
		c.g.use(len(s)) // before the native slice is allocated
		elem := func(v Value) reflect.Value { return m.toNativeW(v, u.Elem(), c) }
		if dst := c.dst; dst != decodeNone {
			// The variadic ...any of a native function that fills each of
			// its arguments (fmt.Sscan(str, &a, &b), see decodeDsts): every
			// element is a destination.
			c.dst = decodeNone
			elem = func(v Value) reflect.Value {
				c.dst = dst
				defer func() { c.dst = decodeNone }()
				return m.toNativeW(v, u.Elem(), c)
			}
		}
		// COPY: aliasing between interpreted and native memory is lost, except
		// for in-place updates during the call, copied back (sort.Strings,
		// binary.PutUint64, io.Reader.Read, json.Unmarshal into s[:0]...).
		if wb == nil && !c.keeps() {
			out := reflect.MakeSlice(rt, len(s), len(s))
			if len(s) > 0 {
				if o, ok := c.g.enterComp(compKey{unsafe.Pointer(&s[0]), len(s), rt}, out); !ok {
					return o
				}
			}
			for i := range s {
				out.Index(i).Set(elem(s[i]))
			}
			return out
		}
		return m.sliceCopyOf(s, rt, u.Elem(), c, elem)
	case *types.Signature:
		rt := m.reflectType(t)
		cl, _ := v.R.(*Closure)
		if cl == nil {
			return reflect.Zero(rt)
		}
		origin := m.originAcct()
		fn, ok := m.fastCallback(cl, u, rt, origin) // callbacks.go
		if !ok {
			fn = reflect.MakeFunc(rt, func(in []reflect.Value) []reflect.Value {
				defer m.nativeCallback(origin)() // a callback on a goroutine of native code
				args := make([]Value, len(in))
				for i := range in {
					args[i] = m.fromNative(in[i], u.Params().At(i).Type())
				}
				res := m.callClosure(cl, args, u)
				return m.unpack(res, u.Results(), rt)
			})
		}
		m.noteClosureFunc(fn, cl, u)
		if c.g.wb != nil {
			// The write-backs of the call give back v for this func
			// (fromNativeDeepC): a read only call does not replace it.
			if w, ok := funcWord(fn); ok {
				if c.g.funcs == nil {
					c.g.funcs = map[unsafe.Pointer]*funcCopy{}
				}
				c.g.funcs[w] = &funcCopy{v: v}
			}
		}
		return fn
	case *types.Pointer:
		if v.R == nil {
			return reflect.Zero(m.reflectType(t))
		}
		cell, ok := v.R.(*Value)
		if !ok {
			switch p := v.R.(type) {
			case *byte: // into a Go []byte or string: native code shares it
				if rt := m.reflectType(t); rt.Elem().Kind() == reflect.Uint8 {
					return reflect.ValueOf(p).Convert(rt)
				}
			case roByte:
				// Native code (reflect) could write through it into
				// read-only memory: refused, and not as an unsupported
				// conversion, which would box the pointer instead.
				panic(errROToNative)
			case *byteView:
				// Plain data over a Go byte array (byteview.go): native code
				// gets the real pointer, with gc's layout, no copy. The
				// view never carries another kind of type (byteViewAt), and
				// the static type is checked again here: native code must
				// not read a pointer word out of bytes the program wrote.
				if p.ro {
					panic(errROToNative)
				}
				if !m.isPOD(u.Elem()) {
					unsupported("unsafe: a %s laid over bytes cannot go to native code (not plain data)", u.Elem())
				}
				rt := m.reflectType(t)
				if p.off+int(rt.Elem().Size()) <= len(p.b) {
					return reflect.NewAt(rt.Elem(), unsafe.Pointer(p.ptr()))
				}
			case *hdrView: // a string or slice header seen as a struct: a copy
				shadow := reflect.New(m.reflectType(t).Elem())
				shadow.Elem().Set(m.toNativeW(p.load(u.Elem()), u.Elem(), c.nested()))
				return shadow
			}
			unsupported("pointer representation %T", v.R)
		}
		if rv, ok := cell.R.(reflect.Value); ok && rv.CanAddr() {
			// native struct living in native memory: share it
			nv := rv.Addr()
			if c.g.wb != nil {
				if c.g.shared == nil {
					c.g.shared = map[sharedKey]*Value{}
				}
				c.g.shared[sharedKey{nv.Pointer(), nv.Type()}] = cell
			}
			return nv
		}
		if sh, ok := c.g.shadows[cell]; ok {
			return sh
		}
		rt := m.reflectType(t)
		shadow := reflect.New(rt.Elem())
		if c.g.shadows == nil {
			c.g.shadows, c.g.cells = map[*Value]reflect.Value{}, map[uintptr]*Value{}
		}
		c.g.shadows[cell], c.g.cells[shadow.Pointer()] = shadow, cell
		shadow.Elem().Set(m.toNativeW(*cell, u.Elem(), c.nested()))
		if c.g.wb != nil {
			if c.g.snaps == nil {
				c.g.snaps = map[uintptr]reflect.Value{}
			}
			snap := reflect.New(rt.Elem()).Elem()
			snap.Set(shadow.Elem())
			c.g.snaps[shadow.Pointer()] = snap
		}
		if wb != nil {
			*wb = append(*wb, func() { m.fromNativeDeepC(shadow, t, c) })
		}
		return shadow
	case *types.Map:
		rt := m.reflectType(t)
		mp, _ := v.R.(*MapV)
		if mp == nil {
			return reflect.Zero(rt)
		}
		c.g.use(len(mp.M))
		// Maps passed as any (fmt.Println(m)) are read by native code, not
		// updated: no write-back, which costs a native map traversal.
		if c.inAny || wb == nil && !c.keeps() {
			out := reflect.MakeMapWithSize(rt, len(mp.M))
			if o, ok := c.g.enterComp(compKey{unsafe.Pointer(mp), 0, rt}, out); !ok {
				return o
			}
			mp.each(func(k, val Value) {
				out.SetMapIndex(m.toNativeW(k, u.Key(), c.nested()), m.toNativeW(val, u.Elem(), c.nested()))
			})
			return out
		}
		// COPY: updates made by native code (url.Values.Add, maps.Copy,
		// json.Unmarshal through a pointer...) are copied back after the
		// call, into the same interpreted map.
		return m.mapCopyOf(mp, rt, u, c, nil)
	case *types.Array:
		rt := m.reflectType(t)
		out := reflect.New(rt).Elem()
		es, _ := v.R.([]Value)
		for i := range es {
			out.Index(i).Set(m.toNativeW(es[i], u.Elem(), c.nested()))
		}
		return out
	case *types.Chan:
		if v.R == nil {
			return reflect.Zero(m.reflectType(t))
		}
		if ch, ok := v.R.(chan Value); ok && c.inBox && !inChanReader() {
			return m.standIn(ch, t, m.reflectType(t))
		}
		unsupported("passing interpreted channel to native code")
	case *types.Struct:
		rt := m.reflectType(t)
		out := reflect.New(rt).Elem()
		fs, _ := v.R.([]Value)
		cut := m.recursiveFields(u)
		for i := range fs {
			f := out.Field(i)
			if !f.CanSet() {
				f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
			}
			if cut[i] {
				f.Set(m.toNativeAs(fs[i], u.Field(i).Type(), f.Type(), c.nested()))
				continue
			}
			setNative(f, m.toNativeW(fs[i], u.Field(i).Type(), c.nested()))
		}
		return out
	}
	unsupported("toNative %s", t)
	return reflect.Value{}
}

// sliceCopyOf converts the interpreted slice s to a native copy of type rt
// with the same capacity, and records it in the graph of c: after the call,
// the elements are written back into the array of s (sliceBack), and the
// copy (or a longer view of it, after an append in place) converts back to
// the array of s. elem converts an element (nil: toNativeW with et; toNativeU
// gives its own, for the unrolled types of recursive.go).
func (m *Machine) sliceCopyOf(s []Value, rt reflect.Type, et types.Type, c *natConv, elem func(Value) reflect.Value) reflect.Value {
	if elem == nil {
		elem = func(v Value) reflect.Value { return m.toNativeW(v, et, c) }
	}
	out := reflect.MakeSlice(rt, len(s), cap(s))
	if len(s) > 0 {
		// A slice that contains itself gives the native copy being built.
		if o, ok := c.g.enterComp(compKey{unsafe.Pointer(&s[0]), len(s), rt}, out); !ok {
			return o
		}
	}
	_, basic := et.Underlying().(*types.Basic)
	var nat []reflect.Value
	if !basic && cap(s) > 0 {
		nat = make([]reflect.Value, len(s))
	}
	for i := range s {
		nv := elem(s[i])
		out.Index(i).Set(nv)
		if nat != nil {
			nat[i] = nv // Set copied it: native code cannot change nv
		}
	}
	if cap(s) == 0 {
		return out // all empty slices share one address: nothing to write back
	}
	r := &sliceCopy{full: s[:cap(s)], n: len(s), out: out.Slice(0, cap(s)), et: et}
	if basic && cap(s) > len(s) {
		// Native code may write past the length, inside the capacity, as
		// an append in place does (hash.Hash.Sum(id[:0]) into a [32]byte
		// field). The copy starts with the elements there, and sliceBack
		// writes back the ones that changed, even when the program does
		// not use the result.
		for i := len(s); i < cap(s); i++ {
			r.out.Index(i).Set(elem(r.full[i]))
		}
		r.tail = true
	}
	if !basic {
		r.orig = make([]Value, len(s))
		copy(r.orig, s)
		r.nat = nat
		r.keys = make(map[any]int, len(s))
		for i := range s {
			if k, ok := refKey(nat[i]); ok {
				r.keys[k] = i
			}
		}
	}
	if c.g.slices == nil {
		c.g.slices = map[uintptr]*sliceCopy{}
	}
	c.g.slices[out.Pointer()] = r
	m.queueFlush(c)
	return out
}

// sliceBack copies the elements of the native copy back into the
// interpreted array after a native call (once). Elements that native code
// only moved (a sort) keep their interpreted value, so pointer identity
// survives; changed ones are converted back.
func (m *Machine) sliceBack(r *sliceCopy, c *natConv) {
	if r.done {
		return
	}
	r.done = true
	for i := 0; i < r.n; i++ {
		storeChanged(&r.full[i], m.sliceElemBack(r, i, c), r.et)
	}
	if r.tail {
		for i := r.n; i < len(r.full); i++ {
			storeChanged(&r.full[i], m.fromNative(r.out.Index(i), r.et), r.et)
		}
	}
}

func (m *Machine) sliceElemBack(r *sliceCopy, i int, c *natConv) Value {
	ev := r.out.Index(i)
	if r.keys == nil {
		return m.fromNative(ev, r.et)
	}
	// Not changed: the interpreted value stays (compared at the same index,
	// with the bits of floats and the dynamic type of interfaces).
	if natSame(r.nat[i], ev) {
		m.keptRef(ev, r.et, c)
		return r.orig[i]
	}
	// Moved (a sort): only a reference value has an identity to find it by.
	if k, ok := refKey(ev); ok {
		if j, found := r.keys[k]; found {
			m.keptRef(ev, r.et, c)
			return r.orig[j]
		}
	}
	return m.fromNativeDeepC(ev, r.et, c)
}

// sliceView converts back rv, a native slice that starts at the array of
// the copy r: the interpreted slice of the same length over the
// interpreted array, with the elements that native code appended in place.
func (m *Machine) sliceView(r *sliceCopy, rv reflect.Value, c *natConv) Value {
	m.sliceBack(r, c)
	for ; r.n+r.ext < rv.Len(); r.ext++ {
		i := r.n + r.ext
		storeChanged(&r.full[i], m.fromNativeDeepC(r.out.Index(i), r.et, c), r.et)
	}
	return Value{R: r.full[:rv.Len()]}
}

// mapCopyOf converts the interpreted map mp to a native copy of type rt and
// records it in the graph of c (see mapCopy). A map met twice in the same
// call gives the same native map. elem converts a value (nil: toNativeW;
// toNativeU gives its own, for the unrolled types of recursive.go).
func (m *Machine) mapCopyOf(mp *MapV, rt reflect.Type, u *types.Map, c *natConv, elem func(Value) reflect.Value) reflect.Value {
	if r, ok := c.g.maps[mp]; ok && r.out.Type() == rt {
		return r.out
	}
	if elem == nil {
		elem = func(v Value) reflect.Value { return m.toNativeW(v, u.Elem(), c.nested()) }
	}
	out := reflect.MakeMapWithSize(rt, len(mp.M))
	// c.g.maps already stops a cycle of copies; enterComp also finds a map
	// that a read only conversion (the fast path of toNativeW) is building.
	if o, ok := c.g.enterComp(compKey{unsafe.Pointer(mp), 0, rt}, out); !ok {
		return o
	}
	r := &mapCopy{mp: mp, out: out, t: u, orig: make(map[any]origEntry, len(mp.M))}
	if c.g.maps == nil {
		c.g.maps, c.g.nmaps = map[*MapV]*mapCopy{}, map[uintptr]*mapCopy{}
	}
	c.g.maps[mp], c.g.nmaps[out.Pointer()] = r, r
	m.queueFlush(c)
	for sk, val := range mp.M {
		nv := elem(val)
		out.SetMapIndex(m.toNativeW(mp.key(sk), u.Key(), c.nested()), nv)
		r.orig[sk] = origEntry{nat: nv, v: val} // SetMapIndex copied nv
		if nk, ok := refKey(nv); ok {
			if r.refs == nil {
				r.refs = map[any]Value{}
			}
			r.refs[nk] = val
		}
	}
	return out
}

// boxedCopy returns the interpreted interface value that the native value
// rv, the dynamic value of a native interface, is the copy of: a map or a
// slice of an interface reached through a pointer (a struct field of type
// any that native code did not replace), or the shadow of a pointer that
// native code kept in the interface (ptrCopy), written into its cell.
func (m *Machine) boxedCopy(rv reflect.Value, c *natConv) *Iface {
	switch rv.Kind() {
	case reflect.Map:
		if r, ok := c.g.nmaps[rv.Pointer()]; ok && r.iv != nil && r.out.Type() == rv.Type() {
			m.mapBack(r, c)
			return r.iv
		}
	case reflect.Slice:
		if r, ok := c.g.slices[rv.Pointer()]; ok && r.iv != nil && r.out.Type() == rv.Type() && rv.Len() == r.n && rv.Cap() == r.out.Len() {
			m.sliceBack(r, c)
			return r.iv
		}
	case reflect.Func:
		if w, ok := funcWord(rv); ok && c.g.funcs != nil {
			if r, ok := c.g.funcs[w]; ok && r.iv != nil {
				return r.iv
			}
		}
	case reflect.Pointer:
		if r, ok := c.g.ptrs[rv.Pointer()]; ok && r.rt == rv.Type() {
			if !r.shared {
				m.fromNativeDeepC(rv, r.t, c) // writes the shadow into the cell of r.iv
			}
			return r.iv
		}
	}
	return nil
}

// noteBoxed records that the native value nv, just converted for the
// interface value iv, is the copy of its map or slice (see boxedCopy).
func (m *Machine) noteBoxed(nv reflect.Value, iv *Iface, c *natConv) {
	switch nv.Kind() {
	case reflect.Map:
		if r, ok := c.g.nmaps[nv.Pointer()]; ok && r.out.Type() == nv.Type() {
			r.iv = iv
		}
	case reflect.Slice:
		if nv.Cap() == 0 {
			return
		}
		if r, ok := c.g.slices[nv.Pointer()]; ok && r.out.Type() == nv.Type() {
			r.iv = iv
		}
	}
}

// mapBack writes the content of the native copy back into the interpreted
// map after a native call (once).
func (m *Machine) mapBack(r *mapCopy, c *natConv) {
	if r.done {
		return
	}
	r.done = true
	m.mapWriteBack(r, c)
}

// mapValueBack returns the interpreted value for nv, the native value now at
// the key k (a key of mp.M) of the copy r. A value that native code did not
// change keeps its interpreted value: the value converted in for the same
// key is compared with natSame (the bits of floats, the dynamic type of
// interfaces). A reference value moved from another key (pointer, map,
// slice) keeps its interpreted value too, found by its identity.
func (m *Machine) mapValueBack(r *mapCopy, k any, nv reflect.Value, c *natConv) Value {
	if e, ok := r.orig[k]; ok && natSame(e.nat, nv) {
		m.keptRef(nv, r.t.Elem(), c)
		return e.v
	}
	if nk, ok := refKey(nv); ok {
		if v, found := r.refs[nk]; found {
			m.keptRef(nv, r.t.Elem(), c)
			return v
		}
	}
	return m.fromNativeDeepC(nv, r.t.Elem(), c)
}

// shadowSame reports whether native code did not change the content of
// the shadow rv (a pointer to an interpreted value of type et) during the
// call: it is the same as the copy made right after the conversion
// (natSame). Then the interpreted cell is not written: converting the
// shadow back is lossy (an any field that holds an interpreted named value
// T{1} comes back as struct { X int }), and a read only call (json.Marshal
// of a shared config) must not write shared values. The pointers in the
// shadow may lead to shadows that native code changed: keptRef writes them
// back.
func (m *Machine) shadowSame(rv reflect.Value, et types.Type, c *natConv) bool {
	snap, ok := c.g.snaps[rv.Pointer()]
	if !ok || snap.Type() != rv.Type().Elem() || !natSame(rv.Elem(), snap) {
		return false
	}
	m.keptRef(rv.Elem(), et, c)
	return true
}

// keptRef writes back the pointers in nv, a value of type t that native
// code kept in a slice or a map (natSame, or moved), into their interpreted
// cells: native code may have written the pointed values. encoding/json,
// xml and gob decode into a non nil pointer that is already there
// (json.Unmarshal into a []*T or a []any holding pointers). Only shadows of
// interpreted cells are written (fromNativeDeepC, boxedCopy); the cells are
// written only when their value changed (storeChanged).
func (m *Machine) keptRef(nv reflect.Value, t types.Type, c *natConv) {
	if c.g.cells == nil || !nv.IsValid() {
		return
	}
	switch nv.Kind() {
	case reflect.Pointer:
		if _, ok := t.Underlying().(*types.Pointer); ok && !nv.IsNil() {
			if _, ok := c.g.cells[nv.Pointer()]; ok {
				m.fromNativeDeepC(nv, t, c)
			}
		}
	case reflect.Interface:
		if types.IsInterface(t) && !nv.IsNil() && nv.Elem().Kind() == reflect.Pointer {
			m.boxedCopy(nv.Elem(), c)
		}
	case reflect.Struct:
		st, ok := t.Underlying().(*types.Struct)
		if !ok || m.isNativeNamed(t) || st.NumFields() != nv.NumField() || m.hasCut(t) {
			return
		}
		nv = addressable(nv)
		for i := 0; i < st.NumFields(); i++ {
			f := nv.Field(i)
			if !f.CanInterface() {
				f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
			}
			m.keptRef(f, st.Field(i).Type(), c)
		}
	case reflect.Array:
		at, ok := t.Underlying().(*types.Array)
		if !ok || m.isNativeNamed(t) {
			return
		}
		for i := 0; i < nv.Len(); i++ {
			m.keptRef(nv.Index(i), at.Elem(), c)
		}
	}
}

// mapWriteBack writes the content of the native copy out back into the
// interpreted map mp after a native call. Values that native code did not
// change keep their interpreted value (mapValueBack). Only the entries
// that changed are written or deleted: when native code only read the map
// (json.Marshal), mp is not written at all, so other goroutines can read it
// at the same time, as in Go.
//
// Entries with a NaN key cannot be found by a lookup. Native code cannot
// change their value in place: it can only add new NaN entries or clear the
// map. When the NaN entries of the copy are the ones converted in
// (nanKept), they are left as they are and the other entries are written
// back as above. Else the map is rebuilt (mapRebuild).
func (m *Machine) mapWriteBack(r *mapCopy, c *natConv) {
	mp, out, t := r.mp, r.out, r.t
	nan := nanKeys(mp.M)
	if nan > 0 && !r.nanKept(nan) {
		m.mapRebuild(r, c)
		return
	}
	seen := make(map[any]struct{}, out.Len())
	it := out.MapRange()
	for it.Next() {
		if nan > 0 && isNaNKey(it.Key()) {
			continue // kept as they are (nanKept)
		}
		kv := m.fromNativeDeepC(it.Key(), t.Key(), c)
		k := storeKey(keyOf(kv, t.Key()))
		seen[k] = struct{}{}
		v := m.mapValueBack(r, k, it.Value(), c)
		if old, ok := mp.M[k]; ok {
			if !sameVMValue(old, v, t.Elem()) {
				mp.M[k] = v // the key stays the old one, as in Go
			}
			continue
		}
		mp.M[k] = v
		mp.setKey(k, kv)
	}
	if len(mp.M) == len(seen)+nan {
		return
	}
	for k := range mp.M {
		if _, ok := k.(nanKey); ok && nan > 0 {
			continue // kept as they are (nanKept)
		}
		if _, ok := seen[k]; !ok {
			delete(mp.M, k)
			mp.deleteKey(k)
		}
	}
}

// nanKept reports whether the NaN entries of the native copy are the nan
// NaN entries of mp converted in, unchanged: the same number, each one
// with the same native value (natSame) as one NaN entry of r.orig, still
// in mp.
func (r *mapCopy) nanKept(nan int) bool {
	var orig []reflect.Value
	for k, e := range r.orig {
		if _, ok := k.(nanKey); ok {
			if _, still := r.mp.M[k]; !still {
				return false
			}
			orig = append(orig, e.nat)
		}
	}
	if len(orig) != nan {
		return false
	}
	n := 0
	it := r.out.MapRange()
	for it.Next() {
		if !isNaNKey(it.Key()) {
			continue
		}
		n++
		if n > nan {
			return false
		}
		nv := it.Value()
		found := false
		for i, ov := range orig {
			if ov.IsValid() && natSame(ov, nv) {
				orig[i], found = reflect.Value{}, true // used once
				break
			}
		}
		if !found {
			return false
		}
	}
	return n == nan
}

// isNaNKey reports whether the native map key k is not equal to itself (a
// NaN, or a value that holds a NaN): a key that no lookup can find.
func isNaNKey(k reflect.Value) bool {
	return !k.Equal(k)
}

// mapRebuild replaces the content of mp with the one of out, for a map with
// NaN keys: they cannot be found by a lookup, so mapWriteBack cannot write
// only the changes.
func (m *Machine) mapRebuild(r *mapCopy, c *natConv) {
	mp, out, t := r.mp, r.out, r.t
	nan := nanKeys(mp.M)
	defer func() {
		// NaN keys can only go away by a clear: when the native copy lost
		// some, the native code cleared the map, and a range loop over mp
		// must stop giving them (MapV.clear). A write back that keeps the
		// NaN keys is not a clear.
		if nanKeys(mp.M) < nan {
			mp.clears++
		}
	}()
	clear(mp.M)
	if mp.K != nil {
		clear(mp.K)
	}
	it := out.MapRange()
	for it.Next() {
		kv := m.fromNativeDeepC(it.Key(), t.Key(), c)
		k := storeKey(keyOf(kv, t.Key()))
		mp.setKey(k, kv)
		mp.M[k] = m.mapValueBack(r, k, it.Value(), c)
	}
}

// storeChanged stores v of type t in *dst, unless *dst already holds the
// same value (sameVMValue). The write-backs of a native call use it: interpreted
// memory that native code did not change is not written, so other
// goroutines can read it during the call, as in Go (json.Marshal of a shared
// config).
func storeChanged(dst *Value, v Value, t types.Type) {
	if !sameVMValue(*dst, v, t) {
		*dst = v
	}
}

// sameVMValue reports whether a and b, two interpreted values of type t, are
// the same value: storing b over a changes nothing that the program can see.
// Composite values are compared by identity (same array, same map, same
// cell), structs and arrays field by field. When it cannot tell, it reports
// false (the value is written, as before).
func sameVMValue(a, b Value, t types.Type) bool {
	if a.N != b.N {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		as, ok1 := a.R.([]Value)
		bs, ok2 := b.R.([]Value)
		if ok1 && ok2 {
			if len(as) != len(bs) || len(as) != u.NumFields() {
				return false
			}
			for i := range as {
				if !sameVMValue(as[i], bs[i], u.Field(i).Type()) {
					return false
				}
			}
			return true
		}
	case *types.Array:
		as, ok1 := a.R.([]Value)
		bs, ok2 := b.R.([]Value)
		if ok1 && ok2 {
			if len(as) != len(bs) {
				return false
			}
			for i := range as {
				if !sameVMValue(as[i], bs[i], u.Elem()) {
					return false
				}
			}
			return true
		}
	case *types.Slice:
		if as, ok := a.R.([]Value); ok {
			bs, ok := b.R.([]Value)
			return ok && len(as) == len(bs) && cap(as) == cap(bs) && (cap(as) == 0 || &as[:1][0] == &bs[:1][0])
		}
		if as, ok := a.R.([]byte); ok {
			bs, ok := b.R.([]byte)
			return ok && len(as) == len(bs) && cap(as) == cap(bs) && (cap(as) == 0 || &as[:1][0] == &bs[:1][0])
		}
		if as, ok := natViewOf(a.R); ok {
			// two views of the same native array (natslice.go)
			bs, ok := natViewOf(b.R)
			return ok && as.Type() == bs.Type() && as.Len() == bs.Len() && as.Cap() == bs.Cap() && as.UnsafePointer() == bs.UnsafePointer()
		}
	case *types.Interface:
		ai, ok1 := a.R.(*Iface)
		bi, ok2 := b.R.(*Iface)
		if ok1 && ok2 {
			if ai == bi {
				return true
			}
			if ai == nil || bi == nil || ai.T != bi.T {
				return false
			}
			if ai.T.T == nil {
				return ai.V.N == bi.V.N && sameR(ai.V.R, bi.V.R)
			}
			return sameVMValue(ai.V, bi.V, ai.T.T)
		}
	}
	return sameR(a.R, b.R)
}

// sameR compares the R words of two values that sameVMValue does not know
// better: comparable Go values with ==, native values with natSame.
func sameR(a, b any) (same bool) {
	if ra, ok := a.(reflect.Value); ok {
		rb, ok := b.(reflect.Value)
		return ok && natSame(ra, rb)
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta := reflect.TypeOf(a)
	if ta != reflect.TypeOf(b) || !ta.Comparable() {
		return false
	}
	switch x := a.(type) {
	case float64: // the bits: -0 is not 0 for the program (1/x, Signbit)
		return math.Float64bits(x) == math.Float64bits(b.(float64))
	case float32:
		return math.Float32bits(x) == math.Float32bits(b.(float32))
	case complex128:
		y := b.(complex128)
		return math.Float64bits(real(x)) == math.Float64bits(real(y)) && math.Float64bits(imag(x)) == math.Float64bits(imag(y))
	case complex64:
		y := b.(complex64)
		return math.Float32bits(real(x)) == math.Float32bits(real(y)) && math.Float32bits(imag(x)) == math.Float32bits(imag(y))
	}
	defer func() {
		if recover() != nil {
			same = false // an interface field that holds a value without ==
		}
	}()
	return a == b
}

// natSame reports whether two native values are the same: the same bits
// for scalars, the same memory for slices, maps and pointers, and the same
// fields or elements for structs and arrays, and the same func value word
// for funcs (funcWord).
func natSame(a, b reflect.Value) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Bool:
		return a.Bool() == b.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() == b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return a.Uint() == b.Uint()
	case reflect.Float32, reflect.Float64:
		return math.Float64bits(a.Float()) == math.Float64bits(b.Float())
	case reflect.Complex64, reflect.Complex128:
		x, y := a.Complex(), b.Complex()
		return math.Float64bits(real(x)) == math.Float64bits(real(y)) && math.Float64bits(imag(x)) == math.Float64bits(imag(y))
	case reflect.String:
		return a.String() == b.String()
	case reflect.Slice:
		return a.Pointer() == b.Pointer() && a.Len() == b.Len() && a.Cap() == b.Cap()
	case reflect.Map, reflect.Pointer, reflect.Chan, reflect.UnsafePointer:
		return a.Pointer() == b.Pointer()
	case reflect.Func:
		// The func value word, not Pointer() (see refKey).
		wa, ok1 := funcWord(a)
		wb, ok2 := funcWord(b)
		return ok1 && ok2 && wa == wb
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() && b.IsNil()
		}
		return natSame(a.Elem(), b.Elem())
	case reflect.Array:
		for i := range a.Len() {
			if !natSame(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		// Addressable: funcWord can read the funcs of unexported fields.
		if !a.CanAddr() && a.CanInterface() {
			a = addressable(a)
		}
		if !b.CanAddr() && b.CanInterface() {
			b = addressable(b)
		}
		for i := range a.NumField() {
			if !natSame(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	}
	return false
}

// nanKeys returns the number of NaN keys (nanKey) in the map M of a MapV.
func nanKeys(M map[any]Value) int {
	n := 0
	for k := range M {
		if _, ok := k.(nanKey); ok {
			n++
		}
	}
	return n
}

// refKey returns the identity of a native reference value (pointer, map,
// slice, func, chan, also inside an interface). Other values have no
// identity: two equal scalars (0 and -0, MyInt(1) and 1) are not the same
// value, so they are never found by a lookup.
func refKey(rv reflect.Value) (any, bool) {
	if !rv.IsValid() {
		return nil, false
	}
	if rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice:
		return refID{rv.Type(), rv.Pointer(), rv.Len()}, true
	case reflect.Func:
		// Not Pointer(): it is the code pointer, the same for all the
		// closures made by reflect.MakeFunc (interpreted funcs given to
		// native code). The func value word tells closures apart.
		if w, ok := funcWord(rv); ok && w != nil {
			return refID{rv.Type(), uintptr(w), 0}, true
		}
		return nil, false
	case reflect.Map, reflect.Chan, reflect.Pointer, reflect.UnsafePointer:
		// With the type: a pointer to a struct and a pointer to its first
		// field have the same address.
		return refID{rv.Type(), rv.Pointer(), 0}, true
	}
	return nil, false
}

// funcWord returns the word of the func value rv: the pointer to its
// closure (for reflect.MakeFunc, its own makeFuncImpl). Two func values
// with the same word are the same func. ok is false when rv cannot be read
// (a func in an unexported field, not addressable).
func funcWord(rv reflect.Value) (w unsafe.Pointer, ok bool) {
	if rv.CanAddr() {
		return *(*unsafe.Pointer)(rv.Addr().UnsafePointer()), true
	}
	if !rv.CanInterface() {
		return nil, false
	}
	// A func is a direct interface value: the data word of the interface
	// is the func value word.
	i := rv.Interface()
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&i))[1], true
}

// refID is the identity of a native reference value (refKey).
type refID struct {
	t reflect.Type
	p uintptr
	n int
}

func (m *Machine) unpack(res Value, rs *types.Tuple, ft reflect.Type) []reflect.Value {
	switch rs.Len() {
	case 0:
		return nil
	case 1:
		return []reflect.Value{m.resultToNative(res, rs.At(0).Type())}
	}
	t := res.R.([]Value)
	out := make([]reflect.Value, len(t))
	for i := range t {
		out[i] = m.resultToNative(t[i], rs.At(i).Type())
	}
	return out
}

// resultToNative converts v, a result of type t of a method of the program
// that native code called. A result of type any whose dynamic type is plain
// data (plainType: a struct or array of scalars and strings, with no name of
// the program) is a native copy, also in AOT mode or with OpaqueAny, where
// box gives opaque handles: native code asserts it to its native type. A
// copy is right for such a value only: it has nothing that aliases. A map,
// slice, pointer, chan or func keeps its handle, so a write through the
// value the program gets back changes the program's value, as in Go. Known
// limit: fcgi.ProcessEnv asserts the value of a Context of the program to
// map[string]string, and gets nil in AOT mode. A func
// whose type is an unnamed func type without a name of the program is a
// native func too (plainFuncType): net asserts the alternate resolver that
// a Context of the program gives to its func type. A func value is a
// reference, so the native func that calls the closure keeps the meaning
// of the program; a named func type of the program keeps its handle.
func (m *Machine) resultToNative(v Value, t types.Type) reflect.Value {
	if iv, ok := v.R.(*Iface); ok && iv != nil && iv.T.T != nil && types.IsInterface(t) && (m.plainType(iv.T.T) || m.plainFuncType(iv.T.T)) {
		c := newConv(nil)
		c.g.plain = true
		return m.toNativeW(v, t, c)
	}
	return m.toNative(v, t)
}

// plainFuncType reports whether t is an unnamed func type whose signature
// has no name of the program. Its native func type is all there is to it.
func (m *Machine) plainFuncType(t types.Type) bool {
	if _, named := types.Unalias(t).(*types.Named); named {
		return false
	}
	_, ok := t.Underlying().(*types.Signature)
	return ok && !m.mentionsInterpNamed(t)
}

// plainType reports whether t is an unnamed struct or array type of plain
// data: no name of the program, and only booleans, numbers, strings, arrays
// and structs inside, so no map, slice, pointer, chan, func or interface.
// Its native copy is the whole value. The answer is cached per type.
func (m *Machine) plainType(t types.Type) bool {
	if r, ok := m.plainTypes.Load(t); ok {
		return r.(bool)
	}
	ok := false
	if _, named := types.Unalias(t).(*types.Named); !named {
		switch t.Underlying().(type) {
		case *types.Array, *types.Struct:
			ok = !m.mentionsInterpNamed(t) && plainData(t)
		}
	}
	m.plainTypes.Store(t, ok)
	return ok
}

// plainData reports whether t holds only booleans, numbers and strings,
// directly or in arrays and structs.
func plainData(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() != types.UnsafePointer && u.Kind() != types.UntypedNil
	case *types.Array:
		return plainData(u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if !plainData(u.Field(i).Type()) {
				return false
			}
		}
		return true
	}
	return false
}

var kindToBasic = map[reflect.Kind]types.Type{
	reflect.Bool: types.Typ[types.Bool], reflect.Int: types.Typ[types.Int], reflect.Int8: types.Typ[types.Int8],
	reflect.Int16: types.Typ[types.Int16], reflect.Int32: types.Typ[types.Int32], reflect.Int64: types.Typ[types.Int64],
	reflect.Uint: types.Typ[types.Uint], reflect.Uint8: types.Typ[types.Uint8], reflect.Uint16: types.Typ[types.Uint16],
	reflect.Uint32: types.Typ[types.Uint32], reflect.Uint64: types.Typ[types.Uint64], reflect.Float64: types.Typ[types.Float64],
	reflect.String: types.Typ[types.String], reflect.Complex128: types.Typ[types.Complex128],
	reflect.Complex64: types.Typ[types.Complex64],
}

// unwrapCut returns the value held by rv when rv is an any standing for a
// non-interface type (a recursive field, see reflectType); ok is false for
// a nil any.
func unwrapCut(rv reflect.Value, t types.Type) (_ reflect.Value, ok bool) {
	if rv.Kind() != reflect.Interface || types.IsInterface(t) {
		return rv, true
	}
	if rv.IsNil() {
		return rv, false
	}
	e := rv.Elem()
	if !cutKindOK(e.Kind(), t) {
		// Native code put its own value in the any (a decoder fills an
		// any with a map[string]any): see recursive.go.
		panic(fmt.Sprintf("vm: native code stored a %s where yaegi v2 expects a value of the recursive type %s", e.Type(), t))
	}
	return e, true
}

// addressable returns rv when reflect can take its address, else a copy of
// rv in new memory. The conversions back from native code read unexported
// fields through their address (reflect.NewAt), but the value of a map entry
// (json.Unmarshal into a map[string]T), and what it holds, has no address.
func addressable(rv reflect.Value) reflect.Value {
	if rv.CanAddr() {
		return rv
	}
	c := reflect.New(rv.Type()).Elem()
	c.Set(rv)
	return c
}

func (m *Machine) fromNative(rv reflect.Value, t types.Type) Value {
	return m.fromNativeS(rv, t, nil)
}

// fromNativeS is fromNative with the native slices being converted (nil
// until one is met): a slice whose elements are interfaces can contain
// itself (type T []T, whose native type is []any, with t[0] = t).
func (m *Machine) fromNativeS(rv reflect.Value, t types.Type, seen *natSeen) Value {
	rv, ok := unwrapCut(rv, t)
	if !ok {
		return deepCopy(m.zero(t), t)
	}
	if tw := m.twinNamed(t); tw != nil {
		if v, ok := m.twinToVM(rv); ok {
			return v
		}
		if v, ok := m.realReflectToVM(rv, tw); ok {
			return v
		}
	}
	if isUniqueHandle(t) {
		return handleFromNative(rv)
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Kind() == types.UnsafePointer:
			return Value{R: rv}
		case u.Info()&types.IsBoolean != 0:
			return Value{N: b2i(rv.Bool())}
		case u.Info()&types.IsString != 0:
			return Value{R: rv.String()}
		case u.Info()&types.IsComplex != 0:
			return Value{R: rv.Complex()}
		case u.Info()&types.IsFloat != 0:
			return Value{N: fv(rv.Float())}
		case u.Info()&types.IsUnsigned != 0:
			return Value{N: int64(rv.Uint())}
		}
		return Value{N: rv.Int()}
	case *types.Interface:
		if !rv.IsValid() {
			return Value{}
		}
		if rv.Kind() == reflect.Interface {
			if rv.IsNil() {
				return Value{}
			}
			rv = rv.Elem()
		}
		if rv.CanInterface() {
			if w, ok := rv.Interface().(vmBoxed); ok && m.owns(w) { // round trip: unwrap
				if h, ok := w.obj().host(); ok {
					rv = h // a value of the host in an AOT wrapper (hostWrapper)
				} else {
					return Value{R: w.iface()}
				}
			} else if iv, ok := m.unwrapWrapper(rv); ok {
				return Value{R: iv}
			}
		}
		if iv := m.shadowOf(rv); iv != nil {
			return Value{R: iv}
		}
		return m.ifaceVM(rv)
	case *types.Slice:
		if rv.IsNil() {
			return Value{}
		}
		if optBytes && rv.Type().Elem().Kind() == reflect.Uint8 {
			return Value{R: rv.Bytes()} // shared, zero-copy
		}
		if natViewable(rv, u.Elem()) {
			return Value{R: natView(rv)} // shared: writes reach native memory
		}
		// Other elements need a conversion: a copy, writes through it do
		// not reach the native array.
		if seen == nil {
			seen = &natSeen{}
		}
		seen.grow(rv.Len())
		out := make([]Value, rv.Len())
		if len(out) > 0 && convShares(rv.Type().Elem().Kind()) {
			// Converted once, also when it is reached again (natSeen).
			if o, ok := seen.enter(compKey{rv.UnsafePointer(), rv.Len(), rv.Type()}, t, Value{R: out}); !ok {
				return o
			}
		}
		for i := range out {
			out[i] = m.fromNativeS(rv.Index(i), u.Elem(), seen)
		}
		return Value{R: out}
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return Value{}
		}
		if s := m.standInOf(rv); s != nil {
			return Value{R: s.ch} // the stand-in of an interpreted channel
		}
		if cl := m.closureOfFunc(rv, t); cl != nil {
			return Value{R: cl} // the native func of an interpreted closure
		}
		if rv.Type() == stdsyms.LoggerType && !m.realStd {
			// a plugin logger stored in a native place (setNative); the
			// pointer itself, so a value read from an unexported field
			// (the twin reflect) works too
			if w, ok := stdsyms.PluginLogger((*log.Logger)(rv.UnsafePointer())); ok {
				return Value{R: w}
			}
		}
		if rv.CanAddr() && rv.CanInterface() {
			// rv is native memory (a field read through a native pointer):
			// keep its value, not the place. Else the Value follows later
			// writes to that place (orig := p.Director; p.Director = f
			// made orig call f). These kinds fit in an interface word,
			// so this does not allocate.
			rv = reflect.ValueOf(rv.Interface())
		}
	}
	if a, ok := t.Underlying().(*types.Array); ok && !m.isNativeNamed(t) {
		// a native array loaded into the VM ([1<<28]byte(s) of a []byte
		// s, through a native pointer): one Value per element, bounded
		if seen == nil {
			seen = &natSeen{}
		}
		seen.grow(rv.Len())
		out := make([]Value, rv.Len())
		for i := range out {
			out[i] = m.fromNativeS(rv.Index(i), a.Elem(), seen)
		}
		return Value{R: out}
	}
	return Value{R: rv}
}

// convShares reports whether the elements of a native slice of kind k can
// reach other slices (directly, or through an interface or an array): such a
// slice can be shared or contain itself, so fromNativeS converts it once
// (natSeen). Other slices cannot make the conversion grow.
func convShares(k reflect.Kind) bool {
	return k == reflect.Interface || k == reflect.Slice || k == reflect.Array
}

// fromNativeDeep converts native data back into interpreted representation,
// including structs of interpreted types (used for pointer write-back).
func (m *Machine) fromNativeDeep(rv reflect.Value, t types.Type) Value {
	return m.fromNativeDeepC(rv, t, newConv(nil))
}

// fromNativeDeepC is fromNativeDeep for the write-backs of a native call:
// shadows of interpreted pointers update and give back their interpreted
// cell, so pointer identity survives, and cycles are followed once.
func (m *Machine) fromNativeDeepC(rv reflect.Value, t types.Type, c *natConv) Value {
	rv, ok := unwrapCut(rv, t)
	if !ok {
		return deepCopy(m.zero(t), t)
	}
	if m.twinNamed(t) != nil {
		// reflect.Type or reflect.Value of the program's reflect: its native
		// value is a twin (RT, RV), not a struct of the fields of t. A
		// shadowed *reflect.Value (the sync.Pool of fxamacker/cbor) is
		// detached here.
		return m.fromNativeS(rv, t, nil)
	}
	if isUniqueHandle(t) {
		return handleFromNative(rv)
	}
	if types.IsInterface(t) && rv.IsValid() && rv.Kind() == reflect.Interface && !rv.IsNil() {
		if iv := m.boxedCopy(rv.Elem(), c); iv != nil {
			return Value{R: iv}
		}
	}
	if _, ok := t.Underlying().(*types.Signature); ok && c.g.funcs != nil && rv.IsValid() && rv.Kind() == reflect.Func {
		// a func made for an interpreted closure of this call: the closure
		if w, ok := funcWord(rv); ok {
			if r, found := c.g.funcs[w]; found {
				return r.v
			}
		}
	}
	if st, ok := t.Underlying().(*types.Struct); ok && !m.isNativeNamed(t) {
		rv = addressable(rv)
		c.g.nopen.grow(st.NumFields())
		fs := make([]Value, st.NumFields())
		for i := range fs {
			f := rv.Field(i)
			if !f.CanInterface() {
				f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
			}
			fs[i] = m.fromNativeDeepC(f, st.Field(i).Type(), c)
		}
		return Value{R: fs}
	}
	if mt, ok := t.Underlying().(*types.Map); ok && !m.isNativeNamed(t) {
		if rv.IsNil() {
			return Value{}
		}
		if r, ok := c.g.nmaps[rv.Pointer()]; ok && r.out.Type() == rv.Type() {
			// the copy of an interpreted map: the same map, updated
			m.mapBack(r, c)
			return Value{R: r.mp}
		}
		c.g.nopen.grow(2 * rv.Len()) // a key and a value per entry
		mp := newMapV(0)
		// A native map can contain itself (type M map[string]M with
		// m["self"] = m, stored in a sync.Map or an atomic.Value): give
		// back the VM map being built, so the VM map has the same cycle.
		if o, ok := c.g.nopen.enter(compKey{rv.UnsafePointer(), 0, rv.Type()}, t, Value{R: mp}); !ok {
			return o
		}
		it := rv.MapRange()
		for it.Next() {
			k := m.fromNativeDeepC(it.Key(), mt.Key(), c)
			kk := storeKey(keyOf(k, mt.Key()))
			mp.M[kk] = m.fromNativeDeepC(it.Value(), mt.Elem(), c)
			mp.setKey(kk, k)
		}
		return Value{R: mp}
	}
	if sl, ok := t.Underlying().(*types.Slice); ok {
		if rv.IsNil() {
			return Value{}
		}
		if optBytes && rv.Type().Elem().Kind() == reflect.Uint8 {
			return Value{R: rv.Bytes()}
		}
		if r, ok := c.g.slices[rv.Pointer()]; ok && r.out.Type() == rv.Type() && rv.Cap() == r.out.Len() {
			// a view of the copy of an interpreted slice: the same array
			return m.sliceView(r, rv, c)
		}
		if natViewable(rv, sl.Elem()) {
			return Value{R: natView(rv)}
		}
		c.g.nopen.grow(rv.Len())
		out := make([]Value, rv.Len())
		if len(out) > 0 {
			// a native slice that contains itself (type T []T with
			// t[0] = t): the VM slice has the same cycle
			if o, ok := c.g.nopen.enter(compKey{rv.UnsafePointer(), rv.Len(), rv.Type()}, t, Value{R: out}); !ok {
				return o
			}
		}
		for i := range out {
			out[i] = m.fromNativeDeepC(rv.Index(i), sl.Elem(), c)
		}
		return Value{R: out}
	}
	if at, ok := t.Underlying().(*types.Array); ok && !m.isNativeNamed(t) {
		c.g.nopen.grow(rv.Len())
		out := make([]Value, rv.Len())
		for i := range out {
			out[i] = m.fromNativeDeepC(rv.Index(i), at.Elem(), c)
		}
		return Value{R: out}
	}
	if rv.Kind() == reflect.Pointer && c.g.shared != nil && !rv.IsNil() {
		if cell, ok := c.g.shared[sharedKey{rv.Pointer(), rv.Type()}]; ok {
			return Value{R: cell} // the program's own pointer, see convGraph.shared
		}
	}
	// A pointer to a native named type is a native value, except the shadow
	// of an interpreted cell (toNativeW): json.Unmarshal(b, &n) with n a
	// json.Number or a time.Duration fills the shadow, which must be written
	// into the cell. Without this, n stayed empty.
	if pt, ok := t.Underlying().(*types.Pointer); ok && (!m.isNativeNamed(pt.Elem()) || isCellShadow(rv, c)) {
		if rv.IsNil() {
			return Value{}
		}
		addr := rv.Pointer()
		if cell, ok := c.g.back[addr]; ok {
			return Value{R: cell}
		}
		cell, isShadow := c.g.cells[addr] // a shadow: update the interpreted cell
		if !isShadow {
			cell = new(Value)
		}
		if c.g.back == nil {
			c.g.back = map[uintptr]*Value{}
		}
		c.g.back[addr] = cell
		if isShadow && m.shadowSame(rv, pt.Elem(), c) {
			return Value{R: cell}
		}
		storeChanged(cell, m.fromNativeDeepC(rv.Elem(), pt.Elem(), c), pt.Elem())
		return Value{R: cell}
	}
	return m.fromNative(rv, t)
}

// isCellShadow reports whether rv is a native pointer made for an
// interpreted cell during the call of c (a shadow, see toNativeW). The type
// is checked too: a pointer to the first field of a shadowed struct has the
// same address.
func isCellShadow(rv reflect.Value, c *natConv) bool {
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return false
	}
	cell, ok := c.g.cells[rv.Pointer()]
	if !ok {
		return false
	}
	sh, ok := c.g.shadows[cell]
	return ok && sh.Type() == rv.Type()
}

// ---- the reflect wall: boxing interpreted values with methods ----
//
// Native code can only see methods of interpreted values through a finite set
// of pre-compiled wrapper types, chosen by method set.

type vmBoxed interface {
	iface() *Iface
	obj() vmObj
}

type vmObj struct {
	m  *Machine
	iv *Iface
	// origin is the account of the call that gave the value to native code
	// (originAcct): its methods that native code calls on a goroutine of its
	// own use it (withOrigin).
	origin *account
	// seg is set on the handle that boxCopy gives past maxBoxDepth: the
	// number of such bounds above it (see fromBoxed and deepSegs)
	seg int
}

// hostObj is a value of the host in the wrapper of an AOT interface
// (hostWrapper): its methods are called directly. It has no machine (m),
// and iv holds the value as a reflect.Value: a field for it would make
// every vmObj bigger.
func hostObj(x reflect.Value) vmObj {
	return vmObj{iv: &Iface{T: &RType{Native: x.Type()}, V: Value{R: x}}}
}

// host returns the value of the host of a hostObj.
func (o vmObj) host() (reflect.Value, bool) {
	if o.m != nil || o.iv == nil {
		return reflect.Value{}, false
	}
	x, ok := o.iv.V.R.(reflect.Value)
	return x, ok
}

func (o vmObj) iface() *Iface { return o.iv }
func (o vmObj) obj() vmObj    { return o }

// owns reports whether the boxed value b can be unwrapped by m: a value of
// m, or a value of the host (hostObj). A value of another machine (Traefik
// chains plugins, one machine each) stays a native value for m: its types
// are not types of m, and its methods run in its own machine.
func (m *Machine) owns(b vmBoxed) bool {
	o := b.obj()
	return o.m == nil || o.m == m
}

// callGeneric calls fn, a method of an instance of a generic type of a
// native package with a type of the program (sql.Null[T].Value, which
// database/sql calls through driver.Valuer). It has no native instance and
// is not interpreted: genericIntrinsic implements it, as for a call
// through an interface in the program (callGenericMethod). callNative
// panicked with "has no method Value".
func (o vmObj) callGeneric(fn *ssa.Function, in []reflect.Value) []reflect.Value {
	defer o.m.resumeShadows(o.m.suspendShadows())
	defer o.m.nativeCallback(o.origin)()
	sig := fn.Signature
	regs := make([]Value, len(in))
	idx := make([]int32, len(in))
	for i := range in {
		regs[i], idx[i] = o.m.fromNative(in[i], sig.Params().At(i).Type()), int32(i)
	}
	return o.m.unpack(o.m.callGenericMethod(fn, o.iv.V, regs, idx), sig.Results(), nil)
}

func (o vmObj) call(name string, in ...reflect.Value) []reflect.Value {
	if h, ok := o.host(); ok {
		return h.MethodByName(name).Call(in)
	}
	fn := o.iv.T.Methods[name]
	if fn != nil && isVMGeneric(fn.ssa) {
		return o.callGeneric(fn.ssa, in)
	}
	if fn == nil || !o.m.isInterp(fn.ssa) {
		return o.callNative(name, in)
	}
	// The arguments are converted before the shadows are suspended: a
	// pointer to a shadowed cell (the target of errors.As given to an As
	// method) then converts to the cell, not to the shadow, whose writes
	// resumeShadows would lose.
	sig := fn.ssa.Signature
	args := make([]Value, 0, 1+len(in))
	args = append(args, o.iv.V)
	for i := range in {
		args = append(args, o.m.fromNative(in[i], sig.Params().At(i).Type()))
	}
	defer o.m.resumeShadows(o.m.suspendShadows())
	defer o.m.nativeCallback(o.origin)() // a method called on a goroutine of native code
	out := o.m.unpack(o.m.Call(fn, args, nil), sig.Results(), nil)
	// Slices alias in Go: copy back what the interpreted method stored in
	// the elements of a native slice argument (e.g. driver.Rows.Next(dest)).
	for i := range in {
		if in[i].Kind() != reflect.Slice || in[i].Type().Elem().Kind() == reflect.Uint8 {
			continue
		}
		if vs, ok := args[i+1].R.([]Value); ok {
			et := sig.Params().At(i).Type().Underlying().(*types.Slice).Elem()
			for j := 0; j < len(vs) && j < in[i].Len(); j++ {
				in[i].Index(j).Set(o.m.toNative(vs[j], et))
			}
		}
	}
	return out
}

// callNative calls a method of a native named type held in VM
// representation (e.g. *json.RawMessage pointing to an interpreted field):
// the receiver is converted, and written back for a pointer receiver.
func (o vmObj) callNative(name string, in []reflect.Value) []reflect.Value {
	m, t := o.m, o.iv.T.T
	if pt, ok := t.(*types.Pointer); ok {
		if cell, ok := o.iv.V.R.(*Value); ok {
			if r, ok := cell.R.(reflect.Value); ok && r.CanAddr() {
				return r.Addr().MethodByName(name).Call(in)
			}
			et := pt.Elem()
			sh := reflect.New(m.reflectType(et))
			sh.Elem().Set(m.toNative(*cell, et))
			mv := sh.MethodByName(name)
			if !mv.IsValid() {
				panic(fmt.Sprintf("vm: type %s has no method %s", t, name))
			}
			out := mv.Call(in)
			*cell = m.fromNativeDeep(sh.Elem(), et)
			return out
		}
	}
	mv := m.toNative(o.iv.V, t).MethodByName(name)
	if !mv.IsValid() {
		panic(fmt.Sprintf("vm: type %s has no method %s", t, name))
	}
	return mv.Call(in)
}

// Format makes native fmt format a boxed value like gc formats the
// interpreted one: Error or String for the verbs that use them, the
// underlying value otherwise (without it, fmt printed the wrapper struct for
// %d, or for values boxed only for their ServeHTTP method).
func (o vmObj) Format(f fmt.State, verb rune) {
	has := func(n string) bool { _, ok := o.iv.T.Methods[n]; return ok && o.stringMethod(n) }
	switch {
	case verb == 'v' && f.Flag('#'):
		if has("GoString") {
			fmt.Fprint(f, o.call("GoString")[0].String())
			return
		}
	case verb == 'v' || verb == 's' || verb == 'x' || verb == 'X' || verb == 'q':
		for _, n := range []string{"Error", "String"} {
			if has(n) {
				fmt.Fprintf(f, fmt.FormatString(f, verb), o.call(n)[0].String())
				return
			}
		}
	}
	fmt.Fprintf(f, fmt.FormatString(f, verb), o.native().Interface())
}

// native returns the native copy of the boxed value. For a handle of
// maxBoxDepth, the copy continues the count of the bounds (segConv), and
// it panics past deepSegs of them: native code that walks the copy
// recurses on the host stack, and a value nested about 300000 times killed
// the host with a fatal stack overflow. Native fmt shows the panic of
// Format in its output (%!v(PANIC=...)).
func (o vmObj) native() reflect.Value {
	return o.m.toNativeW(o.iv.V, o.iv.T.T, o.segConv().boxed())
}

// deepSegs is the number of maxBoxDepth bounds that native code may walk
// through in one value: 2 bounds is 200000 levels on 64-bit, where the host
// stack holds about 250000 (Go itself fails near 1000000). On 32-bit, 5
// bounds is 125000 levels: native fmt on a copy failed with a fatal stack
// overflow between 150000 and 200000 levels.
const deepSegs = 2 + 3*(1-strconv.IntSize/64)

// errNestedTooDeep is the panic of a native copy past deepSegs bounds, and
// the error of json for such a value (MarshalJSON, deepWalkers).
var errNestedTooDeep = fmt.Errorf("vm: a value nested more than %d times in interfaces is too deep", deepSegs*maxBoxDepth)

// segConv returns a conversion of the boxed value that continues the count
// of maxBoxDepth bounds of a handle of the bound (see native).
func (o vmObj) segConv() *natConv {
	c := newConv(nil)
	if o.seg > 0 {
		if o.seg >= deepSegs {
			panic(errNestedTooDeep)
		}
		c.g.seg, c.g.plain = o.seg, true
	}
	return c
}

// stringMethod reports whether the method name of the boxed value has the
// signature func() string.
func (o vmObj) stringMethod(name string) bool {
	sig := o.iv.T.Methods[name].ssa.Signature
	return sig.Params().Len() == 0 && sig.Results().Len() == 1 && types.Identical(sig.Results().At(0).Type(), types.Typ[types.String])
}

// MarshalJSON makes native encoding/json encode a boxed value like the
// interpreted one: with its MarshalJSON method if it has one, as its
// underlying value otherwise (not as the wrapper struct).
func (o vmObj) MarshalJSON() (b []byte, err error) {
	if o.seg > 0 {
		// past deepSegs bounds: the error of json, not a panic
		defer func() {
			if r := recover(); r != nil {
				if r != errNestedTooDeep {
					panic(r)
				}
				b, err = nil, errNestedTooDeep
			}
		}()
	}
	if fn, ok := o.iv.T.Methods["MarshalJSON"]; ok && fn.ssa.Signature.Params().Len() == 0 && fn.ssa.Signature.Results().Len() == 2 {
		res := o.call("MarshalJSON")
		err, _ := res[1].Interface().(error)
		return res[0].Bytes(), err
	}
	return json.Marshal(o.native().Interface())
}

// jsonMarshalerW boxes values whose only known method is MarshalJSON
// (MarshalJSON and Format come from vmObj).
type jsonMarshalerW struct{ vmObj }

type stringerW struct{ vmObj }

func (w stringerW) String() string { return w.call("String")[0].String() }

type errorW struct{ vmObj }

func (w errorW) Error() string { return w.call("Error")[0].String() }

type errStringerW struct{ vmObj }

func (w errStringerW) Error() string  { return w.call("Error")[0].String() }
func (w errStringerW) String() string { return w.call("String")[0].String() }

type handlerW struct{ vmObj }

func (w handlerW) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.call("ServeHTTP", reflect.ValueOf(&rw).Elem(), reflect.ValueOf(r))
}

// ifaceWrapper returns the generic wrapper to use for an interpreted value
// with methods passed as the non-empty native interface rt, or nil when the
// hand-written wrapper chosen by box fits (or nothing does).
func (m *Machine) ifaceWrapper(iv *Iface, rt reflect.Type) *yaegiWrapper {
	if rt.NumMethod() == 0 || len(iv.T.Methods) == 0 || m.isNativeNamed(iv.T.T) || m.vr != nil && iv.T == m.vr.typeRT {
		return nil
	}
	if p, ok := iv.T.T.(*types.Pointer); ok && m.isNativeNamed(p.Elem()) {
		return nil
	}
	if hw, ok := m.handWrapper(vmObj{m: m, iv: iv}); ok && hw.Type().Implements(rt) {
		return nil
	}
	return m.wrapperFor(iv.T, rt)
}

func (m *Machine) box(iv *Iface, c *natConv) reflect.Value {
	return m.boxAs(iv, c, (m.OpaqueAny || c != nil && c.g.aot) && (c == nil || !c.g.plain))
}

// boxAs is box, with opaque handles for values without a known wrapper
// when opaque is set (OpaqueAny, or a call of an AOT copy: aotroute.go).
func (m *Machine) boxAs(iv *Iface, c *natConv, opaque bool) reflect.Value {
	dst := decodeNone
	if c != nil {
		dst, c.dst = c.dst, decodeNone // only the argument itself, not what it holds
		if c.stores {
			// sync.Pool.Put(p), sync.Map.Store(k, v): native code keeps the
			// value and gives it back. A native copy of its graph would be
			// written back into the interpreted cells when it comes back
			// (protobuf's order.RangeFields puts a slice that reaches the
			// message infos into a pool), and is O(graph) for nothing.
			c.stores = false
			return reflect.ValueOf(vmOpaque{vmObj{m: m, iv: iv}})
		}
	}
	if b, ok := iv.V.R.([]byte); (ok || iv.V.R == nil) && opaque && types.Identical(iv.T.T, byteSlice) {
		// A []byte without a name of the program: the VM holds it as the
		// Go []byte itself (nil for a nil slice), so the native value
		// aliases it and a handle keeps nothing more. A handle hid it from
		// the type switches of native code (go/parser's case []byte:
		// "invalid source").
		return reflect.ValueOf(b)
	}
	if pt, ok := iv.T.T.(*types.Pointer); ok && opaque && iv.V.R == nil && isPlainScanElem(pt.Elem()) {
		// A nil *int, *string, *[]byte...: the typed nil pointer of Go. A
		// handle hid the type from the type switches of the AOT copies
		// (fmt.Sscan("5", p) returned "can't scan type: *int" instead of
		// the nil dereference of Go).
		return reflect.Zero(reflect.PointerTo(m.reflectType(pt.Elem())))
	}
	// (c.g.aot: a pointer to a native type that holds interfaces, *any,
	// is a native copy whose interfaces hold handles, see convGraph.aot)
	// Not when an interface is in an unexported field: the copy cannot
	// unbox a handle that reflect reads through an unexported field
	// (CanInterface is false), and printed the fields of the handle (a
	// *Machine). The whole value is then a handle, as before.
	aotIfaces := c != nil && c.g.aot && !m.mentionsInterpNamed(iv.T.T) && mentionsIface(iv.T.T, map[types.Type]bool{}) && !ifaceInUnexported(iv.T.T, false, map[types.Type]bool{})
	if pt, ok := iv.T.T.(*types.Pointer); ok && (m.isNativeNamed(pt.Elem()) || opaque && !aotIfaces && m.counterpart(pt.Elem()) != nil || isByteViewOf(iv.V)) {
		// Pointer to a native named type: hand out a native pointer that
		// aliases the VM cell.
		switch p := iv.V.R.(type) {
		case reflect.Value:
			return p
		case *byteView:
			// Plain data in real bytes (byteview.go): the real pointer, as
			// the native type of the same layout. A handle printed it as
			// <nil>.
			if p.ro {
				panic(errROToNative)
			}
			if m.isNativeNamed(pt.Elem()) || !m.isPOD(pt.Elem()) || len(iv.T.Methods) > 0 {
				break // the wrappers below keep the methods
			}
			ct := m.counterpart(pt.Elem())
			if ct == nil {
				ct = m.reflectType(pt.Elem())
			}
			if p.off+int(ct.Size()) <= len(p.b) {
				nv := reflect.NewAt(ct, unsafe.Pointer(p.ptr()))
				m.notePtr(nv, iv, iv.T.T, c)
				return nv
			}
		case *Value:
			var nv reflect.Value
			cv := p.R
			if s := ownShadow(p); s != nil {
				cv = s
			}
			switch cv := cv.(type) {
			case reflect.Value:
				if cv.CanAddr() {
					nv = cv.Addr()
				} else if opaque && isNatView(cv) {
					// a native slice view (natslice.go) is not
					// addressable: a shadow holds its header, the
					// elements stay in the native array
					nv = m.shadow(p, pt.Elem()).Addr()
				}
			case *shadowRef:
				nv = cv.rv.Addr()
			default:
				if opaque {
					nv = m.shadow(p, pt.Elem()).Addr()
				}
			}
			if nv.IsValid() {
				m.notePtr(nv, iv, iv.T.T, c)
				return nv
			}
		}
	}
	if p, ok := iv.T.T.(*types.Pointer); m.isNativeNamed(iv.T.T) || ok && m.isNativeNamed(p.Elem()) {
		nv := m.toNativeW(iv.V, iv.T.T, c) // e.g. http.HandlerFunc(closure) -> reflect.MakeFunc
		if ok && nv.Kind() == reflect.Pointer {
			m.notePtr(nv, iv, iv.T.T, c)
		}
		return nv
	}
	o := vmObj{m: m, iv: iv, origin: m.originAcct()}
	if m.vr != nil && iv.T == m.vr.typeRT {
		// VM reflect.Type: the twin's canonical descriptor (comparable natively)
		return reflect.ValueOf(iv.V.R.(*vtype).cn())
	}
	if m.twinNamed(iv.T.T) == rvNative {
		r, _ := m.twinFromVM(iv.V, rvNative)
		return r
	}
	if dst == encodeAny {
		// binary.Write(w, order, v): the encoder walks v with reflect, a
		// hand wrapper would hide its fields ("some values are not
		// fixed-sized in type vm.stringerW").
		return m.boxCopy(iv, iv.T.T, c)
	}
	if dst != decodeNone {
		if iv.T.T != nil && isPointer(iv.T.T.Underlying()) {
			// The pointer that a native decoder fills (also of a named
			// pointer type: type pi *int): it must stay a
			// pointer to the native copy, with its write-back. A hand
			// wrapper (stringerW for a type with a String method) is a
			// struct, which decoders refuse ("Unmarshal(non-pointer
			// vm.stringerW)", fmt.Sscan: "type not a pointer:
			// vm.stringerW"), and the decoder does not call String. An
			// opaque handle (-opaque) is refused too, also for a type
			// without methods.
			// A nil pointer never gets the unmarshaler wrapper: the wrapper
			// is not nil, so json would call the method with a nil receiver.
			// boxCopy gives json a nil pointer, and json returns an
			// InvalidUnmarshalError, like go run.
			if !isNilPtr(iv.V.R) {
				if w, ok := m.unmarshalerWrapper(o, dst); ok {
					return w
				}
			}
			nv := m.boxCopy(iv, iv.T.T, c)
			if dst == decodeScan {
				return nilScanDst(o, nv)
			}
			return nv
		}
		if dst == decodeScan {
			// A value with a Scan method of value receiver: fmt asks for
			// a fmt.Scanner before it asks for a pointer.
			if o.scanMethod() {
				return reflect.ValueOf(&scannerP{o})
			}
			if iv.T.T != nil {
				return notPtrScanDst(o)
			}
		}
		if _, ok := iv.T.T.Underlying().(*types.Slice); ok && dst == decodeAny {
			// binary.Read(r, order, s) fills the elements of s: a native
			// copy with its write-back, also with -opaque.
			return m.boxCopy(iv, iv.T.T, c)
		}
	}
	if len(iv.T.Methods) == 0 {
		// A native printer only reads the value: it gets a native copy, as
		// without OpaqueAny. fmt prints %T with the dynamic type of the
		// interface, which the Format method of a handle cannot change: it
		// printed "vm.vmOpaque" for a []int. Values with methods keep the
		// handle (its Format calls GoString).
		if aotIfaces {
			return m.boxCopy(iv, iv.T.T, c)
		}
		if b := basic(iv.T.T); opaque && (b == nil || types.Unalias(iv.T.T) != iv.T.T.Underlying()) {
			if c == nil || !c.g.prints {
				return reflect.ValueOf(m.opaqueOf(o))
			}
			if !c.g.bounded {
				// the copy replaces an O(1) handle: bound it
				c.g.bounded, c.g.left = true, printCopyMax
			}
		}
		return m.boxCopy(iv, iv.T.T, c)
	}
	if hw, ok := m.handWrapper(o); ok {
		return hw
	}
	if c == nil || !c.g.prints {
		if w, ok := m.optAny(iv); ok {
			return w // optwrap.go
		}
	}
	if opaque {
		return reflect.ValueOf(m.opaqueOf(o))
	}
	// No known interface: methods are lost, native code sees the underlying
	// value, with the same write-backs as a value without methods
	// (json.Unmarshal(data, &c) with a MapClaims c of golang-jwt).
	return m.boxCopy(iv, iv.T.T.Underlying(), c)
}

// opaqueOf is the opaque handle of o: two handles of equal values hold the
// same payload (optPayload), so they are equal in native code, as the
// values are in Go: pprof's Profile.Remove finds the value that Add got,
// a map of native code finds its key again.
func (m *Machine) opaqueOf(o vmObj) vmOpaque {
	o.iv = m.optPayload(o.iv)
	return vmOpaque{o}
}

// notePtr records nv, the native pointer that the pointer in iv (of type t)
// was converted to, when the call has write-backs (see ptrCopy). nv is
// either the shadow of the cell made for this call, or a pointer to native
// memory that the cell shares (a native value, or an AOT shadow that
// flushShadows writes back): then the write-back only gives back iv.
func (m *Machine) notePtr(nv reflect.Value, iv *Iface, t types.Type, c *natConv) {
	cell, ok := iv.V.R.(*Value)
	if !ok || cell == nil || c.g.wb == nil || nv.IsNil() {
		return
	}
	r := &ptrCopy{iv: iv, t: t, rt: nv.Type()}
	if sh, ok := c.g.shadows[cell]; !ok || sh.Pointer() != nv.Pointer() {
		cv := cell.R
		if s := ownShadow(cell); s != nil {
			cv = s
		}
		switch cv := cv.(type) {
		case reflect.Value:
			r.shared = cv.CanAddr() && cv.Addr().Pointer() == nv.Pointer()
		case *shadowRef:
			r.shared = cv.rv.Addr().Pointer() == nv.Pointer()
		}
		if !r.shared {
			return
		}
	}
	if c.g.ptrs == nil {
		c.g.ptrs = map[uintptr]*ptrCopy{}
	}
	c.g.ptrs[nv.Pointer()] = r
}

// boxCopy converts the dynamic value of iv, as type t, to a native copy for
// box. A pointer is copied in and out (write-back), and is remembered as the
// shadow of the interpreted one, so that it comes back with its type and
// identity (sync.Pool's New, context values, list elements...). When the
// value has no native copy, the result is an opaque handle.
func (m *Machine) boxCopy(iv *Iface, t types.Type, c *natConv) reflect.Value {
	if c.g.depth >= maxBoxDepth {
		return reflect.ValueOf(vmOpaque{vmObj{m: m, iv: iv, seg: c.g.seg + 1}})
	}
	c.g.depth++
	defer func() { c.g.depth-- }()
	cc := c
	if _, ok := t.Underlying().(*types.Pointer); !ok && c.wb != nil {
		// a value in an interface argument: native code reads it
		// (fmt.Println, json.Marshal), only a slice keeps its write-back
		// (binary.Read(r, order, s)). An interface reached through a
		// pointer keeps c: its map or slice keeps its identity.
		_, isMap := t.Underlying().(*types.Map)
		cc = &natConv{wb: c.wb, g: c.g, inAny: isMap}
	}
	cc = cc.boxed()
	nv, ok := m.tryNativeConv(cc, func() reflect.Value { return m.toNativeW(iv.V, t, cc) }) // pointer: copy-in / copy-out
	if !ok {
		return reflect.ValueOf(vmOpaque{vmObj{m: m, iv: iv}})
	}
	if cc.keeps() && !cc.inAny {
		m.noteBoxed(nv, iv, cc)
	}
	if nv.Kind() == reflect.Func && c.g.funcs != nil {
		if w, ok := funcWord(nv); ok {
			if r, ok := c.g.funcs[w]; ok {
				r.iv = iv // the func of iv: it gives back iv
			}
		}
	}
	if _, ok := t.Underlying().(*types.Pointer); ok && nv.Kind() == reflect.Pointer {
		m.notePtr(nv, iv, t, c)
		if m.mentionsInterpNamed(iv.T.T) {
			m.rememberShadow(nv, iv)
		}
		return nv
	}
	if !c.g.prints && m.mentionsInterpNamed(iv.T.T) {
		if bv, ok := m.rememberValue(nv, iv); ok {
			return bv
		}
		if nv.Type().Size() == 0 {
			return m.zeroSizeHandle(iv)
		}
	}
	return nv
}

// zeroSizeHandle returns the opaque handle that native code gets for a
// zero-size value of a plugin type (type Empty struct{}, [0]int) in an
// interface. Its native copy (struct {}) cannot be told from the copies of
// other zero-size types: rememberValue cannot find it again, and sync.Pool's
// New returning Empty{} gave a struct {} to Get, where Get().(Empty)
// failed. The handle comes back as the plugin value, with its type. There
// is one handle per type and machine: two zero-size values of the same type
// are equal in Go, so native code that compares them (a map key) finds them
// equal, and values of two types are not. Native printers print it as the
// value (vmObj.Format).
func (m *Machine) zeroSizeHandle(iv *Iface) reflect.Value {
	if h, ok := m.zeroHandles.Load(iv.T); ok {
		return h.(reflect.Value)
	}
	h := reflect.ValueOf(vmOpaque{vmObj{m: m, iv: &Iface{T: iv.T, V: iv.V}}})
	actual, _ := m.zeroHandles.LoadOrStore(iv.T, h)
	return actual.(reflect.Value)
}

// valShadow is the interpreted interface value that a native interface
// value was made from (see rememberValue).
type valShadow struct {
	w  weak.Pointer[byte]
	rt reflect.Type
	iv *Iface
}

// rememberValue is rememberShadow for a value that is not a pointer: a
// struct, array, slice, map or basic value of a plugin type (a generic
// instantiation, a local or unexported type) that native code gets in an
// interface. Its native copy has an unnamed or other type, so without this
// it came back with that type (sync.Pool's New returning Box[int]{} gave a
// struct { V int } to Get). Go never changes the value in an interface,
// so the data word of the native interface value identifies the value:
// rememberValue puts the copy in a new allocation of its own, records that
// word, and returns a value whose interface will keep it. ifaceVMs gives
// back iv when native code returns that interface value. ok is false when
// the word cannot identify the value (a zero-size value shares its word,
// a pointer-shaped value is its own word).
func (m *Machine) rememberValue(nv reflect.Value, iv *Iface) (reflect.Value, bool) {
	rt := nv.Type()
	var x any
	switch {
	case !nv.CanInterface() || rt.Size() == 0:
		return nv, false
	case rt.Kind() == reflect.Map:
		// The word is the map itself. It identifies the value only when
		// the map is the copy made for this call of an interpreted map: a
		// native map that the plugin holds in a map type of its own
		// (P(params), params from mime.ParseMediaType) is shared with
		// native code, which can give it back as another value, of the
		// unnamed type. Such a map comes back unnamed, as before.
		if _, ok := iv.V.R.(*MapV); !ok || nv.IsNil() {
			return nv, false
		}
		x = nv.Interface()
	case directIface(rt):
		return nv, false
	case nv.CanAddr():
		x = nv.Interface() // a copy in a new allocation
	default:
		cp := reflect.New(rt).Elem()
		cp.Set(nv)
		x = cp.Interface()
	}
	p := (*byte)(efaceWord(&x))
	k := uintptr(unsafe.Pointer(p))
	m.valShadows.Store(k, valShadow{w: weak.Make(p), rt: rt, iv: iv})
	runtime.AddCleanup(p, func(k uintptr) {
		if r, ok := m.valShadows.Load(k); ok && r.(valShadow).w.Value() == nil {
			m.valShadows.CompareAndDelete(k, r)
		}
	}, k)
	// not addressable: reflect keeps the same word when it puts the value
	// in an interface (packEface)
	return reflect.ValueOf(x), true
}

// valueShadowOf returns the interpreted interface value that e, the
// dynamic value of a native interface, was made from (rememberValue), or
// nil.
func (m *Machine) valueShadowOf(e reflect.Value) *Iface {
	switch e.Kind() {
	case reflect.Invalid, reflect.Pointer, reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Interface:
		return nil
	case reflect.Map:
		if e.IsNil() {
			return nil
		}
	default:
		// A value read from an interface is not addressable, and its
		// pointer is the data word of the interface.
		if e.CanAddr() || e.Type().Size() == 0 || directIface(e.Type()) {
			return nil
		}
	}
	if !e.CanInterface() {
		return nil
	}
	x := e.Interface()
	p := (*byte)(efaceWord(&x))
	r, ok := m.valShadows.Load(uintptr(unsafe.Pointer(p)))
	if !ok {
		return nil
	}
	s := r.(valShadow)
	if s.rt != e.Type() || s.w.Value() != p {
		return nil
	}
	return s.iv
}

// efaceWord returns the data word of the interface value *x.
func efaceWord(x *any) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(x))[1]
}

// tryNative runs a conversion to a native copy, and reports false when the
// value cannot be copied (an interpreted channel or a value of an
// unsupported kind somewhere in its graph). box then hands out an opaque
// handle instead: native code that only stores the value (sync.Pool,
// sync.Map, contexts) gives it back intact, which is better than failing.
func (m *Machine) tryNative(f func() reflect.Value) (rv reflect.Value, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isUnsup := r.(*CompileError); !isUnsup {
				panic(r)
			}
			ok = false
		}
	}()
	return f(), true
}

// tryNativeConv is tryNative for a conversion that shares the graph and the
// write-backs of c: on failure, the shadows and write-backs the partial
// conversion recorded are dropped, so that another value of the same call
// pointing to the same cells does not find a half-built shadow.
func (m *Machine) tryNativeConv(c *natConv, f func() reflect.Value) (reflect.Value, bool) {
	g := *c.g
	// open is not cloned: the conversions that fail remove their own
	// entries, and the outer ones are still running.
	saved := g
	saved.shadows, saved.cells, saved.back = maps.Clone(g.shadows), maps.Clone(g.cells), maps.Clone(g.back)
	saved.maps, saved.nmaps, saved.slices = maps.Clone(g.maps), maps.Clone(g.nmaps), maps.Clone(g.slices)
	saved.ushadows, saved.ptrs, saved.snaps = maps.Clone(g.ushadows), maps.Clone(g.ptrs), maps.Clone(g.snaps)
	saved.shared = maps.Clone(g.shared)
	nwb := -1
	if c.wb != nil {
		nwb = len(*c.wb)
	}
	rv, ok := m.tryNative(func() reflect.Value {
		if g.bounded {
			// the clones above are work too: without this, values
			// nested in interfaces cost n*n
			c.g.left -= len(g.shadows) + len(g.slices) + len(g.maps) + len(g.ptrs) + len(g.snaps)
			c.g.use(0)
		}
		return f()
	})
	if !ok {
		left := c.g.left
		*c.g = saved
		c.g.left = left // the work done is not given back
		if nwb >= 0 {
			*c.wb = (*c.wb)[:nwb]
		}
	}
	return rv, ok
}

// ptrShadow is the interpreted pointer a native shadow was made from.
type ptrShadow struct {
	w  weak.Pointer[byte]
	iv *Iface
}

// rememberShadow records that the native pointer nv is the shadow of the
// interpreted pointer iv, passed to native code as an interface (an element
// of container/list, a context value...): when native code gives it back,
// interpreted code gets its own pointer again, with its type and identity.
// The entry goes away with the shadow.
func (m *Machine) rememberShadow(nv reflect.Value, iv *Iface) {
	p := (*byte)(nv.UnsafePointer())
	if p == nil {
		return
	}
	k := uintptr(unsafe.Pointer(p))
	m.ptrShadows.Store(k, ptrShadow{w: weak.Make(p), iv: iv})
	runtime.AddCleanup(p, func(k uintptr) {
		if r, ok := m.ptrShadows.Load(k); ok && r.(ptrShadow).w.Value() == nil {
			m.ptrShadows.CompareAndDelete(k, r)
		}
	}, k)
}

// shadowOf returns the interpreted pointer whose shadow is rv, or nil.
func (m *Machine) shadowOf(rv reflect.Value) *Iface {
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil
	}
	p := (*byte)(rv.UnsafePointer())
	r, ok := m.ptrShadows.Load(uintptr(unsafe.Pointer(p)))
	if !ok || r.(ptrShadow).w.Value() != p {
		return nil
	}
	return r.(ptrShadow).iv
}

// handWrapper returns the hand-written wrapper matching the method set of
// o's dynamic type, if any.
func (m *Machine) handWrapper(o vmObj) (reflect.Value, bool) {
	has := func(n string) bool { _, ok := o.iv.T.Methods[n]; return ok }
	switch {
	case has("ServeHTTP"):
		return reflect.ValueOf(handlerW{o}), true
	case has("Error"):
		if w, ok := m.optError(o); ok {
			return w, true
		}
		if has("String") {
			return reflect.ValueOf(errStringerW{o}), true
		}
		return reflect.ValueOf(errorW{o}), true
	case has("LogValue"):
		// before String: slog.LogValuer (optwrap.go)
		if w, ok := m.optLogValuer(o); ok {
			return w, true
		}
		if has("String") {
			return reflect.ValueOf(stringerW{o}), true
		}
		if has("MarshalJSON") {
			return reflect.ValueOf(jsonMarshalerW{o}), true
		}
	case has("String"):
		return reflect.ValueOf(stringerW{o}), true
	case has("MarshalJSON"):
		return reflect.ValueOf(jsonMarshalerW{o}), true
	}
	return reflect.Value{}, false
}

// vmOpaque carries an interpreted value through native code that only stores
// it (sync.Pool, sync.Map, context values): fromNative unwraps it.
type vmOpaque struct{ vmObj }

// plainFuncs lists, by the name of their ssa function, the native functions
// that look at their `any` arguments with reflect and call or keep them as
// values of their kind: they get native copies and native funcs, not opaque
// handles (OpaqueAny, -aot). testing/quick.Check said "argument is not a
// function", and testing.F.Add refused a vm.vmOpaque.
var plainFuncs = map[string]bool{
	"testing/quick.Check":      true,
	"testing/quick.CheckEqual": true,
	"(*testing.F).Fuzz":        true,
	"(*testing.F).Add":         true,
}

// printFuncs lists, by the name of their ssa function, the native functions
// that only print their `any` arguments (see convGraph.prints).
var printFuncs = map[string]bool{
	"fmt.Print": true, "fmt.Println": true, "fmt.Printf": true,
	"fmt.Sprint": true, "fmt.Sprintln": true, "fmt.Sprintf": true,
	"fmt.Fprint": true, "fmt.Fprintln": true, "fmt.Fprintf": true,
	"fmt.Append": true, "fmt.Appendln": true, "fmt.Appendf": true,
	"fmt.Errorf": true,
	"log.Print":  true, "log.Println": true, "log.Printf": true,
	"log.Fatal": true, "log.Fatalln": true, "log.Fatalf": true,
	"log.Panic": true, "log.Panicln": true, "log.Panicf": true,
	"(*log.Logger).Print": true, "(*log.Logger).Println": true, "(*log.Logger).Printf": true,
	"(*log.Logger).Fatal": true, "(*log.Logger).Fatalln": true, "(*log.Logger).Fatalf": true,
	"(*log.Logger).Panic": true, "(*log.Logger).Panicln": true, "(*log.Logger).Panicf": true,
}

// storeFuncs lists, by the name of their ssa function, the native methods
// that only store an `any` argument and give it back unchanged, with the
// index of that argument (the receiver is 0): it gets an opaque handle
// (see natConv.stores). CompareAndSwap is not listed: it compares the old
// value with ==, which a handle does not keep.
var storeFuncs = map[string]int{
	"(*sync.Pool).Put":           1,
	"(*sync.Map).Store":          2,
	"(*sync.Map).LoadOrStore":    2,
	"(*sync.Map).Swap":           2,
	"(*sync/atomic.Value).Store": 1,
	"(*sync/atomic.Value).Swap":  1,
}

// printMethods is printFuncs for the methods of native values called
// through an interface.
var printMethods = func() map[methodKey]bool {
	r := map[methodKey]bool{}
	for _, n := range []string{"Print", "Println", "Printf", "Fatal", "Fatalln", "Fatalf", "Panic", "Panicln", "Panicf"} {
		r[methodKey{reflect.TypeFor[*log.Logger](), n}] = true
	}
	return r
}()

// printFuncPtrs is printFuncs for the functions of printFuncs called as func
// values (f := fmt.Sprintf; f(...)), by code pointer.
var printFuncPtrs = func() map[uintptr]bool {
	r := map[uintptr]bool{}
	for _, f := range []any{
		fmt.Print, fmt.Println, fmt.Printf, fmt.Sprint, fmt.Sprintln, fmt.Sprintf,
		fmt.Fprint, fmt.Fprintln, fmt.Fprintf, fmt.Append, fmt.Appendln, fmt.Appendf, fmt.Errorf,
		log.Print, log.Println, log.Printf, log.Fatal, log.Fatalln, log.Fatalf, log.Panic, log.Panicln, log.Panicf,
	} {
		r[reflect.ValueOf(f).Pointer()] = true
	}
	return r
}()
