package vm

// Static checks against the load-time bombs of go/types, go/constant and the
// SSA builder.
//
// A few lines of source can describe a type, a constant or a set of generic
// instances whose size is exponential in the number of lines:
//
//	type T1 struct{ a, b T0 }           // or = P[T0, T0], or = func(T0, T0)
//	type T2 struct{ a, b T1 }           // ... 200 lines
//	const S1 = S0 + S0                   // ... 30 lines: 1 GiB
//	func F1[T any]() { F2[P[T, T]]() }   // ... 20 functions
//
// The type is a small graph, but several walks of go/types (validType,
// finiteSize, the hash of instances), of typeutil (the hash of types, during
// the SSA build) and the SSA builder (monomorphization: the names and the
// bodies of the instances) walk it as a tree, without a memo. They run for
// minutes or allocate gigabytes, and go/types cannot be canceled:
// LoadTimeout only stops the wait.
//
// So before go/types checks a package, checkSourceSize measures on the AST:
//   - the expanded size of every type: one node per type literal, aliases
//     replaced by their type, a defined type replaced by its type where it is
//     part of the value (a struct field, an array element, a type argument),
//     and counted as one node behind a pointer, a slice, a map, a channel, a
//     func or an interface, where these walks stop;
//   - the work of validType on the declarations: for each type declaration,
//     its size times its depth (how many named types are nested in each
//     other in its value), summed on the package. A chain of 2000 structs,
//     each with the previous one as a field, has a small size per type but
//     took 11 seconds;
//   - the bytes of the string constants: each constant, and each constant
//     expression of the package, summed on all the packages of the load.
//
// And before the SSA build, checkInstances measures, with the types of the
// checked packages, the generic instances that the SSA builder will create:
// their number, the expanded size of their type arguments, and their work
// (loadinst.go).
//
// The limits are far above real code. Measured on October 21, 2026 on the 86
// plugins of the catalog that load (all their packages, vendored ones
// included): the largest type has 336 nodes (limit maxTypeNodes, 1<<20),
// the largest work of a package is 3437 (maxTypeWork, 1<<29), and the
// largest string constants of a package are 613 bytes (maxConstBytes,
// 1<<26). The checks are linear in the size of the source: every size is
// computed once (memo) and saturates.
//
// This is a filter of the shapes that are known to be slow, not a bound of
// go/types: another slow path of go/types would still run on the load
// goroutine, and only LoadTimeout bounds the wait for it (one load at a time
// per plugin: loadSlots in plugin.go). The GoList loader
// (go/packages) does not run these checks.

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
)

// Limits of the static checks.
const (
	// maxTypeNodes bounds the expanded size of one type.
	maxTypeNodes = 1 << 20
	// maxTypeWork bounds the work of the check of the type declarations of
	// one package: validType of go/types walks the expanded type of each
	// declaration and compares each named type with the named types it is
	// in, so a declaration costs about its size times its depth. 1<<29 is
	// about half a second of go/types.
	maxTypeWork = 1 << 29
	// maxConstBytes bounds the bytes of all the string constants and
	// constant string expressions of one load (all its source packages
	// together): go/types and the SSA builder keep them until the plugin
	// is dropped. When it was a bound per package, 64 packages of 1.3 KiB
	// each kept 1 GiB.
	maxConstBytes = 1 << 26
	// maxInstances bounds the generic instances (functions and types) that
	// the SSA builder creates for the source packages.
	maxInstances = 1 << 16
	// maxInstanceWork bounds the instances times the size of their type
	// arguments, summed on the sites in generic code (loadinst.go): the SSA
	// builder hashes and compares the type arguments of each call of a
	// generic body, for each instance of the body. A library of generic
	// helpers that call each other, used with 300 types (5400 instances),
	// has a work of 11400; the slow chain of loadinst.go just below the
	// limit builds in about one second.
	maxInstanceWork = 1 << 19
	// maxCycleVisits bounds the work of one cycle of generic declarations
	// until its sizes are final (loadinst.go): the types measured that were
	// not in a memo yet, a walk of a type argument with a new environment
	// counting each of its nodes. Real cycles measure a few small types a
	// few times.
	maxCycleVisits = 1 << 22
	// sizeCap is where the sizes saturate: far above every limit, far below
	// an overflow.
	sizeCap = 1 << 40
)

