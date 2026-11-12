// Package boundary holds the inventory of the native boundary of the
// standard library: its scan of GOROOT (Scan), its files
// (docs/boundary/go1_NN.tsv, docs/BOUNDARY.md) and the row comments of the
// test programs (testdata/boundary). See cmd/boundaryscan.
package boundary

import (
	"bufio"
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// excluded are the standard library packages that plugins cannot import
// (cmd/stdsyms excluded, vm/restrict.go refusedImports and unsafeImports),
// and reflect (see the package comment).
var excluded = map[string]bool{
	"os/exec": true, "plugin": true, "unsafe": true, "syscall": true,
	"runtime/cgo": true, "runtime/race": true, "runtime/msan": true, "runtime/asan": true, "runtime/coverage": true,
	"time/tzdata": true, "testing/cryptotest": true, "crypto/mlkem/mlkemtest": true,
	"net/http/cgi": true, "go/build": true, "go/importer": true, "net/http/pprof": true, "go/types": true,
	"reflect": true,
}

// Row is one line of the inventory.
type Row struct {
	Pkg   string // import path
	Entry string // exported function or method: F, T.M, (*T).M
	Kind  string // assert, reflect, panic
	Check string // what is checked
	Site  string // first place of the check: function file:line (GOROOT/src)
	// Detail lists what the check covers: the unnamed types of the
	// assertions, the reflect API of a category, the key types.
	Detail string
}

// Key identifies a row in every release: everything but the site and the
// detail.
func (r Row) Key() string { return r.Pkg + "\t" + r.Entry + "\t" + r.Kind + "\t" + r.Check }

// pluginPkgs returns the standard library packages that plugins can import.
func pluginPkgs(env []string) ([]string, error) {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName, Env: env}, "std")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range pkgs {
		path := p.PkgPath
		if excluded[path] || strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "vendor/") ||
			strings.HasPrefix(path, "syscall/") || strings.Contains(path, "internal") || strings.Contains(path, "testing") {
			continue
		}
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

// transparent reports whether the calls into path are followed from any
// package: the internal packages of the standard library, which plugins
// never call themselves.
func transparent(path string) bool {
	if path == "internal/reflectlite" {
		return false
	}
	return strings.HasPrefix(path, "internal/") || strings.Contains(path, "/internal/") || strings.HasPrefix(path, "vendor/")
}

func isReflect(path string) bool { return path == "reflect" || path == "internal/reflectlite" }

// Scan builds the inventory for the Go release of the toolchain: the go
// command of PATH and its GOROOT.
func Scan() ([]Row, error) {
	env := append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOFLAGS=")
	paths, err := pluginPkgs(env)
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Env: env}
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("load errors")
	}
	gr, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return nil, fmt.Errorf("go env GOROOT: %v", err)
	}
	goroot := strings.TrimSpace(string(gr))
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	s := &scanner{
		prog:    prog,
		tainted: map[ssa.Value]bool{},
		fields:  map[*types.Var]bool{},
		rets:    map[*ssa.Function]bool{},
		goroot:  filepath.Join(goroot, "src") + string(filepath.Separator),
	}
	all := ssautil.AllFunctions(prog)
	for fn := range all {
		if len(fn.Blocks) > 0 && !isReflect(pkgOf(fn)) {
			s.funcs = append(s.funcs, fn)
		}
	}
	sort.Slice(s.funcs, func(i, j int) bool { return s.funcs[i].String() < s.funcs[j].String() })

	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	type entry struct {
		pkg, name string
		fn        *ssa.Function
	}
	var entries []entry
	for _, sp := range prog.AllPackages() {
		path := sp.Pkg.Path()
		if !want[path] {
			continue
		}
		for name, mem := range sp.Members {
			if !token.IsExported(name) {
				continue
			}
			switch mem := mem.(type) {
			case *ssa.Function:
				if mem.TypeParams().Len() > 0 {
					continue
				}
				entries = append(entries, entry{path, name, mem})
				s.sourceParams(mem, false)
			case *ssa.Type:
				named, ok := mem.Type().(*types.Named)
				if !ok || named.TypeParams().Len() > 0 {
					continue
				}
				s.sourceFields(named)
				if types.IsInterface(named) {
					continue
				}
				mset := prog.MethodSets.MethodSet(types.NewPointer(named))
				for i := 0; i < mset.Len(); i++ {
					sel := mset.At(i)
					m := sel.Obj().(*types.Func)
					if !m.Exported() || m.Pkg() == nil || (m.Pkg().Path() != path && !transparent(m.Pkg().Path())) {
						continue
					}
					fn := prog.MethodValue(sel)
					if fn == nil {
						continue
					}
					ename := named.Obj().Name() + "." + m.Name()
					if _, ptr := m.Signature().Recv().Type().(*types.Pointer); ptr {
						ename = "(*" + named.Obj().Name() + ")." + m.Name()
					}
					entries = append(entries, entry{path, ename, fn})
					s.sourceParams(fn, true)
				}
			}
		}
	}
	s.globalFuncs()
	s.propagate()
	sites := s.checks()

	var rows []Row
	for _, e := range entries {
		at := map[string]int{}
		details := map[string]map[string]bool{}
		for _, fn := range s.reach(e.fn, e.pkg) {
			for _, c := range sites[fn] {
				r := Row{Pkg: e.pkg, Entry: e.name, Kind: c.kind, Check: c.check, Site: c.site}
				if _, ok := at[r.Key()]; !ok {
					at[r.Key()] = len(rows)
					details[r.Key()] = map[string]bool{}
					rows = append(rows, r)
				}
				if c.detail != "" {
					details[r.Key()][c.detail] = true
				}
			}
		}
		for k, i := range at {
			var ds []string
			for d := range details[k] {
				ds = append(ds, d)
			}
			sort.Strings(ds)
			rows[i].Detail = strings.Join(ds, " ")
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key() < rows[j].Key() })
	return rows, nil
}

