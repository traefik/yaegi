package main

import (
	"bytes"
	"fmt"
	"go/format"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/traefik/yaegi/v2/internal/srchash"
)

// glue generates zz_aotglue.go of a package: symbol table, native hooks,
// interface wrappers and type-switch case lists.
type glue struct {
	g       *gen
	pi      *pinfo
	pkg     *types.Package
	inPath  string            // import path of the generated file's package
	imports map[string]string // import path -> local name
	names   map[string]bool
	native  bool // native symbol table (qualify the package's own objects)
	selfQ   string
}

func newGlue(g *gen, pi *pinfo) *glue {
	return &glue{g: g, pi: pi, pkg: pi.p.Types, inPath: pi.aotPath, imports: map[string]string{},
		names: map[string]bool{vmName: true, "aotreflect": true}}
}

func (gl *glue) importName(path, want string) string {
	if n, ok := gl.imports[path]; ok {
		return n
	}
	name := sanitize(want)
	for i := 2; gl.names[name]; i++ {
		name = sanitize(want) + strconv.Itoa(i)
	}
	gl.names[name] = true
	gl.imports[path] = name
	return name
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' && b.Len() > 0 {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "p"
	}
	return b.String()
}

func (gl *glue) qualifier(p *types.Package) string {
	if p == gl.pkg && !gl.native {
		return ""
	}
	if p == gl.pkg && gl.native {
		return gl.selfQ
	}
	return gl.importName(gl.g.importPathFor(p.Path()), p.Name())
}

func (gl *glue) typ(t types.Type) string { return types.TypeString(t, gl.qualifier) }

// printable reports whether t can be written in the generated file.
func (gl *glue) printable(t types.Type) bool {
	ok := true
	seen := map[types.Type]bool{}
	var walk func(t types.Type)
	walk = func(t types.Type) {
		if !ok || t == nil || seen[t] {
			return
		}
		seen[t] = true
		switch t := t.(type) {
		case *types.Alias:
			walk(types.Unalias(t))
		case *types.Named:
			obj := t.Obj()
			if obj.Pkg() != nil {
				if obj.Pkg() != gl.pkg || gl.native {
					if !obj.Exported() {
						ok = false
						return
					}
					if !importable(gl.g.importPathFor(obj.Pkg().Path()), gl.inPath) {
						ok = false
						return
					}
				}
				if obj.Parent() != obj.Pkg().Scope() {
					ok = false // local type
					return
				}
			}
			if ta := t.TypeArgs(); ta != nil {
				for i := 0; i < ta.Len(); i++ {
					walk(ta.At(i))
				}
			}
		case *types.Pointer:
			walk(t.Elem())
		case *types.Slice:
			walk(t.Elem())
		case *types.Array:
			walk(t.Elem())
		case *types.Map:
			walk(t.Key())
			walk(t.Elem())
		case *types.Chan:
			walk(t.Elem())
		case *types.Signature:
			for i := 0; i < t.Params().Len(); i++ {
				walk(t.Params().At(i).Type())
			}
			for i := 0; i < t.Results().Len(); i++ {
				walk(t.Results().At(i).Type())
			}
		case *types.Interface:
			for i := 0; i < t.NumMethods(); i++ {
				m := t.Method(i)
				if !m.Exported() && (m.Pkg() != gl.pkg || gl.native) {
					ok = false
				}
				walk(m.Type())
			}
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				if !t.Field(i).Exported() && gl.native {
					ok = false
				}
				walk(t.Field(i).Type())
			}
		case *types.TypeParam:
			ok = false
		}
	}
	walk(t)
	return ok
}

// methodsOf returns the method set of an interface type.
func methodsOf(t types.Type) []*types.Func {
	it := t.Underlying().(*types.Interface)
	out := make([]*types.Func, it.NumMethods())
	for i := range out {
		out[i] = it.Method(i)
	}
	return out
}