func addCap(a, b int64) int64 {
	if a+b > sizeCap {
		return sizeCap
	}
	return a + b
}

func mulCap(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > sizeCap/b {
		return sizeCap
	}
	return a * b
}

// pairSize is the size of a type in the two kinds of position: where it is
// part of the value (val), and where it is not (ref).
type pairSize struct{ val, ref int64 }

// sizer measures the types and constants of the AST of one package.
type sizer struct {
	fset    *token.FileSet
	imports func(path string) *types.Package // the checked packages it imports (nil: unknown)

	// The names are resolved by go/parser (ast.Ident.Obj; the files must be
	// parsed without parser.SkipObjectResolution): a local name has its
	// declaration, and a name that the file does not declare is one of the
	// package level (types, consts, the first one as in go/types). Looking
	// a name up in every scope made the checks quadratic in the number of
	// local declarations of the same name.
	types   map[string]*ast.TypeSpec        // type declarations of the package level, by name
	consts  map[string]*ast.Ident           // constant declarations of the package level, by name
	constOf map[*ast.Ident]constSpec        // every constant, by the name in its declaration
	files   map[*ast.File]map[string]string // import name -> path, per file
	file    *ast.File                       // the file being measured

	named   map[*ast.TypeSpec]int64 // memo: value size of the non-generic declarations
	aliases map[aliasKey]int64      // memo: size of the non-generic aliases
	generic map[genericKey]pairSize // memo: instances of the generic declarations
	busy    map[*ast.TypeSpec]bool  // declarations being measured (a cycle: invalid type)
	typed   map[typedKey]pairSize   // memo: imported types without type parameters
	visits  int64                   // types measured that were not in a memo
	strs    map[*ast.Ident]int64    // memo: bytes of the constants
	strBusy map[*ast.Ident]bool
	inner   map[*ast.BinaryExpr]bool // sums inside a sum already counted
	depths  map[*ast.TypeSpec]int64  // memo: depth of the declarations
	tdepths map[types.Type]int64     // memo: depth of imported types

	work int64 // size times depth of the type declarations, so far
	err  error // the first limit passed
}

// constSpec is one constant: its name and its value expression (the value
// of an earlier line for the implicit repetition of a const block).
type constSpec struct {
	name *ast.Ident
	val  ast.Expr
}

type genericKey struct {
	spec *ast.TypeSpec
	args string
}

type aliasKey struct {
	spec *ast.TypeSpec
	val  bool
}

// typeEnv holds the sizes of the type parameters of one imported generic
// instance, and the memo of the types measured with it: the memo goes away
// with the env (checkInstances makes a new env each time the sizes of an
// owner grow).
type typeEnv struct {
	params map[*types.TypeParam]pairSize
	memo   map[types.Type]pairSize
}

type typedKey struct {
	t   types.Type
	env *typeEnv
}

func newSizer(fset *token.FileSet, imports func(string) *types.Package) *sizer {
	return &sizer{
		fset: fset, imports: imports,
		types: map[string]*ast.TypeSpec{}, consts: map[string]*ast.Ident{}, constOf: map[*ast.Ident]constSpec{},
		files: map[*ast.File]map[string]string{},
		named: map[*ast.TypeSpec]int64{}, aliases: map[aliasKey]int64{}, generic: map[genericKey]pairSize{},
		busy: map[*ast.TypeSpec]bool{}, typed: map[typedKey]pairSize{},
		strs: map[*ast.Ident]int64{}, strBusy: map[*ast.Ident]bool{}, inner: map[*ast.BinaryExpr]bool{},
		depths: map[*ast.TypeSpec]int64{}, tdepths: map[types.Type]int64{},
	}
}