// pkgOf returns the import path of the package of fn (of its enclosing
// function for a closure, of its generic origin for an instance).
func pkgOf(fn *ssa.Function) string {
	for fn.Parent() != nil {
		fn = fn.Parent()
	}
	if o := fn.Origin(); o != nil {
		fn = o
	}
	if fn.Pkg != nil {
		return fn.Pkg.Pkg.Path()
	}
	if obj := fn.Object(); obj != nil && obj.Pkg() != nil {
		return obj.Pkg().Path()
	}
	return ""
}

type scanner struct {
	prog    *ssa.Program
	funcs   []*ssa.Function
	tainted map[ssa.Value]bool
	fields  map[*types.Var]bool    // struct fields that hold values of the caller
	rets    map[*ssa.Function]bool // functions that return values of the caller
	// callbacks are the functions that the initializer of a package
	// variable stores in it (a FuncMap of html/template): another package
	// calls them with values of the caller. A function that reads the
	// variable reaches them.
	callbacks map[*ssa.Global][]*ssa.Function
	goroot    string
	changed   bool
}

// canCarry reports whether a value of type t can hold a value of the
// program: an interface, a func, a type parameter, or a type that contains
// one.
func canCarry(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t := t.(type) {
	case *types.Interface, *types.Signature, *types.TypeParam:
		return true
	case *types.Named:
		return canCarry(t.Underlying(), seen)
	case *types.Alias:
		return canCarry(types.Unalias(t), seen)
	case *types.Pointer:
		return canCarry(t.Elem(), seen)
	case *types.Slice:
		return canCarry(t.Elem(), seen)
	case *types.Array:
		return canCarry(t.Elem(), seen)
	case *types.Chan:
		return canCarry(t.Elem(), seen)
	case *types.Map:
		return canCarry(t.Key(), seen) || canCarry(t.Elem(), seen)
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if canCarry(t.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func (s *scanner) taint(v ssa.Value) {
	if v != nil && !s.tainted[v] {
		s.tainted[v] = true
		s.changed = true
	}
}

// sourceParams taints the parameters of an exported function or method
// (not the receiver) that can hold a value of the program.
func (s *scanner) sourceParams(fn *ssa.Function, method bool) {
	for i, p := range fn.Params {
		if method && i == 0 && fn.Signature.Recv() != nil {
			continue
		}
		if canCarry(p.Type(), map[types.Type]bool{}) {
			s.taint(p)
		}
	}
	// A method value made by prog.MethodValue for a pointer receiver of a
	// value method is a wrapper: its params are fn.Params with the receiver.
}

// sourceFields taints the exported fields of an exported struct type that
// can hold a value of the program: the caller sets them (http.Server.Handler,
// http.Request.Body).
func (s *scanner) sourceFields(named *types.Named) {
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Exported() && canCarry(f.Type(), map[types.Type]bool{}) {
			s.fields[f.Origin()] = true
		}
	}
}

func fieldVar(x ssa.Value, idx int) *types.Var {
	t := x.Type().Underlying()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem().Underlying()
	}
	st, ok := t.(*types.Struct)
	if !ok || idx >= st.NumFields() {
		return nil
	}
	return st.Field(idx).Origin()
}

// root returns the value whose taint a store through addr changes.
func root(addr ssa.Value) ssa.Value {
	for {
		switch a := addr.(type) {
		case *ssa.IndexAddr:
			addr = a.X
		case *ssa.Slice:
			addr = a.X
		default:
			return addr
		}
	}
}

func (s *scanner) propagate() {
	for {
		s.changed = false
		for _, fn := range s.funcs {
			s.pass(fn)
		}
		if !s.changed {
			return
		}
	}
}

func (s *scanner) pass(fn *ssa.Function) {
	t := s.tainted
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			switch i := ins.(type) {
			case *ssa.Phi:
				for _, e := range i.Edges {
					if t[e] {
						s.taint(i)
					}
				}
			case *ssa.UnOp:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.Field:
				if t[i.X] || s.fields[fieldVar(i.X, i.Field)] {
					s.taint(i)
				}
			case *ssa.FieldAddr:
				if t[i.X] || s.fields[fieldVar(i.X, i.Field)] {
					s.taint(i)
				}
			case *ssa.Index:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.IndexAddr:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.Lookup:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.Slice:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.Extract:
				if t[i.Tuple] {
					s.taint(i)
				}
			case *ssa.Next:
				if t[i.Iter] {
					s.taint(i)
				}
			case *ssa.Range:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.ChangeInterface:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.ChangeType:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.Convert:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.MultiConvert:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.SliceToArrayPointer:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.MakeInterface:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.TypeAssert:
				if t[i.X] {
					s.taint(i)
				}
			case *ssa.MakeClosure:
				cl := i.Fn.(*ssa.Function)
				for j, bv := range i.Bindings {
					if t[bv] && j < len(cl.FreeVars) {
						s.taint(cl.FreeVars[j])
					}
				}
			case *ssa.Store:
				if t[i.Val] {
					if fa, ok := i.Addr.(*ssa.FieldAddr); ok {
						if f := fieldVar(fa.X, fa.Field); f != nil && !s.fields[f] {
							s.fields[f] = true
							s.changed = true
						}
					} else {
						s.taint(i.Addr)
						s.taint(root(i.Addr))
					}
				}
			case *ssa.MapUpdate:
				if t[i.Key] || t[i.Value] {
					s.taint(root(i.Map))
					s.taint(i.Map)
				}
			case *ssa.Send:
				if t[i.X] {
					s.taint(i.Chan)
				}
			case *ssa.Return:
				for _, r := range i.Results {
					if t[r] && !s.rets[fn] {
						s.rets[fn] = true
						s.changed = true
					}
				}
			case ssa.CallInstruction:
				s.call(i)
			}
		}
	}
}

