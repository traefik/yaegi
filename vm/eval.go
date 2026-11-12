package vm

// Eval mode: run Go source one input at a time, as a REPL does (yaegi's
// interp.Eval). go/types and go/ssa work on whole packages, so each input
// becomes a new package of the same SSA program, run on the same machine:
//
//   - The declarations of an input (func, type, const, and var when the
//     input has no statement) are the package-level declarations of a new
//     package.
//   - Its statements are the body of a function of that package, which runs
//     once. A variable declared at the top of the statements (x := e,
//     var x = e) must stay visible to the next inputs: it becomes a global
//     of a "holder" package (a package with no code), and its declaration
//     becomes an assignment. Its type comes from a first type check of the
//     input with the variable still local.
//   - The value of a last expression statement is stored in result globals
//     of the holder package, and returned as Objects.
//   - The objects declared by earlier inputs are inserted in the scope of
//     the new package before its type check. All these packages have the same
//     path: go/types compares the packages of unexported names by path, so
//     the fields and methods of a type of an earlier input stay visible.
//   - The imports of earlier inputs are added to each new input.
//
// Limits of the prototype:
//   - A method must be declared in the same input as its type (Go has no
//     methods on a type of another package, and go/types panics on this
//     layout).
//   - An input with declarations and statements runs as two steps: the
//     declarations first. When the statements fail to type check, the
//     declarations stay.
//   - A variable redeclared at the top of the statements cannot use the
//     previous variable of the same name in the same input (x := x + 1).
//   - A redefined function or variable does not change the code of earlier
//     inputs: that code still uses the old one.
//   - Only standard library packages can be imported.
//   - Each input adds packages to the program: memory grows with the
//     session.

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/ssa"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// ErrIncomplete is the error of Session.Eval for an input that ends inside
// brackets, a string or a comment: a REPL reads more lines and tries again.
var ErrIncomplete = errors.New("incomplete input")

// evalPath is the import path of the packages of a Session (see above).
const evalPath = "repl"

// The names that a Session generates. They start with _eval, which a user
// name may also do: the holder of a result is never visible to later inputs.
const (
	evalRunName    = "_eval_run"
	evalResultName = "_eval_r"
	evalFileName   = "eval"
)

// Session runs Go source one input at a time (Eval), on one machine: each
// input sees the declarations, variables and imports of the earlier ones.
// The methods of a Session must not be called concurrently; goroutines
// started by an input keep running during the next ones.
type Session struct {
	mu   sync.Mutex
	opts Options
	std  *stdtypes.Universe
	prog *ssa.Program
	m    *Machine
	p    *Plugin // the plugin of the Objects of the session (Pkg: the last input)

	imports map[string]string       // local name ("_path", ".path" for blank and dot imports) -> path
	visible map[string]types.Object // the objects declared by earlier inputs
	results []*types.Var            // the result globals of the last input (Sprint)
}

