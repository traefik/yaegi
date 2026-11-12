//go:build ignore

// natsigs_gen writes zz_natsigs_go1_NN.go, for the Go release of its
// toolchain (each release of the window of yaegi v2, gorelease.Window, has its
// file, built by that release only): a direct call (a trampoline, see
// natfast.go) for the simple signatures of the standard library, so that
// these calls need no reflect.
//
//   - Functions of every package whose parameters are predeclared basic
//     types (string, bool, numbers), []byte, []string (also ...string), or
//     one of the interfaces of ifaceParams (error, io.Reader, io.Writer,
//     context.Context), with at most maxParams parameters. A []byte is
//     shared with native code, as on the general path; a []string of the
//     VM is copied and the strings that native code changed are written
//     back (sort.Strings), as on the general path; an interface must hold
//     a value of a native type (otherwise the general path makes the
//     wrapper of the interpreted value). The results may be of any type
//     that the generated code can name.
//   - In the packages of typesPkgs, which plugins use most, also the
//     methods, and the named types: a named type with a basic underlying
//     type (time.Duration) is read from the VM register as a number or a
//     string, a struct or a pointer to a struct (time.Time, *url.URL) must
//     be a native value. When it is not (an interpreted representation of
//     the type), the trampoline takes the general path.
//
// Only the results of basic types are built directly; the others go through
// fromNative, as on the general path.
//
// It also writes genFastInvoke: for each method of the exported interfaces
// of the standard library (io.Reader, context.Context, fmt.Stringer,
// http.Flusher...) whose signature is supported, a direct call of that
// method on a native value in an interface of the plugin, through a type
// assertion of the value to an interface with only that method (no
// reflect, no runtime internals). The call site keys it by the name and
// the signature of the method (invokeKey in natfast.go).
//
// It loads the standard library for linux/amd64 whatever the host, so the
// output does not depend on the machine. A signature that some platform
// does not use only costs one case. Run it with "go generate ./vm" (it
// needs the Go toolchain).
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/traefik/yaegi/v2/internal/gorelease"
)

// maxParams bounds the size of the generated code; longer signatures take
// the general path (the receiver counts). time.Date has 8.
const maxParams = 8

// ifaceParams are the interface types that a trampoline takes as
// parameters, when the argument holds a value of a native type
// (natIfaceArg).
var ifaceParams = map[string]bool{"error": true, "io.Reader": true, "io.Writer": true, "context.Context": true}

// badResultPkgs are the packages whose types the results may not name: not
// on every platform, not importable, or with an init that changes the host
// (stdsyms/sideeffect: expvar and net/http/pprof register handlers on
// http.DefaultServeMux).
var badResultPkgs = map[string]bool{
	"syscall": true, "log/syslog": true, "plugin": true, "runtime/cgo": true, "unsafe": true,
	"expvar": true, "net/http/pprof": true, "testing/quick": true,
}

// typesPkgs are the packages whose named types and methods get
// trampolines.
var typesPkgs = map[string]bool{
	"bufio": true, "bytes": true, "encoding/base64": true, "encoding/json": true,
	"mime": true, "net": true, "net/http": true, "net/netip": true, "net/textproto": true,
	"net/url": true, "regexp": true, "strconv": true, "strings": true, "time": true,
}

// invInfo is a method of an interface: its name and signature.
type invInfo struct {
	name string
	sig  *types.Signature
}

type sigInfo struct {
	sig  *types.Signature
	recv types.Type // nil for a function
}