func (s *scanner) call(i ssa.CallInstruction) {
	t := s.tainted
	cc := i.Common()
	res := i.Value() // nil for go and defer
	anyArg := false
	for _, a := range cc.Args {
		if t[a] {
			anyArg = true
		}
	}
	if cc.IsInvoke() {
		if t[cc.Value] || (anyArg && isReflectType(cc.Value.Type())) {
			s.taint(res)
		}
		return
	}
	if b, ok := cc.Value.(*ssa.Builtin); ok {
		if anyArg && (b.Name() == "append" || b.Name() == "min" || b.Name() == "max") {
			s.taint(res)
		}
		if b.Name() == "copy" && len(cc.Args) == 2 && t[cc.Args[1]] {
			s.taint(root(cc.Args[0]))
		}
		return
	}
	callee := cc.StaticCallee()
	if callee == nil {
		if t[cc.Value] {
			s.taint(res)
		}
		return
	}
	if len(callee.Blocks) == 0 || isReflect(pkgOf(callee)) {
		// assembly, a function of reflect, a function without body: the
		// result comes from the arguments
		if anyArg {
			s.taint(res)
		}
		return
	}
	for j, a := range cc.Args {
		if t[a] && j < len(callee.Params) {
			s.taint(callee.Params[j])
		}
	}
	if s.rets[callee] {
		s.taint(res)
	}
}