// checkSourceSize runs the static checks of the AST of a package (see the
// comment of this file). imports returns the checked packages it imports.
// constBytes is the running total of the bytes of the string constants of
// the load: the packages checked before add to it, so that many packages,
// each just under the limit, cannot keep gigabytes of constants together
// (nil: this package alone).
func checkSourceSize(fset *token.FileSet, files []*ast.File, imports func(string) *types.Package, constBytes *int64) error {
	s := newSizer(fset, imports)
	for _, f := range files {
		s.collect(f)
	}
	if constBytes == nil {
		constBytes = new(int64)
	}
	for _, f := range files {
		s.file = f
		ast.Inspect(f, func(n ast.Node) bool {
			if s.err != nil {
				return false
			}
			switch n := n.(type) {
			case *ast.TypeSpec:
				size := s.declSize(n)
				s.checkType(n.Name, size)
				s.work = addCap(s.work, mulCap(size, max(1, s.declDepth(n))))
				if s.err == nil && s.work > maxTypeWork {
					s.err = loadErrorf("vm load: %s: the type declarations of the package are too large to check: types nested too deep in each other",
						s.fset.Position(n.Pos()))
				}
				return false
			case *ast.GenDecl:
				if n.Tok == token.CONST {
					for _, sp := range n.Specs {
						for _, id := range sp.(*ast.ValueSpec).Names {
							*constBytes = addCap(*constBytes, s.constBytes(id))
							s.checkConst(id, *constBytes)
						}
					}
					return false
				}
			case *ast.BinaryExpr:
				if n.Op == token.ADD && !s.inner[n] {
					// The root of a sum: go/constant folds it when its
					// operands are constants. The sums inside it are
					// part of its bound.
					*constBytes = addCap(*constBytes, s.strBytes(n))
					s.checkConst(n, *constBytes)
					ast.Inspect(n, func(c ast.Node) bool {
						if b, ok := c.(*ast.BinaryExpr); ok && b != n && b.Op == token.ADD {
							s.inner[b] = true
						}
						return true
					})
				}
			case *ast.ArrayType, *ast.StructType, *ast.FuncType, *ast.InterfaceType, *ast.MapType, *ast.ChanType:
				s.checkType(n, s.size(n.(ast.Expr), true, nil))
				return false
			case *ast.IndexExpr, *ast.IndexListExpr:
				if s.genericSpec(n.(ast.Expr)) != nil || s.importedGeneric(n.(ast.Expr)) != nil {
					s.checkType(n, s.size(n.(ast.Expr), true, nil))
					return false
				}
			}
			return true
		})
		if s.err != nil {
			return s.err
		}
	}
	return nil
}

// checkType records an error when the type at n is too large.
func (s *sizer) checkType(n ast.Node, size int64) {
	if s.err == nil && size > maxTypeNodes {
		s.err = loadErrorf("vm load: %s: type too large to check: more than %d nodes once its declarations are expanded",
			s.fset.Position(n.Pos()), int64(maxTypeNodes))
	}
}

// checkConst records an error at n when the string constants of the load
// so far (total bytes) are too large.
func (s *sizer) checkConst(n ast.Node, total int64) {
	if s.err == nil && total > maxConstBytes {
		s.err = loadErrorf("vm load: %s: the string constants of the plugin and its packages are too large to check: more than %d bytes",
			s.fset.Position(n.Pos()), int64(maxConstBytes))
	}
}

// collect records the type and constant declarations and the imports of f.
func (s *sizer) collect(f *ast.File) {
	imps := map[string]string{}
	for _, is := range f.Imports {
		path, err := strconv.Unquote(is.Path.Value)
		if err != nil {
			continue
		}
		name := path
		for i := len(path) - 1; i >= 0; i-- {
			if path[i] == '/' {
				name = path[i+1:]
				break
			}
		}
		if is.Name != nil {
			name = is.Name.Name
		}
		imps[name] = path
	}
	s.files[f] = imps
	top := map[ast.Node]bool{}
	for _, d := range f.Decls {
		top[d] = true
	}
	ast.Inspect(f, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || (gd.Tok != token.TYPE && gd.Tok != token.CONST) {
			return true
		}
		var last ast.Expr // implicit repetition of the earlier value
		for _, sp := range gd.Specs {
			if ts, ok := sp.(*ast.TypeSpec); ok {
				if _, dup := s.types[ts.Name.Name]; top[gd] && !dup {
					s.types[ts.Name.Name] = ts
				}
				continue
			}
			vs := sp.(*ast.ValueSpec)
			for i, id := range vs.Names {
				v := last
				if len(vs.Values) > 0 {
					v = nil
					if i < len(vs.Values) {
						v = vs.Values[i]
					}
				}
				s.constOf[id] = constSpec{id, v}
				if _, dup := s.consts[id.Name]; top[gd] && !dup {
					s.consts[id.Name] = id
				}
			}
			if len(vs.Values) > 0 {
				last = vs.Values[len(vs.Values)-1]
			}
		}
		return true
	})
}