// NewSession returns a new Session that runs inputs with opts, as Load runs
// a plugin. Only the options about the machine and the restrictions apply
// (there is no module to load).
func NewSession(opts Options) (*Session, error) {
	if !stdtypes.Available() {
		return nil, fmt.Errorf("vm: yaegi v2 has no standard library types for %s/%s (only for %s)",
			runtime.GOOS, runtime.GOARCH, strings.Join(stdtypes.Platforms, ", "))
	}
	// With Options.StaleAOTFallback and copies that cannot run here, the
	// session runs in default mode (aotGate); a Session has no notes: the
	// host sees it with CheckAOT.
	var err error
	opts.AOT, opts.Modes, _, err = aotGate(opts.hooks.hostGoVersion(), opts.AOT, opts.Modes, opts.AllowStaleAOT, opts.StaleAOTFallback)
	if err != nil {
		return nil, err
	}
	std, err := stdtypes.Load()
	if err != nil {
		return nil, err
	}
	prog := ssa.NewProgram(token.NewFileSet(), ssa.InstantiateGenerics)
	prog.CreatePackage(types.Unsafe, nil, nil, true)
	// Every standard library package: an input may import any of them. The
	// packages with embedded sources (cmp, iter, slices, maps) give the
	// bodies of their generic functions, as in loadSource.
	interp := map[*types.Package]bool{}
	var stdSrc []*ssa.Package
	pkgs := std.Packages()
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path() < pkgs[j].Path() })
	for _, tp := range pkgs {
		if tp == types.Unsafe || prog.Package(tp) != nil {
			continue
		}
		if s := std.Source(tp.Path()); s != nil {
			sp := prog.CreatePackage(tp, s.Files, s.Info, true)
			stdSrc = append(stdSrc, sp)
			if InterpretedStd[tp.Path()] {
				interp[tp] = true
			}
			continue
		}
		prog.CreatePackage(tp, nil, nil, true)
	}
	for _, sp := range stdSrc {
		sp.Build()
	}
	// The packages of the standard library that have AOT copies run as when
	// a plugin imports one of the triggers (stdAOTModes): an input may
	// import fmt or encoding/json at any time, and the modes of a machine
	// are set when it starts.
	if fmtPkg, err := std.Import("fmt"); err == nil {
		trigger := types.NewPackage(evalPath, "repl")
		trigger.SetImports([]*types.Package{fmtPkg})
		opts.Modes = stdAOTModes(opts.Modes, opts.AOT, []sourcePkg{{types: trigger}}, opts.hooks.hostGoVersion(), opts.AllowStaleAOT,
			func(p string) bool { return InterpretedStd[p] }, !opts.realStd())
	}
	// The machine is made before the inputs: one may import reflect later,
	// and the compiled code depends on the VM reflect (ownReflect).
	opts.vmReflect = true
	m := opts.newMachine(prog, interp, isEvalPkg)
	m.evalInterp = new(sync.Map)
	m.track = true // the traceback of a panic shows where it happened
	if !opts.restricted() {
		m.MaxStack, m.MaxDepth = runMaxStack, runMaxDepth
	}
	s := &Session{
		opts:    opts,
		std:     std,
		prog:    prog,
		m:       m,
		p:       &Plugin{M: m},
		imports: map[string]string{},
		visible: map[string]types.Object{},
	}
	return s, nil
}

// isEvalPkg reports whether p is a package of a Session.
func isEvalPkg(p *types.Package) bool { return p.Path() == evalPath }

// Import implements types.Importer for the inputs: the standard library only.
func (s *Session) Import(path string) (*types.Package, error) {
	if s.std.Has(path) {
		return s.std.Import(path)
	}
	return nil, fmt.Errorf("package %s: an input can only import packages of the standard library", path)
}

// Eval runs the Go source src: declarations, statements, or both. It
// returns the values of src when its last statement is an expression (one
// Object per result of a call), else nil. A call of a print function of fmt
// gives no values. A panic of the input is returned
// as an error; ErrIncomplete means that src ends inside brackets, a string
// or a comment.
func (s *Session) Eval(ctx context.Context, src string) (res []*Object, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			err = panicErr("vm eval", r)
		}
	}()
	chunks, err := splitInput(src)
	if err != nil {
		return nil, err
	}
	imports := map[string]string{}
	for k, v := range s.imports {
		imports[k] = v
	}
	var decls, stmts []chunk
	for _, c := range chunks {
		switch c.kind {
		case chunkImport:
			if err := s.addImports(imports, src[c.off:c.end]); err != nil {
				return nil, err
			}
		case chunkDecl:
			decls = append(decls, c)
		case chunkStmt:
			stmts = append(stmts, c)
		}
	}
	if len(decls) > 0 || len(stmts) == 0 {
		if _, err := s.step(ctx, src, imports, decls, nil); err != nil {
			return nil, err
		}
	}
	if len(stmts) > 0 {
		return s.step(ctx, src, imports, nil, stmts)
	}
	return nil, nil
}

// chunkKind is the kind of a top-level part of an input.
type chunkKind int

const (
	chunkStmt chunkKind = iota
	chunkDecl
	chunkVar // a var declaration: a statement when the input has statements
	chunkImport
	chunkPackage // a package clause, ignored
)

// chunk is a top-level part of an input: a declaration or a statement,
// src[off:end].
type chunk struct {
	kind     chunkKind
	off, end int
}