func isReflectType(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && isReflect(n.Obj().Pkg().Path())
}

// check is one place of a check in a function.
type check struct {
	kind, check, detail, site string
}

func (s *scanner) pos(fn *ssa.Function, p token.Pos) string {
	if !p.IsValid() {
		p = fn.Pos()
	}
	ps := s.prog.Fset.Position(p)
	file := strings.TrimPrefix(ps.Filename, s.goroot)
	name := fn.Name()
	for f := fn; f.Parent() != nil; f = f.Parent() {
		name = f.Parent().Name() + "." + name
	}
	return fmt.Sprintf("%s %s:%d", name, file, ps.Line)
}

// qual prints the types with their package path, except the package of
// the check itself, which is printed with its name.
func qual(p *types.Package) string { return p.Path() }

func typeStr(t types.Type) string { return types.TypeString(t, qual) }

// checks returns the checks of each function, on values of the caller.
func (s *scanner) checks() map[*ssa.Function][]check {
	t := s.tainted
	out := map[*ssa.Function][]check{}
	for _, fn := range s.funcs {
		var cs []check
		add := func(kind, c, d string, p token.Pos) { cs = append(cs, check{kind, c, d, s.pos(fn, p)}) }
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				switch i := ins.(type) {
				case *ssa.TypeAssert:
					if !t[i.X] {
						continue
					}
					at := i.AssertedType
					if !types.IsInterface(at) {
						// A named type of the standard library: no value of
						// the program has it. An unnamed type (string,
						// []byte): a named type of the program must not
						// match it.
						if isNamedConcrete(at) {
							continue
						}
						add("assert", "x.(T), T unnamed", typeStr(at), i.Pos())
						if !i.CommaOk {
							add("panic", "x.(T) without ok, T unnamed: *runtime.TypeAssertionError", typeStr(at), i.Pos())
						}
						continue
					}
					if unexportedMethod(at) {
						// no type of another package implements it
						continue
					}
					// the detail is the static type of x
					add("assert", "x.("+typeStr(at)+")", typeStr(i.X.Type()), i.Pos())
					if !i.CommaOk {
						add("panic", "x.("+typeStr(at)+") without ok: *runtime.TypeAssertionError", typeStr(i.X.Type()), i.Pos())
					}
				case *ssa.MapUpdate:
					if t[i.Key] && types.IsInterface(i.Key.Type()) {
						add("panic", "map key of interface type: hash of unhashable type", typeStr(i.Key.Type()), i.Pos())
					}
				case *ssa.Lookup:
					if _, ok := i.X.Type().Underlying().(*types.Map); ok && t[i.Index] && types.IsInterface(i.Index.Type()) {
						add("panic", "map key of interface type: hash of unhashable type", typeStr(i.Index.Type()), i.Pos())
					}
				case *ssa.BinOp:
					if (i.Op == token.EQL || i.Op == token.NEQ) && t[i.X] && t[i.Y] && types.IsInterface(i.X.Type()) && types.IsInterface(i.Y.Type()) {
						add("panic", "== of two interfaces: comparing uncomparable type", "", i.Pos())
					}
				case *ssa.Panic:
					if s.typeMessage(i.X, 0) {
						add("panic", "panic message with the type name", "", i.Pos())
					}
				case ssa.CallInstruction:
					if c, d := s.reflectCheck(i); c != "" {
						add("reflect", c, d, i.Pos())
					}
				}
			}
		}
		if len(cs) > 0 {
			out[fn] = cs
		}
	}
	return out
}