// typeSpec returns the type declaration that the name id refers to, or nil
// (a type parameter, a predeclared or imported type).
func (s *sizer) typeSpec(id *ast.Ident) *ast.TypeSpec {
	if id.Obj != nil {
		ts, _ := id.Obj.Decl.(*ast.TypeSpec)
		return ts
	}
	return s.types[id.Name]
}

// constName returns the name, in its declaration, of the constant that id
// refers to, or nil.
func (s *sizer) constName(id *ast.Ident) *ast.Ident {
	if id.Obj == nil {
		return s.consts[id.Name]
	}
	vs, ok := id.Obj.Decl.(*ast.ValueSpec)
	if !ok || id.Obj.Kind != ast.Con {
		return nil
	}
	for _, n := range vs.Names {
		if n.Name == id.Name {
			return n
		}
	}
	return nil
}

// declSize is the size of the declaration ts where it is declared.
func (s *sizer) declSize(ts *ast.TypeSpec) int64 {
	if ts.TypeParams == nil {
		if ts.Assign.IsValid() {
			return s.size(ts.Type, true, nil)
		}
		return s.namedSize(ts)
	}
	env := map[string]pairSize{}
	for _, f := range ts.TypeParams.List {
		for _, id := range f.Names {
			env[id.Name] = pairSize{1, 1}
		}
	}
	return s.size(ts.Type, true, env)
}

// namedSize is the value size of the non-generic declaration ts.
func (s *sizer) namedSize(ts *ast.TypeSpec) int64 {
	if n, ok := s.named[ts]; ok {
		return n
	}
	if s.busy[ts] {
		return 1 // a cycle: go/types reports the invalid type
	}
	s.busy[ts] = true
	n := addCap(1, s.size(ts.Type, true, nil))
	delete(s.busy, ts)
	s.named[ts] = n
	return n
}

// size is the expanded size of the type expression e: where it is part of
// the value (val) or not. env holds the sizes of the type parameters in
// scope (nil: none).
func (s *sizer) size(e ast.Expr, val bool, env map[string]pairSize) int64 {
	switch e := e.(type) {
	case *ast.Ident:
		if p, ok := env[e.Name]; ok {
			if val {
				return p.val
			}
			return p.ref
		}
		if ts := s.typeSpec(e); ts != nil {
			return max(1, s.identSize(ts, val))
		}
		return 1
	case *ast.ParenExpr:
		return s.size(e.X, val, env)
	case *ast.SelectorExpr:
		if obj := s.importedObj(e); obj != nil {
			if tn, ok := obj.(*types.TypeName); ok {
				p := s.typeSize(tn.Type(), nil)
				if val {
					return p.val
				}
				return p.ref
			}
		}
		return 1
	case *ast.StarExpr:
		return addCap(1, s.size(e.X, false, env))
	case *ast.ArrayType:
		if e.Len == nil {
			return addCap(1, s.size(e.Elt, false, env))
		}
		return addCap(1, s.size(e.Elt, val, env))
	case *ast.Ellipsis:
		if e.Elt == nil {
			return 1
		}
		return addCap(1, s.size(e.Elt, false, env))
	case *ast.MapType:
		return addCap(1, addCap(s.size(e.Key, false, env), s.size(e.Value, false, env)))
	case *ast.ChanType:
		return addCap(1, s.size(e.Value, false, env))
	case *ast.FuncType:
		n := addCap(1, s.fieldsSize(e.TypeParams, false, env))
		n = addCap(n, s.fieldsSize(e.Params, false, env))
		return addCap(n, s.fieldsSize(e.Results, false, env))
	case *ast.StructType:
		return addCap(1, s.fieldsSize(e.Fields, val, env))
	case *ast.InterfaceType:
		return addCap(1, s.fieldsSize(e.Methods, false, env))
	case *ast.UnaryExpr: // ~T in a constraint
		return addCap(1, s.size(e.X, false, env))
	case *ast.BinaryExpr: // A | B in a constraint
		return addCap(s.size(e.X, false, env), s.size(e.Y, false, env))
	case *ast.IndexExpr, *ast.IndexListExpr:
		return s.instanceSize(e, val, env)
	}
	return 1
}