func main() {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes,
		Env:  append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"),
	}
	pkgs, err := packages.Load(cfg, "std")
	if err != nil {
		log.Fatal(err)
	}
	sigs := map[string]sigInfo{}
	invs := map[string]invInfo{} // interface methods, by invokeKey
	addIface := func(it *types.Interface) {
		for i := 0; i < it.NumMethods(); i++ {
			f := it.Method(i)
			sig := f.Type().(*types.Signature)
			// invokeKey never matches a variadic method
			if f.Exported() && !sig.Variadic() && supported(nil, sig) && invokeResults(sig) {
				invs[f.Name()+" "+goType(nil, sig)] = invInfo{f.Name(), sig}
			}
		}
	}
	addIface(types.Universe.Lookup("error").Type().Underlying().(*types.Interface))
	for _, p := range pkgs {
		if skipPkg(p.PkgPath) || p.Types == nil {
			continue
		}
		sc := p.Types.Scope()
		for _, name := range sc.Names() {
			switch o := sc.Lookup(name).(type) {
			case *types.Func:
				sig := o.Type().(*types.Signature)
				if o.Exported() && supported(nil, sig) {
					sigs[goType(nil, sig)] = sigInfo{sig, nil}
				}
			case *types.TypeName:
				if it, ok := o.Type().Underlying().(*types.Interface); ok && o.Exported() && !o.IsAlias() {
					if nt, ok := o.Type().(*types.Named); ok && nt.TypeParams().Len() == 0 {
						addIface(it)
					}
				}
				if !typesPkgs[p.PkgPath] || !o.Exported() || o.IsAlias() {
					continue
				}
				nt, ok := o.Type().(*types.Named)
				if !ok || nt.TypeParams().Len() > 0 {
					continue
				}
				if _, ok := nt.Underlying().(*types.Interface); ok {
					continue
				}
				// Each method once, with the receiver of its call: T for
				// the method set of T, *T for the methods only *T has.
				vms := types.NewMethodSet(nt)
				for _, recv := range []types.Type{nt, types.NewPointer(nt)} {
					ms := types.NewMethodSet(recv)
					for i := 0; i < ms.Len(); i++ {
						f := ms.At(i).Obj().(*types.Func)
						if !f.Exported() {
							continue
						}
						if _, ptr := recv.(*types.Pointer); ptr && vms.Lookup(f.Pkg(), f.Name()) != nil {
							continue
						}
						sig := f.Type().(*types.Signature)
						if supported(recv, sig) {
							sigs[goType(recv, sig)] = sigInfo{sig, recv}
						}
					}
				}
			}
		}
	}
	keys := make([]string, 0, len(sigs))
	for k := range sigs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	keys = dedupe(keys, sigs)

	imports := map[string]bool{"go/types": true}
	var body bytes.Buffer
	for _, k := range keys {
		writeCase(&body, sigs[k], imports)
	}
	// The invoke cases are written to a scratch buffer first, for their
	// imports.
	for _, k := range sortedKeys(invs) {
		writeInvoke(new(bytes.Buffer), k, invs[k], imports)
	}
	var b bytes.Buffer
	// A plugin may still call deprecated std API (json.InvalidUTF8Error,
	// http.ProtocolError...): its trampolines name it too.
	release, err := gorelease.Constraint(runtime.Version())
	if err != nil {
		log.Fatal(err)
	}
	name := "zz_natsigs_" + gorelease.Prefix(runtime.Version()) + ".go"
	b.WriteString("// Code generated by natsigs_gen.go. DO NOT EDIT.\n\n//go:build " + release + "\n\npackage vm\n\n//lint:file-ignore SA1019 plugins may call deprecated std API: it gets a trampoline like the rest\n\nimport (\n")
	paths := make([]string, 0, len(imports))
	for p := range imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&b, "\t%s %q\n", alias(p), p)
	}
	b.WriteString(`)

// genFastSite returns a direct call of the native function fn (a method
// takes its receiver first), when natsigs_gen.go made one for its
// signature, or nil.
func (m *Machine) genFastSite(s *nativeSite, fn any) fastFn {
	switch f := fn.(type) {
`)
	b.Write(body.Bytes())
	b.WriteString("\t}\n\treturn nil\n}\n")
	ikeys := make([]string, 0, len(invs))
	for k := range invs {
		ikeys = append(ikeys, k)
	}
	sort.Strings(ikeys)
	b.WriteString(`
// genFastInvoke returns a direct call of the method of a native value in an
// interface whose name and signature give key (invokeKey), when
// natsigs_gen.go made one, or nil. The call asserts the value to an
// interface with only that method; it returns false when the value does
// not have it, or when an argument is not a native value (the general path
// converts it). rs are the result types.
func (m *Machine) genFastInvoke(key string, rs []go_types.Type) func(recv any, regs []Value, a []int32) (Value, bool) {
	switch key {
`)
	for _, k := range ikeys {
		writeInvoke(&b, k, invs[k], imports)
	}
	b.WriteString("\t}\n\treturn nil\n}\n")
	out, err := format.Source(b.Bytes())
	if err != nil {
		log.Fatalf("%v\n%s", err, b.Bytes())
	}
	// The files of the releases outside the window go (and the old file
	// without a release).
	old, _ := filepath.Glob("zz_natsigs*.go")
	for _, f := range old {
		if f == "zz_natsigs.go" || gorelease.OutsideWindow(f) {
			os.Remove(f)
		}
	}
	if err := os.WriteFile(name, out, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d signatures\n", name, len(keys))
}

