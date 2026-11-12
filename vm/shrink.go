package vm

// What a load keeps of the front end (parser, go/types, ssa).
//
// The Machine keeps the ssa.Program: the VM compiles a function from its
// SSA at its first call, and ssa builds the instances of generic functions
// on demand, at any time (Program.MethodValue at run time), from the syntax
// of the generic function and the types.Info of its package. For that,
// every ssa.Function built from source keeps its syntax (Syntax) and a
// pointer to the types.Info of its package. So after a load, the whole
// types.Info of the plugin packages stayed alive (a type for every
// expression, an object for every identifier) and, through its
// FileVersions and Scopes keys, every *ast.File with all its declarations
// and comments.
//
// After the SSA build and the load-time checks (restrict.go, embed.go,
// aotsource.go, which read the whole syntax and types.Info), dropSyntax
// removes what nothing reads any more:
//
//   - every entry of types.Info outside the generic functions (and the
//     methods of generic types): ssa built the rest once and for good.
//     Each map is copied without them (a Go map does not give back its
//     memory when keys are deleted). Scopes, FileVersions and InitOrder go
//     too: ssa reads them only while it creates and builds a package.
//   - the statements of the bodies of the non-generic functions, and of
//     the function literals in them. The compiler of the VM reads the
//     syntax of a function when it compiles it: the closing brace of the
//     body (exitPos), the doc comment (uintptrEscapes, nointerface), and two
//     walks of the body. skipUnsafeOperands looks for unsafe.Sizeof calls
//     whose operand depends on a type parameter: only in generic code,
//     which stays. unevaluatedRange looks for "for range x" loops over an
//     array: a declaration with such a loop keeps its whole syntax.
//   - the name resolution of go/parser (ast.Ident.Obj): it points from an
//     identifier to the whole declaration of the name, for example to the
//     value of a package variable.
//
// The positions stay: they are in the SSA instructions and in the line
// tables of the FileSet (runtime.Caller, panics, stack traces). The SSA of
// all functions stays too. Compiling every function at load time and then
// dropping their SSA does not pay: on the Coraza plugin, the code of the
// 2,516 reachable functions takes 9 MB more, and dropping their SSA
// blocks gives back less than 1 MB (the values and instructions refer to
// each other, and gclive.go reads the blocks at run time).
//
// Measured with cmd/loadmem on the Coraza WAF plugin with the OWASP CRS:
// the live heap after vm.Load goes from 80 to 53 MB.

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// dropSyntax drops the syntax and the type information that the source
// packages src no longer need (see above). It must run after the SSA build
// of src and after every check that reads their syntax or types.Info.
func dropSyntax(src []sourcePkg) {
	for _, p := range src {
		if p.info == nil {
			continue
		}
		pruneBodies(p.files, p.info) // reads the types of range expressions
		shrinkInfo(p.files, p.info)
	}
}

// dropSyntaxAll is dropSyntax for every package of pkgs (and of their
// dependencies) that go list loaded with its syntax.
func dropSyntaxAll(pkgs []*packages.Package) {
	var src []sourcePkg
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.TypesInfo != nil && len(p.Syntax) > 0 {
			src = append(src, sourcePkg{fset: p.Fset, types: p.Types, files: p.Syntax, info: p.TypesInfo})
		}
	})
	dropSyntax(src)
}

// clearObjects removes the name resolution of go/parser (ast.Ident.Obj)
// from the syntax under n: only the load-time checks read it (loadbomb.go).
func clearObjects(n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			id.Obj = nil
		}
		return true
	})
}

// isGenericDecl reports whether fd declares a generic function or a
// method of a generic type.
func isGenericDecl(fd *ast.FuncDecl) bool {
	if fd.Type.TypeParams != nil && len(fd.Type.TypeParams.List) > 0 {
		return true
	}
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return false
	}
	t := fd.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.ParenExpr:
			t = x.X
		case *ast.IndexExpr, *ast.IndexListExpr:
			return true
		default:
			return false
		}
	}
}

// shrinkInfo keeps in info only the entries of the generic function
// declarations of files, from which ssa builds new instances.
func shrinkInfo(files []*ast.File, info *types.Info) {
	type span struct{ pos, end token.Pos }
	var spans []span
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil && isGenericDecl(fd) {
				spans = append(spans, span{fd.Pos(), fd.End()})
			}
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].pos < spans[j].pos })
	generic := func(pos token.Pos) bool {
		i := sort.Search(len(spans), func(i int) bool { return spans[i].end > pos })
		return i < len(spans) && spans[i].pos <= pos
	}
	if len(spans) == 0 {
		generic = nil
	}
	info.Types = keepOnly(info.Types, generic)
	info.Uses = keepOnly(info.Uses, generic)
	info.Defs = keepOnly(info.Defs, generic)
	info.Selections = keepOnly(info.Selections, generic)
	info.Implicits = keepOnly(info.Implicits, generic)
	info.Instances = keepOnly(info.Instances, generic)
	info.Scopes = nil
	info.FileVersions = nil
	info.InitOrder = nil
}

// keepOnly returns a copy of m with only the nodes for which keep reports
// true (nil when there is none, or when keep is nil).
func keepOnly[K interface {
	comparable
	ast.Node
}, V any](m map[K]V, keep func(token.Pos) bool) map[K]V {
	if keep == nil {
		return nil
	}
	n := 0
	for k := range m {
		if keep(k.Pos()) {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	out := make(map[K]V, n)
	for k, v := range m {
		if keep(k.Pos()) {
			out[k] = v
		}
	}
	return out
}

// pruneBodies removes the statements of the bodies of the non-generic
// functions of files, and of the function literals in them and in the
// values of package variables. A declaration with a loop that
// unevaluatedRange may look for keeps its syntax. It also clears the name
// resolution of go/parser in the syntax that stays reachable (the function
// declarations and literals, which ssa keeps); the rest of the files is
// no longer reachable once shrinkInfo has run.
func pruneBodies(files []*ast.File, info *types.Info) {
	for _, f := range files {
		f.Scope = nil
		f.Unresolved = nil
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Body == nil || isGenericDecl(d) {
					clearObjects(d)
					continue
				}
				lits, keep := scanSyntax(d.Body, info)
				if keep {
					clearObjects(d)
					continue
				}
				d.Body.List = nil
				clearObjects(d)
				for _, lit := range lits {
					lit.Body.List = nil
					clearObjects(lit)
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, sp := range d.Specs {
					for _, v := range sp.(*ast.ValueSpec).Values {
						lits, keep := scanSyntax(v, info)
						for _, lit := range lits {
							if !keep {
								lit.Body.List = nil
							}
							clearObjects(lit)
						}
					}
				}
			}
		}
	}
}

// scanSyntax returns the function literals in n, and whether n has a "for
// range x" or "for i := range x" loop (no value variable) over an array, a
// pointer, or an expression of unknown type: unevaluatedRange (panics.go)
// looks for such loops in the syntax of the function it compiles.
func scanSyntax(n ast.Node, info *types.Info) (lits []*ast.FuncLit, arrayRange bool) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			lits = append(lits, n)
		case *ast.RangeStmt:
			if arrayRange || n.Value != nil && !isBlank(n.Value) {
				break
			}
			tv, ok := info.Types[n.X]
			if !ok || tv.Type == nil {
				arrayRange = true
				break
			}
			switch tv.Type.Underlying().(type) {
			case *types.Slice, *types.Map, *types.Chan, *types.Basic, *types.Signature:
			default:
				arrayRange = true
			}
		}
		return true
	})
	return lits, arrayRange
}