// identSize is the size of a name declared by ts.
func (s *sizer) identSize(ts *ast.TypeSpec, val bool) int64 {
	switch {
	case ts.TypeParams != nil:
		return 1 // a generic type without type arguments: the instance counts
	case ts.Assign.IsValid():
		key := aliasKey{ts, val}
		if n, ok := s.aliases[key]; ok {
			return n
		}
		if s.busy[ts] {
			return 1
		}
		s.busy[ts] = true
		n := s.size(ts.Type, val, nil)
		delete(s.busy, ts)
		s.aliases[key] = n
		return n
	case val:
		return s.namedSize(ts)
	}
	return 1
}

// fieldsSize is the size of a field list: each name of a field counts.
func (s *sizer) fieldsSize(fl *ast.FieldList, val bool, env map[string]pairSize) int64 {
	if fl == nil {
		return 0
	}
	var n int64
	for _, f := range fl.List {
		k := int64(max(len(f.Names), 1))
		n = addCap(n, mulCap(k, s.size(f.Type, val, env)))
	}
	return n
}

// indexParts returns the generic expression and the type arguments of an
// instance G[A] or G[A, B].
func indexParts(e ast.Expr) (ast.Expr, []ast.Expr) {
	switch e := e.(type) {
	case *ast.IndexExpr:
		return e.X, []ast.Expr{e.Index}
	case *ast.IndexListExpr:
		return e.X, e.Indices
	}
	return nil, nil
}

// genericSpec returns the generic declaration of the package that the
// instance e instantiates, or nil.
func (s *sizer) genericSpec(e ast.Expr) *ast.TypeSpec {
	x, _ := indexParts(e)
	if id, ok := x.(*ast.Ident); ok {
		if ts := s.typeSpec(id); ts != nil && ts.TypeParams != nil {
			return ts
		}
	}
	return nil
}

// importedGeneric returns the imported generic type that the instance e
// instantiates, or nil.
func (s *sizer) importedGeneric(e ast.Expr) *types.TypeName {
	x, _ := indexParts(e)
	if sel, ok := x.(*ast.SelectorExpr); ok {
		if tn, ok := s.importedObj(sel).(*types.TypeName); ok {
			if n, ok := tn.Type().(*types.Named); ok && n.TypeParams().Len() > 0 {
				return tn
			}
		}
	}
	return nil
}

// instanceSize is the size of the instance e of a generic type: its type
// arguments, and where it is part of the value, the type of the generic
// declaration with these arguments.
func (s *sizer) instanceSize(e ast.Expr, val bool, env map[string]pairSize) int64 {
	_, idx := indexParts(e)
	args := make([]pairSize, len(idx))
	n := int64(1)
	for i, a := range idx {
		args[i] = pairSize{s.size(a, true, env), s.size(a, false, env)}
		n = addCap(n, args[i].ref)
	}
	if !val {
		return n
	}
	if ts := s.genericSpec(e); ts != nil {
		return addCap(n, s.genericBody(ts, args))
	}
	if tn := s.importedGeneric(e); tn != nil {
		named := tn.Type().(*types.Named)
		tenv := &typeEnv{params: map[*types.TypeParam]pairSize{}}
		for i := 0; i < named.TypeParams().Len() && i < len(args); i++ {
			tenv.params[named.TypeParams().At(i)] = args[i]
		}
		return addCap(n, s.typeSize(named.Underlying(), tenv).val)
	}
	// A generic func instance in a type position (an error) or a value index.
	for _, a := range args {
		n = addCap(n, a.val)
	}
	return n
}