// sigText writes the parameter and result lists of a wrapper method and the
// call arguments.
func (gl *glue) sigText(s *types.Signature) (params, results, args string, nres int) {
	var ps, as []string
	for i := 0; i < s.Params().Len(); i++ {
		t := s.Params().At(i).Type()
		name := fmt.Sprintf("a%d", i)
		if s.Variadic() && i == s.Params().Len()-1 {
			ps = append(ps, name+" ..."+gl.typ(t.(*types.Slice).Elem()))
		} else {
			ps = append(ps, name+" "+gl.typ(t))
		}
		as = append(as, ", "+vmName+".In(&"+name+")")
	}
	var rs []string
	for i := 0; i < s.Results().Len(); i++ {
		rs = append(rs, gl.typ(s.Results().At(i).Type()))
	}
	results = strings.Join(rs, ", ")
	if len(rs) > 1 {
		results = "(" + results + ")"
	}
	return strings.Join(ps, ", "), results, strings.Join(as, ""), len(rs)
}

func (gl *glue) wrapperType(b *bytes.Buffer, name string, ms []*types.Func) {
	fmt.Fprintf(b, "type %s struct{ %s.Obj }\n\n", name, vmName)
	for _, m := range ms {
		s := m.Type().(*types.Signature)
		params, results, args, nres := gl.sigText(s)
		fmt.Fprintf(b, "func (w %s) %s(%s) %s {\n", name, m.Name(), params, results)
		call := fmt.Sprintf("w.Obj.Call(%q%s)", m.Name(), args)
		if nres == 0 {
			fmt.Fprintf(b, "\t%s\n}\n\n", call)
			continue
		}
		fmt.Fprintf(b, "\tout := %s\n\treturn ", call)
		for i := 0; i < nres; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "%s.Out[%s](out, %d)", vmName, gl.typ(s.Results().At(i).Type()), i)
		}
		b.WriteString("\n}\n\n")
	}
}

// wrappable reports whether a wrapper can implement interface t here.
func (gl *glue) wrappable(t types.Type) bool {
	if !nonEmptyIface(t) || !gl.printable(t) || !t.Underlying().(*types.Interface).IsMethodSet() {
		return false
	}
	for _, m := range methodsOf(t) {
		if !m.Exported() && m.Pkg() != gl.pkg {
			return false
		}
	}
	return true
}

// hookable reports whether the generated hook can convert a parameter of
// type t without losing the aliasing semantics of the VM value.
func hookable(t types.Type, depth int) bool {
	if depth > 3 {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() != types.UnsafePointer && u.Info()&types.IsComplex == 0
	case *types.Interface:
		return true
	case *types.Slice:
		return hookable(u.Elem(), depth+1)
	case *types.Pointer:
		_, named := types.Unalias(u.Elem()).(*types.Named)
		return named // pointers to native named types are shared with the VM
	case *types.Struct:
		_, named := types.Unalias(t).(*types.Named)
		return named
	case *types.Signature:
		for i := 0; i < u.Params().Len(); i++ {
			if !hookable(u.Params().At(i).Type(), depth+1) {
				return false
			}
		}
		for i := 0; i < u.Results().Len(); i++ {
			if !hookable(u.Results().At(i).Type(), depth+1) {
				return false
			}
		}
		return true
	}
	return false
}