// dedupe drops the keys whose function type is identical to the type of an
// earlier key: an alias makes two spellings of one type (os.FileMode and
// fs.FileMode, any and interface{}), and a type switch refuses two cases of
// the same type.
func dedupe(keys []string, sigs map[string]sigInfo) []string {
	var out []string
	var kept []*types.Signature
	for _, k := range keys {
		ft := fnType(sigs[k].recv, sigs[k].sig)
		dup := false
		for _, o := range kept {
			if types.Identical(ft, o) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, k)
			kept = append(kept, ft)
		}
	}
	return out
}

func skipPkg(path string) bool {
	switch path {
	case "syscall", "unsafe", "runtime/cgo", "os/exec", "plugin":
		return true
	}
	return strings.HasPrefix(path, "vendor/") || strings.Contains(path, "internal")
}

func supported(recv types.Type, sig *types.Signature) bool {
	n := sig.Params().Len()
	if recv != nil {
		n++
		if !natArg(recv) && !scalar(recv) {
			return false
		}
	}
	if sig.TypeParams().Len() > 0 || n > maxParams {
		return false
	}
	if sig.Variadic() && !isStrings(sig.Params().At(sig.Params().Len()-1).Type()) {
		return false // only ...string
	}
	strs := 0
	for i := 0; i < sig.Params().Len(); i++ {
		t := sig.Params().At(i).Type()
		if isStrings(t) {
			// One copied []string at most: two arguments that share their
			// array would get two copies (the general path makes one).
			if strs++; strs > 1 {
				return false
			}
			continue
		}
		if !scalar(t) && !natArg(t) && !isBytes(t) && !ifaceParam(t) {
			return false
		}
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if !nameable(sig.Results().At(i).Type()) {
			return false
		}
	}
	return true
}

// invokeResults reports whether the results of the interface method sig
// are of the types that genFastInvoke has always taken (basic, error,
// []string, []byte, the named types of typesPkgs). A method with other
// results (http.FileSystem.Open gives an http.File) takes the general
// path: TestGenInvokeReplaced checks that no direct call exists for the
// methods that a restricted machine replaces.
func invokeResults(sig *types.Signature) bool {
	for i := 0; i < sig.Results().Len(); i++ {
		t := sig.Results().At(i).Type()
		if !scalar(t) && !isError(t) && !scalarSlice(t) && !natArg(t) {
			return false
		}
	}
	return true
}

func isBytes(t types.Type) bool   { return isSliceOf(t, types.Uint8) }
func isStrings(t types.Type) bool { return isSliceOf(t, types.String) }

// isSliceOf reports whether t is the unnamed type []k.
func isSliceOf(t types.Type, k types.BasicKind) bool {
	s, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	b, ok := s.Elem().(*types.Basic)
	return ok && b.Kind() == k
}

func ifaceParam(t types.Type) bool { return ifaceParams[typeString(t)] }

func typeString(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string { return p.Path() })
}

