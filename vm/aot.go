package vm

// Runtime support of packages compiled ahead of time against the VM value
// model (cmd/aotgen, aot/...), and of native symbol tables of non-standard
// packages:
//
//   - per import path execution mode (Mode): native, aot or interpreted;
//   - symbol tables (Package) registered by generated code;
//   - native hooks (RegisterNative): direct calls of AOT functions and
//     methods from interpreted code, without reflect.Call;
//   - interface wrappers (RegisterWrapper): native types implementing an
//     interface by calling the methods of an interpreted value;
//   - helpers called by the rewritten AOT code (AssertI, TS, EqualAny) and by
//     the generated hooks (Arg, Ret, Tuple, WriteBack).

import (
	"fmt"
	"go/types"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/tools/go/ssa"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// Mode is how the VM runs the code of an import path.
type Mode int

const (
	// ModeDefault: standard library packages are native, other packages are
	// interpreted; with AOT enabled, registered AOT packages are AOT.
	ModeDefault Mode = iota
	// ModeNative calls the package natively (stdlib: yaegi symbol tables;
	// other packages: tables registered with RegisterNativePackage).
	ModeNative
	// ModeAOT calls the copy of the package compiled against the VM value
	// model (registered with RegisterAOT).
	ModeAOT
	// ModeInterp interprets the package from its sources.
	ModeInterp
)

func (md Mode) String() string {
	switch md {
	case ModeNative:
		return "native"
	case ModeAOT:
		return "aot"
	case ModeInterp:
		return "interp"
	}
	return "default"
}

// ParseMode parses "native", "aot" or "interp".
func ParseMode(s string) (Mode, error) {
	switch s {
	case "native":
		return ModeNative, nil
	case "aot":
		return ModeAOT, nil
	case "interp", "interpreted", "src":
		return ModeInterp, nil
	}
	return ModeDefault, fmt.Errorf("unknown mode %q (want native, aot or interp)", s)
}

// ParseModes parses "path=mode,path=mode".
func ParseModes(s string) (map[string]Mode, error) {
	out := map[string]Mode{}
	for _, kv := range strings.Split(s, ",") {
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("bad mode %q: want path=mode", kv)
		}
		md, err := ParseMode(v)
		if err != nil {
			return nil, err
		}
		out[k] = md
	}
	return out, nil
}

// Package is the symbol table of a package compiled into the host.
type Package struct {
	Path    string                   // import path of the original package
	AOTPath string                   // import path of the generated AOT copy ("" for a native table)
	Funcs   map[string]reflect.Value // exported functions
	Types   map[string]reflect.Type  // exported named types
	Vars    map[string]reflect.Value // pointers to the exported variables
	// Wrappers are the interface wrappers of a native table, by interface
	// name: structs in the format of the stdsyms wrappers (an IValue field,
	// then a W<Method> func field per method). An interpreted value given to
	// native code as one of these interfaces is boxed in its wrapper.
	Wrappers map[string]reflect.Type
	// GoVersion is the Go version the AOT copy was made with ("go1.26.8",
	// set by cmd/aotgen): Load and RunMain compare it with the host's in
	// AOT mode (see aotversion.go). Empty: not checked.
	GoVersion string

	// Module and Version are the module the AOT copy of a non-standard
	// package was generated from, and Sources the srchash.Dir of each
	// non-standard package the copy was built from (the package itself
	// and its dependencies). A plugin uses the copy only when it has the
	// same module version and the same sources (see aotsource.go).
	Module  string
	Version string
	Sources map[string]string
}

var (
	regMu   sync.RWMutex
	aotPkgs = map[string]*Package{} // original path -> AOT table
	natPkgs = map[string]*Package{} // original path -> native table (non-std)
	aotOrig = map[string]string{}   // AOT path -> original path
)

// RegisterAOT registers the AOT copy of a package (called by generated code).
func RegisterAOT(p *Package) {
	regMu.Lock()
	defer regMu.Unlock()
	aotPkgs[p.Path] = p
	aotOrig[p.AOTPath] = p.Path
}

// RegisterNativePackage registers the native symbol table of a non-standard
// package (called by generated code, cmd/aotgen -native).
func RegisterNativePackage(p *Package) {
	regMu.Lock()
	defer regMu.Unlock()
	natPkgs[p.Path] = p
}

// RegisterAOTPath maps the import path of a generated copy that has no
// symbol table (internal packages copied by aotgen) to its original path.
func RegisterAOTPath(aotPath, orig string) {
	regMu.Lock()
	defer regMu.Unlock()
	aotOrig[aotPath] = orig
}