// splitInput cuts src into its top-level parts, at the semicolons (written
// or inserted at the end of a line) outside brackets.
func splitInput(src string) ([]chunk, error) {
	fset := token.NewFileSet()
	file := fset.AddFile(evalFileName, -1, len(src))
	var errs scanner.ErrorList
	var sc scanner.Scanner
	sc.Init(file, []byte(src), func(pos token.Position, msg string) { errs.Add(pos, msg) }, 0)
	var chunks []chunk
	var toks []token.Token
	depth, start := 0, -1
	end := func(off int) {
		k := classify(toks)
		if k != chunkPackage {
			chunks = append(chunks, chunk{kind: k, off: start, end: off})
		}
		start = -1
	}
	for {
		pos, tok, _ := sc.Scan()
		if tok == token.EOF {
			break
		}
		off := file.Offset(pos)
		if start < 0 {
			if tok == token.SEMICOLON {
				continue
			}
			start, toks = off, toks[:0]
		}
		toks = append(toks, tok)
		switch tok {
		case token.LPAREN, token.LBRACK, token.LBRACE:
			depth++
		case token.RPAREN, token.RBRACK, token.RBRACE:
			depth--
		case token.SEMICOLON:
			if depth == 0 {
				end(off)
			}
		}
	}
	for _, e := range errs {
		if strings.Contains(e.Msg, "not terminated") {
			return nil, ErrIncomplete
		}
	}
	if err := errs.Err(); err != nil {
		return nil, err
	}
	if depth > 0 {
		return nil, ErrIncomplete
	}
	if start >= 0 {
		end(len(src))
	}
	hasStmt := false
	for _, c := range chunks {
		hasStmt = hasStmt || c.kind == chunkStmt
	}
	for i := range chunks {
		if chunks[i].kind == chunkVar {
			chunks[i].kind = chunkDecl
			if hasStmt {
				chunks[i].kind = chunkStmt
			}
		}
	}
	return chunks, nil
}

// classify returns the kind of the top-level part with the tokens toks.
func classify(toks []token.Token) chunkKind {
	switch toks[0] {
	case token.PACKAGE:
		return chunkPackage
	case token.IMPORT:
		return chunkImport
	case token.TYPE, token.CONST:
		return chunkDecl
	case token.VAR:
		return chunkVar
	case token.FUNC:
		// func f(...), func (r T) m(...), func[...]: declarations;
		// func(...) {...}(...): a function literal in a statement.
		if len(toks) > 1 && toks[1] == token.IDENT {
			return chunkDecl
		}
		if len(toks) > 1 && toks[1] == token.LPAREN {
			depth := 0
			for i := 1; i < len(toks); i++ {
				switch toks[i] {
				case token.LPAREN:
					depth++
				case token.RPAREN:
					depth--
				}
				if depth == 0 {
					if i+2 < len(toks) && toks[i+1] == token.IDENT && (toks[i+2] == token.LPAREN || toks[i+2] == token.LBRACK) {
						return chunkDecl
					}
					break
				}
			}
		}
	}
	return chunkStmt
}

// addImports adds the imports of the import declaration text to imports.
func (s *Session) addImports(imports map[string]string, text string) error {
	f, err := parser.ParseFile(token.NewFileSet(), evalFileName, "package repl; "+text, parser.ImportsOnly)
	if err != nil {
		return err
	}
	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, "`\"")
		tp, err := s.Import(path)
		if err != nil {
			return err
		}
		name := tp.Name()
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "_" || name == "." {
			name += path
		}
		imports[name] = path
	}
	return nil
}

// lineDirective returns the /*line*/ comment that gives the position of
// src[off:] to the next token of a generated file.
func lineDirective(src string, off int) string {
	line := 1 + strings.Count(src[:off], "\n")
	col := off - strings.LastIndex(src[:off], "\n")
	return fmt.Sprintf("/*line %s:%d:%d*/", evalFileName, line, col)
}

// genFile returns the source of the package of one step: the imports, the
// declarations decls, and the statements stmts in the function _eval_run.
func genFile(src string, imports map[string]string, decls, stmts []chunk) string {
	var b strings.Builder
	b.WriteString("package repl\n")
	names := make([]string, 0, len(imports))
	for name := range imports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := imports[name]
		switch {
		case strings.HasPrefix(name, "_"+path), strings.HasPrefix(name, "."+path):
			name = name[:1]
		}
		fmt.Fprintf(&b, "import %s %q\n", name, path)
	}
	for _, c := range decls {
		b.WriteString("\n" + lineDirective(src, c.off) + src[c.off:c.end] + "\n")
	}
	if len(stmts) > 0 {
		b.WriteString("\nfunc " + evalRunName + "() {")
		for _, c := range stmts {
			b.WriteString("\n" + lineDirective(src, c.off) + src[c.off:c.end] + ";")
		}
		b.WriteString("\n}\n")
	}
	return b.String()
}