// nameable reports whether the generated code can name t (all its named
// types exported, from importable packages, on every platform), so that a
// result of type t can be converted by fromNative.
func nameable(t types.Type) bool {
	switch t := t.(type) {
	case *types.Basic:
		return t.Kind() != types.UnsafePointer && t.Info()&types.IsUntyped == 0
	case *types.Alias:
		return nameable(types.Unalias(t)) && pkgOK(t.Obj())
	case *types.Named:
		if !pkgOK(t.Obj()) {
			return false
		}
		for i := 0; i < t.TypeArgs().Len(); i++ {
			if !nameable(t.TypeArgs().At(i)) {
				return false
			}
		}
		return true
	case *types.Pointer:
		return nameable(t.Elem())
	case *types.Slice:
		return nameable(t.Elem())
	case *types.Array:
		return nameable(t.Elem())
	case *types.Map:
		return nameable(t.Key()) && nameable(t.Elem())
	case *types.Chan:
		return nameable(t.Elem())
	case *types.Signature:
		if t.TypeParams().Len() > 0 {
			return false
		}
		for _, tu := range []*types.Tuple{t.Params(), t.Results()} {
			for i := 0; i < tu.Len(); i++ {
				if !nameable(tu.At(i).Type()) {
					return false
				}
			}
		}
		return true
	case *types.Interface:
		return t.Empty()
	}
	return false // struct literals, type parameters
}

// pkgOK reports whether the type name o can be written in the generated code.
func pkgOK(o *types.TypeName) bool {
	if o.Pkg() == nil {
		return true // error, any, comparable
	}
	p := o.Pkg().Path()
	return o.Exported() && !skipPkg(p) && !badResultPkgs[p]
}

// basicOf returns the basic type that the VM keeps in Value.N (numbers,
// bool) or Value.R (string) for t: a predeclared type, or a named type of
// typesPkgs with such an underlying type. Otherwise nil.
func basicOf(t types.Type) *types.Basic {
	if _, ok := t.(*types.Alias); ok { // os.FileMode: check the type it names
		return basicOf(types.Unalias(t))
	}
	if nt, ok := t.(*types.Named); ok && !usable(nt) {
		return nil
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok || b.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) == 0 || b.Info()&types.IsUntyped != 0 {
		return nil
	}
	return b
}

func scalar(t types.Type) bool { return basicOf(t) != nil }

// usable reports whether the generated code can name nt.
func usable(nt *types.Named) bool {
	o := nt.Obj()
	return o.Pkg() != nil && typesPkgs[o.Pkg().Path()] && o.Exported() && nt.TypeParams().Len() == 0
}

// natArg reports whether t is a named struct, map or slice type of
// typesPkgs (time.Time, url.Values, net.IP), or a pointer to such a struct:
// the trampoline needs the native value.
func natArg(t types.Type) bool {
	p, ptr := t.(*types.Pointer)
	if ptr {
		t = p.Elem()
	}
	nt, ok := types.Unalias(t).(*types.Named)
	if !ok || !usable(nt) {
		return false
	}
	switch nt.Underlying().(type) {
	case *types.Struct:
		return true
	case *types.Map, *types.Slice:
		return !ptr
	}
	return false
}

func isError(t types.Type) bool { return types.Identical(t, types.Universe.Lookup("error").Type()) }

func scalarSlice(t types.Type) bool {
	s, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	b, ok := s.Elem().(*types.Basic)
	return ok && (b.Kind() == types.String || b.Kind() == types.Uint8)
}

func alias(path string) string {
	return strings.NewReplacer("/", "_", ".", "_").Replace(path)
}

// goType writes the type of the function (with the receiver first) as Go
// source, without parameter names.
func goType(recv types.Type, sig *types.Signature) string {
	return typeName(fnType(recv, sig), nil)
}

// fnType returns the type of the function (with the receiver first),
// without parameter names.
func fnType(recv types.Type, sig *types.Signature) *types.Signature {
	var ps, rs []types.Type
	if recv != nil {
		ps = append(ps, recv)
	}
	for i := 0; i < sig.Params().Len(); i++ {
		ps = append(ps, sig.Params().At(i).Type())
	}
	for i := 0; i < sig.Results().Len(); i++ {
		rs = append(rs, sig.Results().At(i).Type())
	}
	return types.NewSignatureType(nil, nil, nil, tuple(ps), tuple(rs), sig.Variadic())
}