// typeMessage reports whether v, a panic value, is made from the type of a
// value of the caller: a fmt call with %T and such an argument, or a type
// name read by reflect.
func (s *scanner) typeMessage(v ssa.Value, depth int) bool {
	if depth > 6 || v == nil {
		return false
	}
	switch v := v.(type) {
	case *ssa.MakeInterface:
		return s.typeMessage(v.X, depth+1)
	case *ssa.ChangeInterface:
		return s.typeMessage(v.X, depth+1)
	case *ssa.BinOp:
		return s.typeMessage(v.X, depth+1) || s.typeMessage(v.Y, depth+1)
	case *ssa.Call:
		cc := v.Common()
		if cc.IsInvoke() {
			return isReflectType(cc.Value.Type()) && s.tainted[cc.Value] && (cc.Method.Name() == "String" || cc.Method.Name() == "Name")
		}
		callee := cc.StaticCallee()
		if callee == nil {
			return false
		}
		if pkgOf(callee) == "fmt" && len(cc.Args) > 0 {
			if c, ok := cc.Args[0].(*ssa.Const); ok && c.Value != nil && c.Value.Kind() == constant.String && strings.Contains(constant.StringVal(c.Value), "%T") {
				for _, a := range cc.Args[1:] {
					if s.tainted[a] {
						return true
					}
				}
			}
		}
		if isReflect(pkgOf(callee)) && (callee.Name() == "String" || callee.Name() == "Name") {
			for _, a := range cc.Args {
				if s.tainted[a] {
					return true
				}
			}
		}
		for _, a := range cc.Args {
			if s.typeMessage(a, depth+1) {
				return true
			}
		}
	case *ssa.Alloc, *ssa.Slice, *ssa.IndexAddr:
		// a []any for a variadic call: its elements
		ref := v.(ssa.Value).Referrers()
		if ref == nil {
			return false
		}
		for _, r := range *ref {
			if st, ok := r.(*ssa.Store); ok && st.Addr != v {
				continue
			} else if ok && s.typeMessage(st.Val, depth+1) {
				return true
			}
			if ia, ok := r.(*ssa.IndexAddr); ok && ia != v && s.typeMessage(ia, depth+1) {
				return true
			}
		}
	}
	return false
}

// reflectCategory groups the reflect API by what it reads of the type.
var reflectCategory = map[string]string{
	"String": "type name", "Name": "type name", "PkgPath": "type name",
	"Kind": "kind",
	"Len":  "length", "Cap": "length",
	"Method": "method set", "MethodByName": "method set", "NumMethod": "method set", "Implements": "method set",
	"Comparable": "comparability", "Equal": "comparability", "DeepEqual": "comparability",
}

// reflectCheck returns the reflect check of a call on a value of the
// caller, or "".
func (s *scanner) reflectCheck(i ssa.CallInstruction) (cat, api string) {
	cc := i.Common()
	var name, recv string
	tainted := false
	if cc.IsInvoke() {
		if !isReflectType(cc.Value.Type()) {
			return "", ""
		}
		name = cc.Method.Name()
		recv = types.Unalias(cc.Value.Type()).(*types.Named).Obj().Name() + "."
		tainted = s.tainted[cc.Value]
	} else {
		callee := cc.StaticCallee()
		if callee == nil || !isReflect(pkgOf(callee)) {
			return "", ""
		}
		name = callee.Name()
		if r := callee.Signature.Recv(); r != nil {
			rt := r.Type()
			if p, ok := rt.(*types.Pointer); ok {
				rt = p.Elem()
			}
			if n, ok := types.Unalias(rt).(*types.Named); ok {
				recv = n.Obj().Name() + "."
			}
		}
	}
	for _, a := range cc.Args {
		if s.tainted[a] {
			tainted = true
		}
	}
	if !tainted {
		return "", ""
	}
	cat = reflectCategory[name]
	if cat == "" {
		cat = "walk"
	}
	return cat, pkgOfName(cc) + recv + name
}

func pkgOfName(cc *ssa.CallCommon) string {
	if cc.IsInvoke() {
		return types.Unalias(cc.Value.Type()).(*types.Named).Obj().Pkg().Name() + "."
	}
	return filepath.Base(pkgOf(cc.StaticCallee())) + "."
}