// step type checks, builds and runs one package: the declarations decls,
// or the statements stmts.
func (s *Session) step(ctx context.Context, src string, imports map[string]string, decls, stmts []chunk) ([]*Object, error) {
	gen := genFile(src, imports, decls, stmts)
	var holder *types.Package
	var holders, results []*types.Var
	if len(stmts) > 0 {
		var err error
		if gen, holder, holders, results, err = s.hoist(gen); err != nil {
			return nil, err
		}
	}
	fset := s.prog.Fset
	f, err := parser.ParseFile(fset, evalFileName, gen, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if err := s.checkMethods(fset, f); err != nil {
		return nil, err
	}
	var extra []types.Object
	for _, v := range holders {
		extra = append(extra, v)
	}
	for _, v := range results {
		extra = append(extra, v)
	}
	tp, info, err := s.check(fset, f, extra, false)
	if err != nil {
		return nil, err
	}
	files := []*ast.File{f}
	if err := s.opts.check([]sourcePkg{{fset: fset, types: tp, files: files, info: info}}); err != nil {
		return nil, err
	}
	var hs *ssa.Package
	if holder != nil {
		hs = s.prog.CreatePackage(holder, nil, nil, false)
		s.m.evalInterp.Store(holder, true)
	}
	sp := s.prog.CreatePackage(tp, files, info, false)
	sp.Build()
	s.m.evalInterp.Store(tp, true)
	s.p.Pkg = sp
	if err := s.opts.preflight(s.p, func() *Machine { return s.m }, isEvalPkg); err != nil {
		return nil, err
	}
	// The input is accepted: its declarations and imports are visible to
	// the next inputs, also when it panics.
	s.results = results
	for _, name := range tp.Scope().Names() {
		if obj := tp.Scope().Lookup(name); obj.Pkg() == tp && name != "_" && name != evalRunName {
			s.visible[name] = obj
		}
	}
	for _, v := range holders {
		s.visible[v.Name()] = v
	}
	s.imports = imports
	m := s.m
	m.runInits(ctx, m.initOrder(sp))
	if fn := sp.Func(evalRunName); fn != nil {
		m.callTop(m.cancelCtx(ctx), m.funcOf(fn), nil, nil)
	}
	var out []*Object
	for _, v := range results {
		g := hs.Members[v.Name()].(*ssa.Global)
		cell := m.globalPtr(g).R.(*Value)
		out = append(out, &Object{p: s.p, v: *cell, t: v.Type()})
	}
	return out, nil
}

// check type checks the file f of a new package of the session. The
// objects of the earlier inputs are in the scope of the package, except the
// ones that f declares again, and the objects of extra. With local, the
// errors about unused local variables are left out (the first check of
// hoist, where the variables of the top level are still local).
func (s *Session) check(fset *token.FileSet, f *ast.File, extra []types.Object, local bool) (tp *types.Package, info *types.Info, err error) {
	declared := map[string]bool{}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				declared[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					declared[spec.Name.Name] = true
				case *ast.ValueSpec:
					for _, n := range spec.Names {
						declared[n.Name] = true
					}
				}
			}
		}
	}
	scope := map[string]types.Object{}
	for name, obj := range s.visible {
		if !declared[name] {
			scope[name] = obj
		}
	}
	for _, obj := range extra {
		scope[obj.Name()] = obj
	}
	tp = types.NewPackage(evalPath, "repl")
	for _, obj := range scope {
		tp.Scope().Insert(obj)
	}
	info = &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Implicits: map[ast.Node]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Scopes: map[ast.Node]*types.Scope{},
		Instances: map[*ast.Ident]types.Instance{}, FileVersions: map[*ast.File]string{},
	}
	var errs []error
	conf := types.Config{
		Importer: s,
		Sizes:    types.SizesFor("gc", runtime.GOARCH),
		Error: func(err error) {
			if te, ok := err.(types.Error); ok {
				// The imports of earlier inputs are in every input.
				if te.Soft && strings.Contains(te.Msg, "imported and not used") {
					return
				}
				// The variables of the top level become globals, and a
				// last expression is stored in a result (hoist).
				if local && (te.Soft && strings.Contains(te.Msg, "declared and not used") || strings.Contains(te.Msg, "is not used")) {
					return
				}
				// go/types compares the packages by pointer here, not by
				// path as for the other unexported names: a struct literal
				// without keys of a type of an earlier input is valid.
				if strings.Contains(te.Msg, "implicit assignment to unexported field") && strings.Contains(te.Msg, " struct literal of type "+evalPath+".") {
					return
				}
			}
			if len(errs) < 10 {
				errs = append(errs, err)
			}
		},
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				errs = append(errs, fmt.Errorf("type check: %v", r))
			}
		}()
		_ = types.NewChecker(&conf, fset, tp, info).Files([]*ast.File{f})
	}()
	if len(errs) > 0 {
		return nil, nil, &LoadError{Msg: errors.Join(errs...).Error()}
	}
	return tp, info, nil
}