// genericBody is the value size of the type of the generic declaration ts
// with the type arguments args.
func (s *sizer) genericBody(ts *ast.TypeSpec, args []pairSize) int64 {
	key := genericKey{ts, ""}
	for _, a := range args {
		key.args += strconv.FormatInt(a.val, 36) + "," + strconv.FormatInt(a.ref, 36) + ";"
	}
	if p, ok := s.generic[key]; ok {
		return p.val
	}
	if s.busy[ts] {
		return 1
	}
	s.busy[ts] = true
	env := map[string]pairSize{}
	i := 0
	for _, f := range ts.TypeParams.List {
		for _, id := range f.Names {
			if i < len(args) {
				env[id.Name] = args[i]
			}
			i++
		}
	}
	n := s.size(ts.Type, true, env)
	delete(s.busy, ts)
	s.generic[key] = pairSize{val: n}
	return n
}

// importedObj returns the object pkg.Name of an imported package, or nil.
func (s *sizer) importedObj(sel *ast.SelectorExpr) types.Object {
	x, ok := sel.X.(*ast.Ident)
	if !ok || s.imports == nil || s.file == nil {
		return nil
	}
	path, ok := s.files[s.file][x.Name]
	if !ok {
		return nil
	}
	p := s.imports(path)
	if p == nil {
		return nil
	}
	return p.Scope().Lookup(sel.Sel.Name)
}

// typeSize is the size of a checked type, as size is for the AST. env
// holds the sizes of the type parameters (nil: none).
// Each call counts in s.visits, also when the type is in the memo: a
// struct of many fields of the same type costs one call per field.
func (s *sizer) typeSize(t types.Type, env *typeEnv) pairSize {
	s.visits++
	memo, key := s.typed, typedKey{t, nil}
	if env != nil {
		if env.memo == nil {
			env.memo = map[types.Type]pairSize{}
		}
		if p, ok := env.memo[t]; ok {
			return p
		}
		env.memo[t] = pairSize{1, 1} // a cycle through a named type
		p := s.typeSize0(t, env)
		env.memo[t] = p
		return p
	}
	if p, ok := memo[key]; ok {
		return p
	}
	memo[key] = pairSize{1, 1} // a cycle through a named type
	p := s.typeSize0(t, env)
	memo[key] = p
	return p
}

func (s *sizer) typeSize0(t types.Type, env *typeEnv) pairSize {
	one := pairSize{1, 1}
	ref := func(ts ...types.Type) pairSize { // children that are not part of the value
		n := int64(1)
		for _, t := range ts {
			n = addCap(n, s.typeSize(t, env).ref)
		}
		return pairSize{n, n}
	}
	switch t := t.(type) {
	case *types.TypeParam:
		if env != nil {
			if p, ok := env.params[t]; ok {
				return p
			}
		}
		return one
	case *types.Alias:
		return s.typeSize(types.Unalias(t), env)
	case *types.Named:
		args := t.TypeArgs()
		if args.Len() == 0 {
			env = nil // no type parameter of env inside: share the memo
		}
		n := int64(1)
		for i := 0; i < args.Len(); i++ {
			n = addCap(n, s.typeSize(args.At(i), env).ref)
		}
		u := s.typeSize(t.Underlying(), env).val
		return pairSize{addCap(n, u), n}
	case *types.Pointer:
		return ref(t.Elem())
	case *types.Slice:
		return ref(t.Elem())
	case *types.Map:
		return ref(t.Key(), t.Elem())
	case *types.Chan:
		return ref(t.Elem())
	case *types.Array:
		p := s.typeSize(t.Elem(), env)
		return pairSize{addCap(1, p.val), addCap(1, p.ref)}
	case *types.Struct:
		var v, r int64 = 1, 1
		for i := 0; i < t.NumFields(); i++ {
			p := s.typeSize(t.Field(i).Type(), env)
			v, r = addCap(v, p.val), addCap(r, p.ref)
		}
		return pairSize{v, r}
	case *types.Signature:
		var ts []types.Type
		for _, tup := range []*types.Tuple{t.Params(), t.Results()} {
			for i := 0; i < tup.Len(); i++ {
				ts = append(ts, tup.At(i).Type())
			}
		}
		return ref(ts...)
	case *types.Interface:
		var ts []types.Type
		for i := 0; i < t.NumExplicitMethods(); i++ {
			ts = append(ts, t.ExplicitMethod(i).Type())
		}
		for i := 0; i < t.NumEmbeddeds(); i++ {
			ts = append(ts, t.EmbeddedType(i))
		}
		return ref(ts...)
	case *types.Union:
		var ts []types.Type
		for i := 0; i < t.Len(); i++ {
			ts = append(ts, t.Term(i).Type())
		}
		return ref(ts...)
	}
	return one
}