func isByteSlice(t types.Type) bool {
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	b, ok := s.Elem().Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

// hook writes the RegisterNative call of a function or method.
func (gl *glue) hook(b *bytes.Buffer, key, callee string, recv types.Type, s *types.Signature) bool {
	all := []types.Type{}
	if recv != nil {
		all = append(all, recv)
	}
	for i := 0; i < s.Params().Len(); i++ {
		all = append(all, s.Params().At(i).Type())
	}
	for _, t := range all {
		if !gl.printable(t) || !hookable(t, 0) {
			return false
		}
	}
	for i := 0; i < s.Results().Len(); i++ {
		if !gl.printable(s.Results().At(i).Type()) {
			return false
		}
	}
	fmt.Fprintf(b, "\t%s.RegisterNative(%q, func(m *%s.Machine, a []%s.Value) %s.Value {\n", vmName, key, vmName, vmName, vmName)
	var args, after []string
	ai := 0
	if recv != nil {
		fmt.Fprintf(b, "\t\trecv := %s.Arg[%s](m, a[0])\n", vmName, gl.typ(recv))
		if st, isSlice := recv.Underlying().(*types.Slice); isSlice && !isByteSlice(recv) && !isIface(st.Elem()) {
			after = append(after, fmt.Sprintf("\t\t%s.WriteBack(m, a[0], recv)\n", vmName))
		}
		callee = "recv." + callee
		ai = 1
	}
	for i := 0; i < s.Params().Len(); i++ {
		t := s.Params().At(i).Type()
		v := fmt.Sprintf("x%d", i)
		fmt.Fprintf(b, "\t\t%s := %s.Arg[%s](m, a[%d])\n", v, vmName, gl.typ(t), ai+i)
		variadic := s.Variadic() && i == s.Params().Len()-1
		if st, isSlice := t.Underlying().(*types.Slice); isSlice && !isByteSlice(t) && !variadic && !isIface(st.Elem()) {
			after = append(after, fmt.Sprintf("\t\t%s.WriteBack(m, a[%d], %s)\n", vmName, ai+i, v))
		}
		if s.Variadic() && i == s.Params().Len()-1 {
			v += "..."
		}
		args = append(args, v)
	}
	call := callee + "(" + strings.Join(args, ", ") + ")"
	n := s.Results().Len()
	switch n {
	case 0:
		fmt.Fprintf(b, "\t\t%s\n", call)
	default:
		var rs []string
		for i := 0; i < n; i++ {
			rs = append(rs, fmt.Sprintf("r%d", i))
		}
		fmt.Fprintf(b, "\t\t%s := %s\n", strings.Join(rs, ", "), call)
	}
	for _, a := range after {
		b.WriteString(a)
	}
	switch n {
	case 0:
		fmt.Fprintf(b, "\t\treturn %s.Value{}\n", vmName)
	case 1:
		fmt.Fprintf(b, "\t\treturn %s.Ret(m, r0)\n", vmName)
	default:
		var rs []string
		for i := 0; i < n; i++ {
			rs = append(rs, fmt.Sprintf("%s.Ret(m, r%d)", vmName, i))
		}
		fmt.Fprintf(b, "\t\treturn %s.Tuple(%s)\n", vmName, strings.Join(rs, ", "))
	}
	b.WriteString("\t})\n")
	return true
}

// addWrap adds t (and the interfaces of its methods, transitively) to the
// interfaces to wrap.
func addWrap(list []types.Type, t types.Type, gl *glue) []types.Type {
	if !gl.wrappable(t) {
		return list
	}
	for _, x := range list {
		if types.Identical(x, t) {
			return list
		}
	}
	list = append(list, t)
	for _, m := range methodsOf(t) {
		s := m.Type().(*types.Signature)
		for i := 0; i < s.Params().Len(); i++ {
			list = addWrap(list, s.Params().At(i).Type(), gl)
		}
		for i := 0; i < s.Results().Len(); i++ {
			list = addWrap(list, s.Results().At(i).Type(), gl)
		}
	}
	return list
}

// writeSources writes the Module, Version and Sources fields of the
// registration of a non-standard package: the module version the copy is
// generated from, and the hash of the sources it stands for. The VM uses
// the copy only for a plugin with the same version and the same sources
// (vm/aotsource.go). Standard library copies have no such fields.
//
// Sources has the packages of the copy's import closure that belong to its
// own module (a fork may change any of them), and the packages of other
// modules that aotgen copies too (their code is part of the copy). The
// other dependencies stay native: the copy calls yaegi v2's version of them,
// as the plugin's code calls yaegi v2's standard library.
func (g *gen) writeSources(body *bytes.Buffer, pi *pinfo) error {
	mod := pi.p.Module
	if mod == nil {
		return nil
	}
	if mod.Replace != nil {
		return fmt.Errorf("%s: module %s is replaced in go.mod: the version check of the AOT copy needs the real module", pi.p.PkgPath, mod.Path)
	}
	fmt.Fprintf(body, "\t\tModule: %q,\n\t\tVersion: %q,\n\t\tSources: map[string]string{\n", mod.Path, mod.Version)
	var deps []*packages.Package
	packages.Visit([]*packages.Package{pi.p}, nil, func(p *packages.Package) {
		if p.Module == nil {
			return
		}
		if qi := g.pkgs[p.PkgPath]; p.Module.Path == mod.Path || qi != nil && qi.kind != kNative {
			deps = append(deps, p)
		}
	})
	sort.Slice(deps, func(i, j int) bool { return deps[i].PkgPath < deps[j].PkgPath })
	for _, p := range deps {
		dir := p.Dir
		if dir == "" && len(p.GoFiles) > 0 {
			dir = filepath.Dir(p.GoFiles[0])
		}
		h, err := srchash.Dir(dir)
		if err != nil {
			return fmt.Errorf("%s: %v", p.PkgPath, err)
		}
		fmt.Fprintf(body, "\t\t\t%q: %q,\n", p.PkgPath, h)
	}
	body.WriteString("\t\t},\n")
	return nil
}

func (g *gen) writeGlue(pi *pinfo) error {
	gl := newGlue(g, pi)
	var body bytes.Buffer
	scope := pi.p.Types.Scope()
	names := scope.Names()
	sort.Strings(names)
	fmt.Fprintf(&body, "func init() {\n")
	if pi.kind == kCopy {
		fmt.Fprintf(&body, "\t%s.RegisterAOTPath(%q, %q)\n}\n", vmName, pi.aotPath, pi.p.PkgPath)
		return g.writeGlueFile(pi, gl, body.Bytes(), nil)
	}
	// Symbol table.
	// GoVersion: the copies follow the standard library of the Go version
	// they are made with, which vm.Load compares with the host's.
	fmt.Fprintf(&body, "\t%s.RegisterAOT(&%s.Package{\n\t\tPath: %q,\n\t\tAOTPath: %q,\n\t\tGoVersion: %q,\n", vmName, vmName, pi.p.PkgPath, pi.aotPath, runtime.Version())
	if err := g.writeSources(&body, pi); err != nil {
		return err
	}
	var funcs, typs, vars []string
	var exportedFuncs []*types.Func
	var exportedTypes []*types.TypeName
	for _, n := range names {
		obj := scope.Lookup(n)
		if tn, ok := obj.(*types.TypeName); ok && !obj.Exported() {
			// Unexported types reach the VM through exported variables
			// and results (e.g. binary.BigEndian of type bigEndian).
			if nt, ok := tn.Type().(*types.Named); ok && nt.TypeParams() == nil && !tn.IsAlias() {
				if it, ok := nt.Underlying().(*types.Interface); !ok || it.IsMethodSet() {
					typs = append(typs, fmt.Sprintf("\t\t\t%q: aotreflect.TypeFor[%s](),\n", n, n))
				}
			}
			continue
		}
		if !obj.Exported() {
			continue
		}
		switch o := obj.(type) {
		case *types.Func:
			if o.Type().(*types.Signature).TypeParams() != nil {
				continue
			}
			funcs = append(funcs, fmt.Sprintf("\t\t\t%q: aotreflect.ValueOf(%s),\n", n, n))
			exportedFuncs = append(exportedFuncs, o)
		case *types.TypeName:
			if nt, ok := o.Type().(*types.Named); ok && nt.TypeParams() != nil {
				continue
			}
			if al, ok := o.Type().(*types.Alias); ok && al.TypeParams() != nil {
				continue
			}
			if it, ok := o.Type().Underlying().(*types.Interface); ok && !it.IsMethodSet() {
				continue // constraint interface
			}
			typs = append(typs, fmt.Sprintf("\t\t\t%q: aotreflect.TypeFor[%s](),\n", n, n))
			exportedTypes = append(exportedTypes, o)
		case *types.Var:
			vars = append(vars, fmt.Sprintf("\t\t\t%q: aotreflect.ValueOf(&%s),\n", n, n))
		}
	}
	writeMap := func(field, typ string, entries []string) {
		fmt.Fprintf(&body, "\t\t%s: map[string]aotreflect.%s{\n", field, typ)
		for _, e := range entries {
			body.WriteString(e)
		}
		body.WriteString("\t\t},\n")
	}
	writeMap("Funcs", "Value", funcs)
	writeMap("Types", "Type", typs)
	writeMap("Vars", "Value", vars)
	body.WriteString("\t})\n")
	// Native hooks.
	for _, f := range exportedFuncs {
		s := f.Type().(*types.Signature)
		if gl.hook(&body, pi.p.PkgPath+"."+f.Name(), f.Name(), nil, s) {
			pi.hooks++
		} else {
			pi.noHook = append(pi.noHook, f.Name())
		}
		for i := 0; i < s.Params().Len(); i++ {
			pi.wrapIfaces = append(pi.wrapIfaces, s.Params().At(i).Type())
		}
	}
	for _, tn := range exportedTypes {
		nt, ok := tn.Type().(*types.Named)
		if !ok || tn.IsAlias() {
			continue
		}
		if _, isI := nt.Underlying().(*types.Interface); isI {
			pi.wrapIfaces = append(pi.wrapIfaces, nt)
			continue
		}
		for i := 0; i < nt.NumMethods(); i++ {
			m := nt.Method(i)
			if !m.Exported() {
				continue
			}
			s := m.Type().(*types.Signature)
			recv := s.Recv().Type()
			key := pi.p.PkgPath + "." + tn.Name() + "." + m.Name()
			if _, ptr := recv.(*types.Pointer); ptr {
				key = "(*" + pi.p.PkgPath + "." + tn.Name() + ")." + m.Name()
			}
			if gl.hook(&body, key, m.Name(), recv, s) {
				pi.hooks++
			} else {
				pi.noHook = append(pi.noHook, tn.Name()+"."+m.Name())
			}
			for j := 0; j < s.Params().Len(); j++ {
				pi.wrapIfaces = append(pi.wrapIfaces, s.Params().At(j).Type())
			}
		}
	}
	// Interface wrappers.
	var wraps []types.Type
	for _, t := range pi.wrapIfaces {
		wraps = addWrap(wraps, t, gl)
	}
	var decls bytes.Buffer
	for _, t := range wraps {
		g.wrapSeq++
		name := fmt.Sprintf("aotW%d", g.wrapSeq)
		fmt.Fprintf(&decls, "// %s wraps an interpreted value as %s.\n", name, gl.typ(t))
		gl.wrapperType(&decls, name, methodsOf(t))
		fmt.Fprintf(&body, "\t%s.RegisterWrapper[%s](func(o %s.Obj) %s { return %s{o} })\n", vmName, gl.typ(t), vmName, gl.typ(t), name)
		pi.wrappers++
	}
	body.WriteString("}\n\n")
	// Type switch case lists: one wrapper per case implementing the static
	// type of the switched value and the case interface.
	for _, e := range pi.tsEntries {
		var cases []string
		for _, ct := range e.cases {
			g.wrapSeq++
			name := fmt.Sprintf("aotW%d", g.wrapSeq)
			ms := methodsOf(ct)
			if nonEmptyIface(e.static) {
				for _, sm := range methodsOf(e.static) {
					dup := false
					for _, m := range ms {
						if m.Name() == sm.Name() {
							dup = true
						}
					}
					if !dup {
						ms = append(ms, sm)
					}
				}
			}
			fmt.Fprintf(&decls, "// %s is case %s of a type switch on %s.\n", name, gl.typ(ct), gl.typ(e.static))
			gl.wrapperType(&decls, name, ms)
			cases = append(cases, fmt.Sprintf("\t%s.TSCase[%s](func(o %s.Obj) any { return %s{o} }),\n", vmName, gl.typ(ct), vmName, name))
		}
		fmt.Fprintf(&body, "var %s = %s.NewTSCases(\n%s)\n\n", e.name, vmName, strings.Join(cases, ""))
	}
	if err := g.writeGlueFile(pi, gl, body.Bytes(), decls.Bytes()); err != nil {
		return err
	}
	return g.writeGenericGlue(pi)
}

// writeGenericGlue writes zz_aotgeneric.go, the hand-written hooks of the
// generic functions of the package (generics/<path>/*.go.in), if it has
// some. The glue has no hook for a generic function: a copy has only the
// instances its own code needs, and the program calls them with its own
// types. The hooks register with aotvm.RegisterGeneric. AOTPATH in them is
// the import path of the copies.
func (g *gen) writeGenericGlue(pi *pinfo) error {
	src := genericGlue(pi.p.PkgPath)
	if src == nil {
		return nil
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by aotgen from cmd/aotgen/generics/%s. DO NOT EDIT.\n\n", pi.p.PkgPath)
	if pi.exists != nil {
		fmt.Fprintf(&b, "//go:build %s\n\n", pi.exists)
	}
	b.Write(bytes.ReplaceAll(src, []byte("AOTPATH"), []byte(g.outPath)))
	out, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("%s: generics glue: %v", pi.p.PkgPath, err)
	}
	return os.WriteFile(filepath.Join(pi.dir, "zz_aotgeneric.go"), out, 0o644)
}

func (g *gen) writeGlueFile(pi *pinfo, gl *glue, body, decls []byte) error {
	var b bytes.Buffer
	b.WriteString("// Code generated by aotgen. DO NOT EDIT.\n\n")
	if pi.exists != nil {
		fmt.Fprintf(&b, "//go:build %s\n\n", pi.exists) // as the files of the package (constraints.go)
	}
	fmt.Fprintf(&b, "// AOT glue of %s: see cmd/aotgen.\n\npackage %s\n\nimport (\n", pi.p.PkgPath, pi.p.Name)
	if bytes.Contains(body, []byte("aotreflect.")) || bytes.Contains(decls, []byte("aotreflect.")) {
		b.WriteString("\taotreflect \"reflect\"\n")
	}
	fmt.Fprintf(&b, "\t%s %q\n", vmName, vmPath)
	paths := make([]string, 0, len(gl.imports))
	for p := range gl.imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&b, "\t%s %q\n", gl.imports[p], p)
	}
	b.WriteString(")\n\n")
	b.Write(body)
	b.Write(decls)
	src, err := format.Source(b.Bytes())
	if err != nil {
		src = b.Bytes()
		pi.blockers = append(pi.blockers, "glue does not gofmt: "+err.Error())
	}
	return os.WriteFile(filepath.Join(pi.dir, "zz_aotglue.go"), src, 0o644)
}

// writeAll writes <o>/all: a package importing every generated package.
// The release guard is in aot/all (writeTop): the tree of a release is only
// built by that release (addRelease).
func (g *gen) writeAll() error {
	var b bytes.Buffer
	b.WriteString("// Code generated by aotgen. DO NOT EDIT.\n\n// Package all registers every package generated by aotgen (import it for its side effects).\npackage all\n\nimport (\n")
	for _, pi := range g.sorted() {
		if _, internal := internalParent(pi.p.PkgPath); internal {
			continue // registered by the packages importing it
		}
		if pi.exists != nil {
			continue // in a file with its build constraint (writeAllConstrained)
		}
		if pi.kind == kConvert && (pi.built || pi.buildErr == "") {
			fmt.Fprintf(&b, "\t_ %q\n", pi.aotPath)
		}
	}
	b.WriteString(")\n")
	dir := filepath.Join(g.outDir, "all")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src, err := format.Source(b.Bytes())
	if err != nil {
		return err
	}
	// The copies follow the internals of the standard library of one Go
	// release: vm.Load also compares the patch level (Package.GoVersion).
	return os.WriteFile(filepath.Join(dir, "all.go"), src, 0o644)
}