// typeName writes t as Go source of zz_natsigs.go, and records the imports
// it needs.
func typeName(t types.Type, imports map[string]bool) string {
	s := types.TypeString(t, func(p *types.Package) string {
		if imports != nil {
			imports[p.Path()] = true
		}
		return alias(p.Path())
	})
	// byte and rune are the same types as uint8 and int32: one spelling,
	// so that a signature is not two cases of the type switch.
	s = byteRE.ReplaceAllString(s, "uint8")
	return runeRE.ReplaceAllString(s, "int32")
}

// typeNameRaw is typeName with byte and rune kept.
func typeNameRaw(t types.Type, imports map[string]bool) string {
	return types.TypeString(t, func(p *types.Package) string {
		if imports != nil {
			imports[p.Path()] = true
		}
		return alias(p.Path())
	})
}

var (
	byteRE = regexp.MustCompile(`\bbyte\b`)
	runeRE = regexp.MustCompile(`\brune\b`)
)

// arg returns the statements that read the Go value of argument register
// i, of type t, into the variable x<i>.
func arg(t types.Type, i int, imports map[string]bool) string {
	return argFail(t, i, imports, "return m.callNativeSlow(s, regs)")
}

// argFail is arg with the statement fail when a native argument is not a
// native value.
func argFail(t types.Type, i int, imports map[string]bool, fail string) string {
	r := fmt.Sprintf("regs[a[%d]]", i)
	x := fmt.Sprintf("x%d", i)
	switch {
	case natArg(t):
		return fmt.Sprintf("%s, ok := natRecv[%s](%s)\nif !ok {\n%s\n}\n", x, typeName(t, imports), r, fail)
	case isBytes(t):
		return fmt.Sprintf("%s, ok := natBytes(%s)\nif !ok {\n%s\n}\n", x, r, fail)
	case isStrings(t):
		// src: the VM elements that were copied, for stringsBack
		return fmt.Sprintf("%s, src%d, ok := natStrings(%s)\nif !ok {\n%s\n}\n", x, i, r, fail)
	case ifaceParam(t):
		return fmt.Sprintf("%s, ok := natIfaceArg[%s](m, %s)\nif !ok {\n%s\n}\n", x, typeName(t, imports), r, fail)
	}
	b := basicOf(t)
	var e string
	switch {
	case b.Info()&types.IsString != 0:
		e = "vStr(" + r + ")"
	case b.Info()&types.IsBoolean != 0:
		e = r + ".N != 0"
	case b.Info()&types.IsFloat != 0:
		e = "f64(" + r + ")"
	default:
		e = r + ".N"
	}
	if !types.Identical(t, types.Typ[types.Int64]) && !types.Identical(t, types.Typ[types.Float64]) && !types.Identical(t, types.Typ[types.String]) {
		e = typeName(t, imports) + "(" + e + ")"
	}
	if b.Info()&types.IsBoolean != 0 && !types.Identical(t, types.Typ[types.Bool]) {
		e = typeName(t, imports) + "(" + r + ".N != 0)"
	}
	return fmt.Sprintf("%s := %s\n", x, e)
}

// result returns the expression of the VM value of the result x of type t,
// the result number i of the call.
func result(t types.Type, x string, i int) string {
	return resultIn(t, x, i, "s.results")
}

// resultIn is result with the result types in the slice rs.
func resultIn(t types.Type, x string, i int, rs string) string {
	switch {
	case isError(t):
		return fmt.Sprintf("m.natErrOut(%s, %s[%d])", x, rs, i)
	case types.IsInterface(t):
		// with its interface type, as the general path gives it
		return fmt.Sprintf("natIfaceOut(m, %s, %s[%d])", x, rs, i)
	case isBytes(t):
		return fmt.Sprintf("m.natBytesOut(%s, %s[%d])", x, rs, i)
	case basicOf(t) == nil:
		return fmt.Sprintf("m.natOut(%s, %s[%d])", x, rs, i)
	}
	b := basicOf(t)
	switch {
	case b.Info()&types.IsString != 0:
		return "Value{R: string(" + x + ")}"
	case b.Info()&types.IsBoolean != 0:
		return "Value{N: b2i(bool(" + x + "))}"
	case b.Info()&types.IsFloat != 0:
		return "Value{N: fv(float64(" + x + "))}"
	}
	return "Value{N: int64(" + x + ")}"
}