// declDepth is the depth of the declaration ts: how many named types are
// nested in each other in its value, at most. It is an upper bound: the
// type parameters count as zero, and an instance adds the depth of its
// arguments to the depth of its generic type.
func (s *sizer) declDepth(ts *ast.TypeSpec) int64 {
	if d, ok := s.depths[ts]; ok {
		return d
	}
	if s.busy[ts] {
		return 0 // a cycle: invalid type
	}
	s.busy[ts] = true
	d := s.depth(ts.Type)
	if !ts.Assign.IsValid() {
		d = addCap(d, 1)
	}
	delete(s.busy, ts)
	s.depths[ts] = d
	return d
}

// depth is the depth of the type expression e in a value position.
func (s *sizer) depth(e ast.Expr) int64 {
	switch e := e.(type) {
	case *ast.Ident:
		if ts := s.typeSpec(e); ts != nil {
			return s.declDepth(ts)
		}
		return 0
	case *ast.ParenExpr:
		return s.depth(e.X)
	case *ast.SelectorExpr:
		if tn, ok := s.importedObj(e).(*types.TypeName); ok {
			return s.typeDepth(tn.Type())
		}
	case *ast.ArrayType:
		if e.Len != nil {
			return s.depth(e.Elt)
		}
	case *ast.StructType:
		var d int64
		for _, f := range e.Fields.List {
			d = max(d, s.depth(f.Type))
		}
		return d
	case *ast.IndexExpr, *ast.IndexListExpr:
		x, args := indexParts(e)
		d := s.depth(x)
		var a int64
		for _, arg := range args {
			a = max(a, s.depth(arg))
		}
		return addCap(d, a)
	}
	return 0 // pointers, slices, maps, channels, funcs, interfaces
}

// typeDepth is depth for a checked type.
func (s *sizer) typeDepth(t types.Type) int64 {
	if d, ok := s.tdepths[t]; ok {
		return d
	}
	s.tdepths[t] = 0 // a cycle through a named type
	var d int64
	switch t := t.(type) {
	case *types.Alias:
		d = s.typeDepth(types.Unalias(t))
	case *types.Named:
		d = addCap(1, s.typeDepth(t.Underlying()))
	case *types.Array:
		d = s.typeDepth(t.Elem())
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			d = max(d, s.typeDepth(t.Field(i).Type()))
		}
	}
	s.tdepths[t] = d
	return d
}

// constBytes is the bound of the bytes of the constant id.
func (s *sizer) constBytes(id *ast.Ident) int64 {
	if n, ok := s.strs[id]; ok {
		return n
	}
	if s.strBusy[id] {
		return 0 // a cycle: go/types reports it
	}
	s.strBusy[id] = true
	var n int64
	if c, ok := s.constOf[id]; ok && c.val != nil {
		n = s.strBytes(c.val)
	}
	delete(s.strBusy, id)
	s.strs[id] = n
	return n
}

// strBytes bounds the bytes of the string value of the constant expression
// e (0 when it is not a string). A name of a constant counts as the
// constant it refers to.
func (s *sizer) strBytes(e ast.Expr) int64 {
	switch e := e.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			return int64(len(e.Value))
		case token.CHAR:
			return 4
		}
	case *ast.Ident:
		if c := s.constName(e); c != nil {
			return s.constBytes(c)
		}
		return 0
	case *ast.SelectorExpr:
		if c, ok := s.importedObj(e).(*types.Const); ok && c.Val().Kind() == constant.String {
			return int64(len(constant.StringVal(c.Val())))
		}
	case *ast.ParenExpr:
		return s.strBytes(e.X)
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			return addCap(s.strBytes(e.X), s.strBytes(e.Y))
		}
	case *ast.CallExpr:
		// A conversion string(x) or T(x): the bytes of x, 4 for a rune.
		n := int64(4)
		for _, a := range e.Args {
			n = addCap(n, s.strBytes(a))
		}
		return n
	}
	return 0
}