// checkMethods refuses a method of a type of an earlier input (see the
// limits above): go/types panics on it.
func (s *Session) checkMethods(fset *token.FileSet, f *ast.File) error {
	local := map[string]bool{}
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
			for _, spec := range gd.Specs {
				local[spec.(*ast.TypeSpec).Name.Name] = true
			}
		}
	}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		t := fd.Recv.List[0].Type
		for {
			switch e := t.(type) {
			case *ast.StarExpr:
				t = e.X
				continue
			case *ast.ParenExpr:
				t = e.X
				continue
			case *ast.IndexExpr:
				t = e.X
				continue
			case *ast.IndexListExpr:
				t = e.X
				continue
			}
			break
		}
		id, ok := t.(*ast.Ident)
		if !ok || local[id.Name] || s.visible[id.Name] == nil {
			continue
		}
		return &LoadError{Msg: fmt.Sprintf("%s: method %s of type %s, declared in an earlier input: declare a type and its methods in the same input",
			fset.Position(fd.Name.Pos()), fd.Name.Name, id.Name)}
	}
	return nil
}

// hoist makes the variables declared at the top of the statements of gen
// globals of a holder package, and stores the value of a last expression
// statement in result globals. It type checks gen once with the variables
// still local, to know their types, and returns the source with their
// declarations made assignments.
func (s *Session) hoist(gen string) (string, *types.Package, []*types.Var, []*types.Var, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, evalFileName, gen, parser.ParseComments)
	if err != nil {
		return "", nil, nil, nil, err
	}
	tp, info, err := s.check(fset, f, nil, true)
	if err != nil {
		return "", nil, nil, nil, err
	}
	var body *ast.BlockStmt
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == evalRunName {
			body = fd.Body
		}
	}
	tf := fset.File(f.Pos())
	off := func(p token.Pos) int { return tf.Offset(p) }
	type edit struct {
		off, end int
		text     string
	}
	var edits []edit
	holder := types.NewPackage(evalPath, "repl")
	var holders, results []*types.Var
	defined := map[string]bool{}
	define := func(id *ast.Ident) {
		obj, ok := info.Defs[id].(*types.Var)
		if !ok || id.Name == "_" || defined[id.Name] {
			return
		}
		defined[id.Name] = true
		v := types.NewVar(id.Pos(), holder, id.Name, obj.Type())
		holder.Scope().Insert(v)
		holders = append(holders, v)
	}
	for i, st := range body.List {
		switch st := st.(type) {
		case *ast.AssignStmt:
			if st.Tok != token.DEFINE {
				continue
			}
			for _, l := range st.Lhs {
				if id, ok := l.(*ast.Ident); ok {
					define(id)
				}
			}
			edits = append(edits, edit{off(st.TokPos), off(st.TokPos) + 2, " ="})
		case *ast.DeclStmt:
			gd, ok := st.Decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			var parts []string
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				var names []string
				for _, n := range vs.Names {
					define(n)
					names = append(names, n.Name)
				}
				if len(vs.Values) > 0 {
					parts = append(parts, strings.Join(names, ", ")+" = "+gen[off(vs.Values[0].Pos()):off(vs.Values[len(vs.Values)-1].End())])
				}
			}
			edits = append(edits, edit{off(gd.Pos()), off(gd.End()), strings.Join(parts, "; ")})
		case *ast.ExprStmt:
			if i != len(body.List)-1 || isFmtPrint(info, st.X) {
				continue
			}
			tv := info.Types[st.X]
			var ts []types.Type
			switch {
			case tv.IsVoid(), tv.IsType(), tv.Type == nil:
			case isTuple(tv.Type):
				for j := 0; j < tv.Type.(*types.Tuple).Len(); j++ {
					ts = append(ts, tv.Type.(*types.Tuple).At(j).Type())
				}
			default:
				t := types.Default(tv.Type)
				if b, ok := t.(*types.Basic); ok && b.Kind() == types.UntypedNil {
					continue
				}
				ts = append(ts, t)
			}
			var names []string
			for j, t := range ts {
				v := types.NewVar(token.NoPos, holder, fmt.Sprintf("%s%d", evalResultName, j), t)
				holder.Scope().Insert(v)
				results = append(results, v)
				names = append(names, v.Name())
			}
			if len(names) > 0 {
				edits = append(edits, edit{off(st.Pos()), off(st.Pos()), strings.Join(names, ", ") + " = "})
			}
		}
	}
	// x := x + 1: the new x is a global of the holder package, which the
	// x on the right can no longer name.
	for id, obj := range info.Uses {
		if obj.Pkg() != tp && defined[obj.Name()] && s.visible[obj.Name()] == obj {
			return "", nil, nil, nil, &LoadError{Msg: fmt.Sprintf("%s: %s is declared again in this input, which also uses the previous %s: use two inputs",
				fset.Position(id.Pos()), obj.Name(), obj.Name())}
		}
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].off > edits[j].off })
	for _, e := range edits {
		gen = gen[:e.off] + e.text + gen[e.end:]
	}
	if len(holders) == 0 && len(results) == 0 {
		holder = nil
	}
	return gen, holder, holders, results, nil
}