func writeCase(b *bytes.Buffer, si sigInfo, imports map[string]bool) {
	var ts []types.Type
	if si.recv != nil {
		ts = append(ts, si.recv)
	}
	for i := 0; i < si.sig.Params().Len(); i++ {
		ts = append(ts, si.sig.Params().At(i).Type())
	}
	var rts []types.Type // without the result names
	for i := 0; i < si.sig.Results().Len(); i++ {
		rts = append(rts, si.sig.Results().At(i).Type())
	}
	fmt.Fprintf(b, "\tcase %s:\n\t\treturn func(regs []Value, a []int32) Value {\n", typeName(types.NewSignatureType(nil, nil, nil, tuple(ts), tuple(rts), si.sig.Variadic()), imports))
	for i, t := range ts {
		b.WriteString(arg(t, i, imports))
	}
	writeCall(b, "f", ts, rts, si.sig.Variadic(), "s.results", "")
	b.WriteString("\t\t}\n")
}

// writeCall writes the call of f with the arguments x0, x1..., the
// write-back of its []string arguments, and the return of its results of
// types rts (rs: the slice of their go/types types) followed by tail.
func writeCall(b *bytes.Buffer, f string, ts, rts []types.Type, variadic bool, rs, tail string) {
	var args, back []string
	for i, t := range ts {
		x := fmt.Sprintf("x%d", i)
		if isStrings(t) {
			back = append(back, fmt.Sprintf("stringsBack(%s, src%d)\n", x, i))
		}
		if variadic && i == len(ts)-1 {
			x += "..."
		}
		args = append(args, x)
	}
	call := f + "(" + strings.Join(args, ", ") + ")"
	var rv, vs []string
	for i, t := range rts {
		r := fmt.Sprintf("r%d", i)
		rv = append(rv, r)
		vs = append(vs, resultIn(t, r, i, rs))
	}
	if len(rv) > 0 {
		call = strings.Join(rv, ", ") + " := " + call
	}
	b.WriteString("\t\t\t" + call + "\n")
	for _, s := range back {
		b.WriteString("\t\t\t" + s)
	}
	switch len(vs) {
	case 0:
		fmt.Fprintf(b, "\t\t\treturn Value{}%s\n", tail)
	case 1:
		fmt.Fprintf(b, "\t\t\treturn %s%s\n", vs[0], tail)
	default:
		fmt.Fprintf(b, "\t\t\treturn Value{R: []Value{%s}}%s\n", strings.Join(vs, ", "), tail)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// writeInvoke writes the case of genFastInvoke for the interface method
// inv, of key k.
func writeInvoke(b *bytes.Buffer, k string, inv invInfo, imports map[string]bool) {
	sig := inv.sig
	var ps, rts []types.Type
	for i := 0; i < sig.Params().Len(); i++ {
		ps = append(ps, sig.Params().At(i).Type())
	}
	for i := 0; i < sig.Results().Len(); i++ {
		rts = append(rts, sig.Results().At(i).Type())
	}
	// The method keeps the spelling of byte and rune (go vet checks the
	// signatures of well-known methods such as WriteByte).
	ft := typeNameRaw(types.NewSignatureType(nil, nil, nil, tuple(ps), tuple(rts), false), imports)
	fmt.Fprintf(b, "\tcase %q:\n\t\treturn func(recv any, regs []Value, a []int32) (Value, bool) {\n", k)
	fmt.Fprintf(b, "\t\t\tx, ok := recv.(interface{ %s%s })\n\t\t\tif !ok {\n\t\t\t\treturn Value{}, false\n\t\t\t}\n", inv.name, strings.TrimPrefix(ft, "func"))
	for i, t := range ps {
		b.WriteString(argFail(t, i, imports, "return Value{}, false"))
	}
	writeCall(b, "x."+inv.name, ps, rts, false, "rs", ", true")
	b.WriteString("\t\t}\n")
}

func tuple(ts []types.Type) *types.Tuple {
	vs := make([]*types.Var, len(ts))
	for i, t := range ts {
		vs[i] = types.NewParam(0, nil, "", t)
	}
	return types.NewTuple(vs...)
}