// AOTPackages lists the original import paths with a registered AOT copy.
func AOTPackages() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	var out []string
	for p := range aotPkgs {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func aotPkg(path string) *Package {
	regMu.RLock()
	defer regMu.RUnlock()
	return aotPkgs[path]
}

func natPkg(path string) *Package {
	regMu.RLock()
	defer regMu.RUnlock()
	return natPkgs[path]
}

// origPath maps the package path of a native type of an AOT copy to the
// import path of the original package.
func origPath(path string) string {
	regMu.RLock()
	defer regMu.RUnlock()
	if p, ok := aotOrig[path]; ok {
		return p
	}
	return path
}

// isStdPath reports whether path is a standard library package. It uses the
// list of the release (stdtypes.IsStd), not "no dot in the first path
// element": the packages of a main module "test" (test/a) are not standard
// and have no native registry entry, so they must be interpreted.
func isStdPath(path string) bool { return stdtypes.IsStd(path) }

// ModeFor resolves the mode of path for the given explicit modes and AOT flag.
// It does not check the version of the AOT copy: the loaders do, and give
// ModeInterp in modes to the packages whose copy does not match the plugin
// (see aotFallbacks).
func ModeFor(modes map[string]Mode, aot bool, path string) Mode {
	if md, ok := modes[path]; ok && md != ModeDefault {
		if md == ModeAOT && aotPkg(path) == nil {
			return ModeFor(nil, false, path)
		}
		return md
	}
	if md, ok := familyMode(modes, path); ok {
		return md
	}
	if aot && aotPkg(path) != nil {
		return ModeAOT
	}
	if isStdPath(path) {
		return ModeNative
	}
	return ModeInterp
}

// jsonFamily are the packages that share the types of
// encoding/json/internal/jsonopts in their API (json.Options is
// jsonopts.Options): a copy and a native package of the family cannot be
// mixed. jsontext.WithIndent of a native jsontext gave a native option to
// the copy of json/v2 (a reflect.Set panic).
var jsonFamily = []string{"encoding/json/v2", "encoding/json/jsontext"}

// familyMode returns, for a package of jsonFamily without an explicit mode,
// the explicit mode of another package of the family: -mode
// encoding/json/jsontext=native keeps json/v2 native too, with -aot and
// with the AOT triggers of default mode, and -mode encoding/json/v2=aot
// runs jsontext as a copy.
func familyMode(modes map[string]Mode, path string) (Mode, bool) {
	if !slices.Contains(jsonFamily, path) {
		return 0, false
	}
	for _, q := range jsonFamily {
		if md, ok := modes[q]; ok && q != path && md != ModeDefault && (md != ModeAOT || aotPkg(path) != nil) {
			return md, true
		}
	}
	return 0, false
}

// stdAOTTriggers are the standard packages whose use makes a plugin in
// default mode run the standard library as in AOT mode. Their API takes the
// values of the plugin in methods (Template.Execute, Logger.Info) and in
// values of their own types (slog.Any), and looks at them with reflect: the
// native package sees a native copy of the underlying type, without the
// methods and the names of the plugin types. A template that calls a
// method of the data failed, and slog ignored a LogValuer and the json tags
// of a struct. aotroute.go only routes functions whose arguments are plain
// `any` values: a method of a native value, or a native value that holds
// the plugin value (slog.Attr), cannot go to a copy, which has other types.
// encoding/gob is used through Encoder and Decoder methods, and native gob
// ignored GobEncode and MarshalBinary of plugin types (other bytes than Go,
// with no error). encoding/xml compares types with its own (xml.Name,
// xml.Attr): a value that holds them is not routed to the copy, and native
// xml ignored MarshalXML and MarshalXMLAttr there. encoding/json/v2 (Go
// 1.27) takes the plugin's types in its generic functions (MarshalFunc[T]
// keys on the type T, aotgeneric.go), and native v2 ignored MarshalJSONTo
// and the names of the plugin's types. encoding/json/jsontext alone is not
// a trigger: it does not look at the plugin's types, and its generic
// functions have native instances (bytesGeneric); with json/v2 it runs as
// a copy too (jsonFamily). flag takes the values of the plugin in methods
// (FlagSet.Var, TextVar) and looks at them with reflect: TextVar panicked
// ("variable value type must be a pointer"), and the usage made a zero
// value of the wrapper to find the default (isZeroValue). It is a trigger
// only for a machine with the real standard library: a restricted machine
// has its own flag.CommandLine, which the copy cannot give (stdAOTModes).
// database/sql
// takes the driver of the plugin (sql.Register, OpenDB) and its values
// (Exec args, Scan destinations) and looks for their optional methods
// (driver.QueryerContext, SessionResetter, Valuer, Scanner...) with type
// assertions and reflect: the native package called the driver without
// them and converted the values with their underlying types. net/rpc finds
// the methods of the service of the plugin with reflect (Register): the
// native package found none.
var stdAOTTriggers = map[string]bool{
	"text/template":    true,
	"html/template":    true,
	"log/slog":         true,
	"encoding/gob":     true,
	"encoding/xml":     true,
	"encoding/json/v2": true,
	"flag":             true,
	"database/sql":     true,
	"net/rpc":          true,
}

// stdAOTModes returns modes with ModeAOT for every standard package that
// has a registered AOT copy, in default mode (aot false), when the source
// packages src import one of stdAOTTriggers: the standard library then runs
// as in AOT mode (the copies call each other: html/template.FuncMap is the
// FuncMap of the copy of text/template, slog formats with the copy of fmt),
// and the packages of the plugin's modules stay interpreted. The copies
// must have been made for the Go release of the host (hostGo; allowStale
// as Options.AllowStaleAOT); a trigger whose mode is set explicitly does
// not count, and an explicit mode is kept. A standard package interpreted
// from its source (fromSrc: Options.Src, InterpretedStd) is neither a
// trigger (the VM reflect sees the plugin types) nor changed. As with
// Options.Modes, a package in ModeAOT turns on the opaque handles for
// native `any` parameters (Options.OpaqueAny), which the AOT copies need.
// modes itself is not changed.
//
// ownFlags is set for a machine with its own state of package flag (not
// realStd: flagState in exit.go). flag then stays native, and is not a
// trigger: the copy of flag has one flag.CommandLine for the process, so
// two plugins (or two instances of one plugin) defined their flags in the
// same flag set, and the second flag.Bool("x", ...) panicked with "flag
// redefined". In AOT mode too (aot), flag is native for such a machine,
// unless its mode is set explicitly. The cost: FlagSet.TextVar with a type
// of the plugin panics there in every mode ("variable value type must be
// a pointer"), as it did in default mode before.
func stdAOTModes(modes map[string]Mode, aot bool, src []sourcePkg, hostGo string, allowStale bool, fromSrc func(string) bool, ownFlags bool) map[string]Mode {
	if aot {
		if !ownFlags || modes["flag"] != ModeDefault || fromSrc("flag") {
			return modes
		}
		out := make(map[string]Mode, len(modes)+1)
		for k, v := range modes {
			out[k] = v
		}
		out["flag"] = ModeNative
		return out
	}
	trigger := false
	for _, sp := range src {
		for _, imp := range sp.types.Imports() {
			if p := imp.Path(); stdAOTTriggers[p] && !(ownFlags && p == "flag") && modes[p] == ModeDefault && !fromSrc(p) && aotPkg(p) != nil {
				if md, pinned := familyMode(modes, p); pinned && md != ModeAOT {
					continue // json/v2 with jsontext native: both stay native
				}
				trigger = true
			}
		}
	}
	if !trigger || checkAOTVersion(hostGo, allowStale) != nil {
		return modes // without the copies, or stale ones: the packages stay native
	}
	out := make(map[string]Mode, len(modes)+len(stdAOTTriggers))
	for k, v := range modes {
		out[k] = v
	}
	for _, path := range AOTPackages() {
		if md, pinned := familyMode(modes, path); pinned && md != ModeAOT {
			continue
		}
		if ownFlags && path == "flag" {
			continue
		}
		if isStdPath(path) && out[path] == ModeDefault && !fromSrc(path) {
			out[path] = ModeAOT
		}
	}
	return out
}

func (m *Machine) modeOf(path string) Mode { return ModeFor(m.Modes, m.AOT, path) }

// table returns the registered symbol table used for path, if any.
func (m *Machine) table(path string) *Package {
	switch m.modeOf(path) {
	case ModeAOT:
		return aotPkg(path)
	case ModeNative:
		if path == xsysUnixPath && !m.realStd {
			return m.xsysSubset() // the read-only subset (xsysunix.go)
		}
		return natPkg(path)
	}
	return nil
}

// ---- native hooks ----

// NativeHook implements a call to a non-interpreted function or method in
// Go, with direct access to VM values (no reflect.Call boundary). For a
// method, args[0] is the receiver.
type NativeHook func(m *Machine, args []Value) Value

var nativeHooks = map[string]NativeHook{}

// RegisterNative installs a hook for "pkgpath.Func", "pkgpath.T.Method" or
// "(*pkgpath.T).Method". Hooks are used for packages run in ModeAOT. It is
// safe to call while plugins compile (nativeHooks is guarded by regMu).
func RegisterNative(name string, h NativeHook) {
	regMu.Lock()
	defer regMu.Unlock()
	nativeHooks[name] = h
}

// hookName is the RegisterNative key of f.
func hookName(f *ssa.Function) string {
	recv := f.Signature.Recv()
	if recv == nil {
		return f.Pkg.Pkg.Path() + "." + f.Name()
	}
	t := recv.Type()
	ptr := false
	if p, ok := t.(*types.Pointer); ok {
		t, ptr = p.Elem(), true
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return ""
	}
	name := n.Obj().Pkg().Path() + "." + n.Obj().Name()
	if ptr {
		return "(*" + name + ")." + f.Name()
	}
	return name + "." + f.Name()
}

func (m *Machine) hookFor(f *ssa.Function) NativeHook {
	if f.Pkg == nil || m.modeOf(f.Pkg.Pkg.Path()) != ModeAOT {
		return nil
	}
	name := hookName(f)
	if h, ok := m.hookOverrides[name]; ok {
		return h
	}
	if _, ok := m.overrides[name]; ok && f.Signature.Recv() == nil {
		return nil // the machine's own function (log/slog of SetStdio): a native call to it
	}
	if vmHooksAOT[name] {
		return vmHooks[name] // sortslice.go
	}
	if f.Signature.Recv() == nil {
		if h, ok := m.hostHook(name); ok {
			return h // the machine's own registries (hostregs.go)
		}
	}
	regMu.RLock()
	defer regMu.RUnlock()
	return nativeHooks[name]
}

// Tuple builds the VM value of multiple results.
func Tuple(vs ...Value) Value { return Value{R: vs} }

// Arg converts a VM argument to the Go type T of an AOT parameter.
func Arg[T any](m *Machine, v Value) T {
	var x T
	switch p := any(&x).(type) {
	case *string:
		*p, _ = v.R.(string)
		return x
	case *int:
		*p = int(v.N)
		return x
	case *int64:
		*p = v.N
		return x
	case *bool:
		*p = v.N != 0
		return x
	case *float64:
		*p = f64(v)
		return x
	case *[]byte:
		if v.R != nil {
			*p = valBytes(v)
		}
		return x
	case *any:
		if iv, _ := v.R.(*Iface); iv != nil {
			*p = m.ifaceAny(iv)
		} else if rv, ok := v.R.(reflect.Value); ok {
			*p = rv.Interface()
		}
		return x
	}
	rv := m.argNat(v, reflect.TypeFor[T]())
	if !rv.IsValid() || rv.Kind() == reflect.Interface && rv.IsNil() {
		return x
	}
	return rv.Interface().(T)
}

// WriteBack copies the elements of a native slice argument back into the
// interpreted slice it was converted from (slices alias in Go).
func WriteBack[T any](m *Machine, v Value, x T) {
	s, ok := v.R.([]Value)
	if !ok {
		return
	}
	rv := reflect.ValueOf(x)
	for i := 0; i < len(s) && i < rv.Len(); i++ {
		s[i] = m.retVM(rv.Index(i))
	}
}

// Ret converts an AOT result of Go type T to a VM value.
func Ret[T any](m *Machine, x T) Value {
	switch p := any(&x).(type) {
	case *string:
		return Value{R: *p}
	case *int:
		return Value{N: int64(*p)}
	case *int64:
		return Value{N: *p}
	case *bool:
		return Value{N: b2i(*p)}
	case *float64:
		return Value{N: fv(*p)}
	case *[]byte:
		return BytesVal(*p)
	case *error:
		return m.ErrorVal(*p)
	}
	return m.retVM(reflect.ValueOf(&x).Elem())
}

// BytesOf converts an interpreted []byte to Go.
func BytesOf(v Value) []byte { return valBytes(v) }

// BytesVal converts a Go []byte to an interpreted []byte.
func BytesVal(b []byte) Value {
	if b == nil {
		return Value{}
	}
	if optBytes {
		return Value{R: b}
	}
	s := make([]Value, len(b))
	for i := range b {
		s[i].N = int64(b[i])
	}
	return Value{R: s}
}

// ErrorVal converts a native error to an interpreted error value.
func (m *Machine) ErrorVal(err error) Value {
	if err == nil {
		return Value{}
	}
	return m.fromNative(reflect.ValueOf(&err).Elem(), types.Universe.Lookup("error").Type())
}

// ---- interface wrappers ----

var aotWrappers sync.Map // reflect.Type (interface) -> func(vmObj) any

// Obj is the interpreted value embedded in generated interface wrappers.
// It is not a vmObj: the methods of vmObj for native printers and
// encoders (Format, MarshalJSON) would be methods of every wrapper, and a
// type switch of AOT code took a wrapper made for case fmt.Stringer for an
// earlier case json.Marshaler (html/template printed a plugin Stringer as
// JSON).
type Obj struct{ o vmObj }

// Call calls the method name of the interpreted value.
func (o Obj) Call(name string, in ...reflect.Value) []reflect.Value { return o.o.call(name, in...) }

func (o Obj) iface() *Iface { return o.o.iv }
func (o Obj) obj() vmObj    { return o.o }

// Call calls the method name of the interpreted value.
func (o vmObj) Call(name string, in ...reflect.Value) []reflect.Value { return o.call(name, in...) }

// RegisterWrapper registers the wrapper of interface type I: a native type
// implementing I by calling the methods of an interpreted value. The first
// registration of an interface wins.
func RegisterWrapper[I any](f func(Obj) I) {
	rt := reflect.TypeFor[I]()
	if rt.Kind() != reflect.Interface {
		panic("vm.RegisterWrapper: not an interface: " + rt.String())
	}
	aotWrappers.LoadOrStore(rt, func(o vmObj) any { return f(Obj{o}) })
}

func wrapperFor(rt reflect.Type) func(vmObj) any {
	if w, ok := aotWrappers.Load(rt); ok {
		return w.(func(vmObj) any)
	}
	return nil
}

// In passes an argument of a wrapper method to the interpreted method.
func In[T any](p *T) reflect.Value { return reflect.ValueOf(p).Elem() }

// Out extracts result i of an interpreted method called by a wrapper.
func Out[T any](out []reflect.Value, i int) T {
	var x T
	if i >= len(out) || !out[i].IsValid() {
		return x
	}
	if out[i].Kind() == reflect.Interface && out[i].IsNil() {
		return x
	}
	x, _ = out[i].Interface().(T)
	return x
}

// implementsNat reports whether the interpreted type t implements the native
// interface type rt.
func (m *Machine) implementsNat(t *RType, rt reflect.Type) bool {
	if rt.NumMethod() == 0 {
		return true
	}
	if t.T == nil {
		return t.Native.Implements(rt)
	}
	if it := m.vr.resolveIface(rt); it != nil {
		return m.implements(t, it)
	}
	return false
}

// hostImpl caches hostImplements: hostImplKey -> bool.
var hostImpl sync.Map

type hostImplKey struct{ iface, dyn reflect.Type }

// hostWrapper returns x, a value of the host, in the wrapper of the AOT
// interface rt when x implements the host's form of rt: the methods of an
// AOT copy name the copy's types. A *big.Int has Format(fmt.State, rune),
// and the copy of fmt asks for Format(its own State, rune): fmt printed the
// fields of the struct instead of calling Format.
func hostWrapper(rt reflect.Type, x any) (any, bool) {
	if x == nil || rt.NumMethod() == 0 {
		return nil, false
	}
	dt := reflect.TypeOf(x)
	k := hostImplKey{rt, dt}
	ok, found := hostImpl.Load(k)
	if !found {
		ok, _ = hostImpl.LoadOrStore(k, hostImplements(rt, dt))
	}
	if !ok.(bool) {
		return nil, false
	}
	w := wrapperFor(rt)
	if w == nil {
		return nil, false
	}
	return w(hostObj(reflect.ValueOf(x))), true
}

// hostImplements reports whether dt has the methods of the interface rt,
// up to an interface parameter of the copy that has the methods of the
// host's parameter (fmt.State), and a result of the host that is
// assignable to the copy's.
func hostImplements(rt, dt reflect.Type) bool {
	for i := range rt.NumMethod() {
		im := rt.Method(i)
		dm, ok := dt.MethodByName(im.Name)
		if !ok || im.Type.NumIn() != dm.Type.NumIn()-1 || im.Type.NumOut() != dm.Type.NumOut() || im.Type.IsVariadic() != dm.Type.IsVariadic() {
			return false
		}
		for j := range im.Type.NumIn() {
			ct, ht := im.Type.In(j), dm.Type.In(j+1)
			if ct != ht && (ct.Kind() != reflect.Interface || ht.Kind() != reflect.Interface || !ct.Implements(ht)) {
				return false
			}
		}
		for j := range im.Type.NumOut() {
			if !dm.Type.Out(j).AssignableTo(im.Type.Out(j)) {
				return false
			}
		}
	}
	return true
}

// AssertI implements x.(I) in AOT code for an interface type I: an
// interpreted value implementing I is returned as I's wrapper.
func AssertI[I any](x any) (I, bool) {
	// The handler that Object.Interface gives the host hides the other methods
	// of the plugin value (GetTracingInformation of a Traefik middleware).
	if t, ok := x.(topHandler); ok {
		if r, ok := x.(I); ok {
			return r, true
		}
		if _, serves := reflect.TypeFor[I]().MethodByName("ServeHTTP"); serves {
			// the ServeHTTP of the handler behind would skip the budget,
			// the cancel and the recover of topHandler
			var zero I
			return zero, false
		}
		x = t.h
	}
	b, ok := x.(vmBoxed)
	if !ok {
		r, ok := x.(I)
		if !ok {
			// a json.Number of the host is the copy's (copyValTypeOf)
			if c, isVal := copyValAny(x); isVal {
				r, ok = c.(I)
			}
		}
		if !ok {
			if w, isHost := hostWrapper(reflect.TypeFor[I](), x); isHost {
				r, ok = w.(I)
			}
		}
		return r, ok
	}
	var zero I
	rt := reflect.TypeFor[I]()
	o := b.obj()
	if !o.m.implementsNat(o.iv.T, rt) {
		return zero, false
	}
	if r, ok := x.(I); ok {
		return r, true
	}
	w := wrapperFor(rt)
	if w == nil {
		unsupported("vm: no AOT wrapper for interface %s (dynamic type %s)", rt, o.iv.T)
	}
	return w(o).(I), true
}

// MustAssertI is the single-result form of AssertI (panics on failure).
func MustAssertI[I any](x any) I {
	r, ok := AssertI[I](x)
	if !ok {
		panic(assertIError(x, reflect.TypeFor[I]()))
	}
	return r
}

// assertIError is the error of a failed x.(I) of AOT code, I the interface
// it: the *runtime.TypeAssertionError of gc when the dynamic type of x is
// native (slog's v.any.(LogValuer) on a Kind), or a type of the program
// with a native type of the same name (struct { A int }), else the error of
// the VM with the same message. gc has no static type here
// (runtime.assertE2I).
func assertIError(x any, it reflect.Type) error {
	e := &typeAssertError{iface: "interface", asserted: it.String()}
	var ct reflect.Type // the dynamic type for gc, nil when none
	switch b := x.(type) {
	case nil:
	case vmBoxed:
		o := b.obj()
		if o.iv.T.T == nil {
			// a native value in a box: no *_type to name
			e.concrete = o.iv.T.Native.String()
			e.missing = nativeMissing(o.iv.T.Native, it)
			break
		}
		e.concrete = typeString(o.iv.T.T)
		if ti := o.m.vr.resolveIface(it); ti != nil {
			if f, _ := types.MissingMethod(o.iv.T.T, ti, true); f != nil {
				e.missing = f.Name()
			}
		}
		nt, ok := o.m.sameNativeType(o.iv.T.T)
		if !ok {
			return e // a named type of the program: gc cannot name it
		}
		ct = nt
	default:
		ct = reflect.TypeOf(x)
		e.concrete = ct.String()
		e.missing = nativeMissing(ct, it)
	}
	if !hostAssertOK || ct == nil && e.concrete != "" {
		return e
	}
	r := new(runtime.TypeAssertionError)
	*(*typeAssertionLayout)(unsafe.Pointer(r)) = typeAssertionLayout{
		concrete: rtypePtr(ct), asserted: rtypePtr(it), missing: e.missing,
	}
	if r.Error() != e.Error() {
		return e
	}
	return r
}

// nativeMissing returns the first method of the interface it that the
// native type t does not have, as gc names it.
func nativeMissing(t, it reflect.Type) string {
	for i := 0; i < it.NumMethod(); i++ {
		im := it.Method(i)
		m, ok := t.MethodByName(im.Name)
		if !ok || m.Type.NumIn() != im.Type.NumIn()+1 {
			return im.Name
		}
	}
	return ""
}

// TSCases lists the interface cases of a type switch of AOT code, in order,
// with the wrapper of each case (it implements the case interface and the
// static type of the switched value).
type TSCases struct{ cases []tsCase }

type tsCase struct {
	rt   reflect.Type
	wrap func(Obj) any
}

// TSCase describes the case I of a type switch.
func TSCase[I any](wrap func(Obj) any) tsCase {
	return tsCase{rt: reflect.TypeFor[I](), wrap: wrap}
}

// NewTSCases builds the case list of a type switch.
func NewTSCases(cs ...tsCase) *TSCases { return &TSCases{cases: cs} }

// TSG returns the value a type switch of AOT code switches on: an
// interpreted value is replaced by the wrapper of the first interface case it
// implements, so that the native switch selects that case.
func TSG[X any](x X, c *TSCases) X {
	b, ok := any(x).(vmBoxed)
	if !ok {
		return x
	}
	o := b.obj()
	xt := reflect.TypeOf(any(x))
	for i, cs := range c.cases {
		if !o.m.implementsNat(o.iv.T, cs.rt) {
			continue
		}
		// x itself, when no earlier case matches it natively: a wrapper
		// with a String method is also a json.Marshaler (vmObj), and
		// html/template has case json.Marshaler before case fmt.Stringer
		if xt.Implements(cs.rt) && !slices.ContainsFunc(c.cases[:i], func(e tsCase) bool { return xt.Implements(e.rt) }) {
			return x
		}
		return cs.wrap(Obj{o}).(X)
	}
	// No case for the interpreted type. A handle has methods of its own
	// (vmObj: Format, MarshalJSON) that a case may match natively:
	// html/template took a fmt.Stringer of the plugin for a json.Marshaler
	// (case json.Marshaler before case fmt.Stringer). The switch then gets
	// a bare handle, with no exported method.
	for _, cs := range c.cases {
		if xt.Implements(cs.rt) {
			if bare, ok := any(vmBare{o}).(X); ok {
				return bare
			}
		}
	}
	return x
}

// vmBare carries an interpreted value through a type switch of AOT code
// that no interface case of its type matches (TSG). It has no exported
// method; Unbox takes it as an opaque handle.
type vmBare struct{ o vmObj }

func (b vmBare) iface() *Iface { return b.o.iv }
func (b vmBare) obj() vmObj    { return b.o }

// Unbox implements the operand of a type assertion or type switch of AOT
// code to an unnamed composite type (x.([]byte), case map[string]any): an
// interpreted value of such a type with a native counterpart reaches AOT
// code as an opaque handle, which is replaced by a native copy. The copy of
// a map or a slice is written back into the interpreted value when the AOT
// call returns (or calls back into interpreted code): writes through it
// reach the interpreted map or array (binary.Read into a slice), as in Go.
func Unbox(x any) any {
	o, ok := x.(vmOpaque)
	if b, isBare := x.(vmBare); isBare {
		o, ok = vmOpaque{b.o}, true
	}
	if !ok {
		return x
	}
	t := types.Unalias(o.iv.T.T)
	switch t.(type) {
	case *types.Slice, *types.Map, *types.Array, *types.Struct, *types.Signature, *types.Chan:
	default:
		return x
	}
	rt := o.m.counterpart(t)
	if rt == nil {
		return x
	}
	if ch, ok := o.iv.V.R.(chan Value); ok {
		return o.m.standIn(ch, t, rt).Interface() // a value in an any: a stand-in (chanstandin.go)
	}
	if rv, ok := o.m.unboxCopy(o.iv.V, t, rt); ok {
		return rv.Interface()
	}
	return o.m.argNat(o.iv.V, rt).Interface()
}

// NoUnbox returns x as it is. cmd/aotgen puts it in place of Unbox where
// the native copy breaks the code (rewrite.go, jsonV2Cycle): an
// interpreted map or slice stays an opaque handle, so a case
// map[string]any or []any does not match it and the code takes its reflect
// path instead, where the twin keeps the identity of the VM value
// (AddrCopy). Unbox makes a new native copy at each call: a cycle check
// keyed on the address of that copy never sees the same map or slice twice.
func NoUnbox(x any) any { return x }

// unboxCopy is the native copy of the interpreted map or slice v for Unbox,
// recorded with the shadows (shadowRef without cell) to be written back.
func (m *Machine) unboxCopy(v Value, t types.Type, rt reflect.Type) (reflect.Value, bool) {
	switch t.Underlying().(type) {
	case *types.Map:
		if _, ok := v.R.(*MapV); !ok {
			return reflect.Value{}, false
		}
	case *types.Slice:
		if _, ok := v.R.([]Value); !ok {
			return reflect.Value{}, false
		}
	default:
		return reflect.Value{}, false
	}
	if m.tryReflectType(t) != rt {
		return reflect.Value{}, false
	}
	s := &shadowRef{m: m, t: t, wb: new([]func())}
	s.c = newConv(s.wb)
	rv, ok := m.tryNativeConv(s.c, func() reflect.Value { return m.toNativeW(v, t, s.c) })
	if !ok {
		return reflect.Value{}, false
	}
	m.addShadow(s)
	return rv, true
}

// EqualAny implements == between interface values in AOT code: interpreted
// values compare by VM type and value, not by wrapper identity.
func EqualAny(a, b any) bool {
	ab, aok := a.(vmBoxed)
	bb, bok := b.(vmBoxed)
	switch {
	case aok && bok:
		x, y := ab.iface(), bb.iface()
		if x == y {
			return true
		}
		if x.T != y.T {
			return false
		}
		if !types.Comparable(x.T.T) {
			panic(runtimeError("comparing uncomparable type " + typeString(x.T.T)))
		}
		return ab.obj().m.equal(x.V, y.V, x.T.T)
	case aok || bok:
		return false
	}
	if a == b {
		return true
	}
	// json.Number of the host and json.Number of the AOT copy of json are
	// the same type for the copy (sameValType).
	at, aok := a.(*vtype)
	bt, bok := b.(*vtype)
	return aok && bok && at != nil && bt != nil && at.nat != nil && bt.nat != nil && at.kind == bt.kind && sameValType(at.nat, bt.nat)
}

// ---- interface resolution between native and VM types ----

func (vr *vmReflect) resolveIface(nt reflect.Type) *types.Interface {
	vr.m.mu.Lock()
	if vr.ifaces == nil {
		vr.ifaces = map[reflect.Type]*types.Interface{}
	}
	it, ok := vr.ifaces[nt]
	vr.m.mu.Unlock()
	if ok {
		return it
	}
	if nt.PkgPath() == "" && nt.Name() == "error" {
		it = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	} else if nt.Name() != "" {
		if tp := vr.typesPkg(origPath(nt.PkgPath())); tp != nil {
			if obj := tp.Scope().Lookup(nt.Name()); obj != nil {
				it, _ = obj.Type().Underlying().(*types.Interface)
			}
		}
	}
	if it == nil && nt.Kind() == reflect.Interface {
		it = vr.synthIface(nt) // e.g. unexported or anonymous interfaces
	}
	vr.m.mu.Lock()
	vr.ifaces[nt] = it
	vr.m.mu.Unlock()
	return it
}

// synthIface builds a go/types interface from a native one (nil when a
// method signature has no interpreted counterpart).
func (vr *vmReflect) synthIface(nt reflect.Type) *types.Interface {
	var fns []*types.Func
	for i := 0; i < nt.NumMethod(); i++ {
		mt := nt.Method(i)
		var ps, rs []*types.Var
		for j := 0; j < mt.Type.NumIn(); j++ {
			t := vr.m.goTypeOf(mt.Type.In(j))
			if t == nil {
				return nil
			}
			ps = append(ps, types.NewParam(0, nil, "", t))
		}
		for j := 0; j < mt.Type.NumOut(); j++ {
			t := vr.m.goTypeOf(mt.Type.Out(j))
			if t == nil {
				return nil
			}
			rs = append(rs, types.NewParam(0, nil, "", t))
		}
		var pkg *types.Package
		if mt.PkgPath != "" {
			pkg = vr.typesPkg(origPath(mt.PkgPath))
		}
		sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(ps...), types.NewTuple(rs...), mt.Type.IsVariadic())
		fns = append(fns, types.NewFunc(0, pkg, mt.Name, sig))
	}
	return types.NewInterfaceType(fns, nil).Complete()
}

// typesPkg finds a package of the program's import graph by path.
func (vr *vmReflect) typesPkg(path string) *types.Package {
	vr.m.mu.Lock()
	if vr.pkgs == nil {
		vr.pkgs = map[string]*types.Package{}
		seen := map[*types.Package]bool{}
		var walk func(p *types.Package)
		walk = func(p *types.Package) {
			if seen[p] {
				return
			}
			seen[p] = true
			vr.pkgs[p.Path()] = p
			for _, q := range p.Imports() {
				walk(q)
			}
		}
		for _, sp := range vr.m.prog.AllPackages() {
			walk(sp.Pkg)
		}
	}
	p := vr.pkgs[path]
	vr.m.mu.Unlock()
	return p
}