// reach returns the functions that entry calls in its own package and in
// the internal packages: static calls, closures and function values.
func (s *scanner) reach(entry *ssa.Function, pkg string) []*ssa.Function {
	in := func(fn *ssa.Function) bool {
		p := pkgOf(fn)
		return p == pkg || transparent(p)
	}
	seen := map[*ssa.Function]bool{entry: true}
	queue := []*ssa.Function{entry}
	for len(queue) > 0 {
		fn := queue[0]
		queue = queue[1:]
		visit := func(f *ssa.Function) {
			if f != nil && !seen[f] && in(f) {
				seen[f] = true
				queue = append(queue, f)
			}
		}
		for _, anon := range fn.AnonFuncs {
			visit(anon)
		}
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				if c, ok := ins.(ssa.CallInstruction); ok {
					visit(c.Common().StaticCallee())
				}
				for _, op := range ins.Operands(nil) {
					if op == nil {
						continue
					}
					switch v := (*op).(type) {
					case *ssa.Function:
						visit(v)
					case *ssa.Global:
						for _, f := range s.callbacks[v] {
							visit(f)
						}
					}
				}
			}
		}
	}
	out := make([]*ssa.Function, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// WriteTSV writes the rows to path.
func WriteTSV(path string, rows []Row) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "# Generated by go run ./cmd/boundaryscan with %s. DO NOT EDIT.\n", runtime.Version())
	fmt.Fprintln(w, "# package\tentry\tkind\tcheck\tdetail\tfirst site")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Pkg, r.Entry, r.Kind, r.Check, r.Detail, r.Site)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return f.Close()
}

// isNamedConcrete reports whether t is a named type that is not an
// interface, or a pointer to one.
func isNamedConcrete(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	_, ok := types.Unalias(t).(*types.Named)
	return ok && !types.IsInterface(t)
}

// unexportedMethod reports whether the interface t has an unexported
// method: a type of the program cannot implement it.
func unexportedMethod(t types.Type) bool {
	it := t.Underlying().(*types.Interface)
	for i := 0; i < it.NumMethods(); i++ {
		if !it.Method(i).Exported() {
			return true
		}
	}
	return false
}

// globalFuncs finds the callbacks: the functions that a package
// initializer stores in a package variable, directly or in the map, slice
// or struct stored there. Their parameters that can hold a value of the
// program are sources.
func (s *scanner) globalFuncs() {
	s.callbacks = map[*ssa.Global][]*ssa.Function{}
	for _, fn := range s.funcs {
		if fn.Name() != "init" || fn.Parent() != nil {
			continue
		}
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				st, ok := ins.(*ssa.Store)
				if !ok {
					continue
				}
				g, ok := st.Addr.(*ssa.Global)
				if !ok {
					continue
				}
				seen := map[ssa.Value]bool{}
				var walk func(v ssa.Value)
				walk = func(v ssa.Value) {
					if v == nil || seen[v] {
						return
					}
					seen[v] = true
					switch v := v.(type) {
					case *ssa.Function:
						s.callbacks[g] = append(s.callbacks[g], v)
					case *ssa.MakeClosure:
						walk(v.Fn)
					case *ssa.MakeInterface:
						walk(v.X)
					case *ssa.ChangeType:
						walk(v.X)
					case *ssa.MakeMap, *ssa.Alloc, *ssa.Slice:
						if refs := v.Referrers(); refs != nil {
							for _, r := range *refs {
								switch r := r.(type) {
								case *ssa.MapUpdate:
									walk(r.Value)
								case *ssa.Store:
									walk(r.Val)
								case *ssa.IndexAddr:
									walk(r)
								case *ssa.FieldAddr:
									walk(r)
								case *ssa.Slice:
									walk(r)
								}
							}
						}
					case *ssa.IndexAddr, *ssa.FieldAddr:
						if refs := v.Referrers(); refs != nil {
							for _, r := range *refs {
								if st, ok := r.(*ssa.Store); ok {
									walk(st.Val)
								}
							}
						}
					case *ssa.UnOp:
						walk(v.X)
					}
				}
				walk(st.Val)
			}
		}
	}
	for _, fns := range s.callbacks {
		for _, f := range fns {
			for _, p := range f.Params {
				if canCarry(p.Type(), map[types.Type]bool{}) {
					s.taint(p)
				}
			}
		}
	}
}