// isFmtPrint reports whether e calls a print function of package fmt
// (Println, Fprintf...): its values (a count and an error) are not results
// of the input, the call has written what it had to.
func isFmtPrint(info *types.Info, e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	var id *ast.Ident
	switch fn := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = fn
	case *ast.SelectorExpr:
		id = fn.Sel
	}
	if id == nil {
		return false
	}
	f, ok := info.Uses[id].(*types.Func)
	return ok && f.Pkg() != nil && f.Pkg().Path() == "fmt" &&
		(strings.HasPrefix(f.Name(), "Print") || strings.HasPrefix(f.Name(), "Fprint")) // not Sprint: its result is the value
}

// Sprint returns the values of the last input (see Eval), as
// fmt.Sprintln writes them in the session: with the methods of their types
// (String, Error, Format), which a native value of Object.Interface does
// not have. It runs a small package of its own.
func (s *Session) Sprint(ctx context.Context) (str string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			err = panicErr("vm eval", r)
		}
	}()
	if len(s.results) == 0 {
		return "", nil
	}
	var names []string
	extra := make([]types.Object, len(s.results))
	for i, v := range s.results {
		names = append(names, v.Name())
		extra[i] = v
	}
	gen := "package repl\nimport _eval_fmt \"fmt\"\nfunc _eval_sprint() string { return _eval_fmt.Sprintln(" + strings.Join(names, ", ") + ") }\n"
	fset := s.prog.Fset
	f, err := parser.ParseFile(fset, evalFileName, gen, 0)
	if err != nil {
		return "", err
	}
	tp, info, err := s.check(fset, f, extra, false)
	if err != nil {
		return "", err
	}
	sp := s.prog.CreatePackage(tp, []*ast.File{f}, info, false)
	sp.Build()
	s.m.evalInterp.Store(tp, true)
	m := s.m
	m.runInits(ctx, m.initOrder(sp))
	out := m.callTop(m.cancelCtx(ctx), m.funcOf(sp.Func("_eval_sprint")), nil, nil)
	str, _ = out.R.(string)
	return str, nil
}

func isTuple(t types.Type) bool {
	_, ok := t.(*types.Tuple)
	return ok
}
