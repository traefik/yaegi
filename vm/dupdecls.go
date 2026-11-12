package vm

// Options.IgnoreDuplicateDecls: a package that declares the same function,
// method, type or constant twice, with the same text, in two of its files
// (a copy of main.go left as adm.go) loads. gc refuses it ("F redeclared in
// this block"), but yaegi runs it, so a plugin tested only with yaegi may
// have such copies.
//
// yaegi has no single rule for a name declared twice: it keeps the last
// function, constant and variable, but the first method, and it runs the
// initializers of both variables. yaegi v2 does not try to copy this. It makes
// the package valid before go/types checks it: a declaration is removed
// only when an earlier declaration of the package has the same names and the
// same text, in a file with the same imports and the same Go version. The
// two declarations then mean the same thing, so the one kept does not
// matter, and go/types checks a valid package: its type information is
// complete and has one object per name.
//
// Everything else still refuses the plugin with the error of go/types:
//   - two declarations of a name with different texts (which one would run
//     is the choice of yaegi above, not a property of the program);
//   - variables, even with the same text: yaegi runs both initializers
//     ("var V = f()" calls f twice), and one declaration calls it once;
//   - a declaration in a file with other imports or another Go version
//     ("//go:build go1.21"): the same text may then mean another thing
//     (another package for a name, another loop variable semantics);
//   - a declaration that only partly repeats another one ("const A = 1"
//     and "const (A = 1; B = 2)").
//
// init functions and blank names are never removed: they are not
// redeclarations. An import that only the removed copies used is removed
// with them (planDups): the kept copies use it in their file. The default
// loader removes the declarations from the syntax trees; the go list loader blanks them in a copy of the file (an
// overlay of go list, and the ParseFile of go/packages), so that the
// positions of the other declarations stay the same.

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// dupDecls returns the top-level declarations of files (the files of one
// package, in the order of the loader) that repeat an earlier declaration:
// same kind, same names, same text, and a file with the same imports and Go
// version. See the comment at the top of this file.
func dupDecls(fset *token.FileSet, files []*ast.File) map[ast.Decl]bool {
	type first struct{ text, scope string }
	seen := map[string]first{}
	var drop map[ast.Decl]bool
	for _, f := range files {
		scope := fileScope(f)
		for _, d := range f.Decls {
			key := dupDeclKey(d)
			if key == "" {
				continue
			}
			var buf bytes.Buffer
			// The doc comment is printed too (its directives must be the
			// same), and the comments of a spec or a field; the comments in
			// a body are not (go/printer prints the comments attached to a
			// node, not the comment list of the file).
			if err := printer.Fprint(&buf, fset, d); err != nil {
				continue
			}
			text := buf.String()
			prev, ok := seen[key]
			if !ok {
				seen[key] = first{text, scope}
				continue
			}
			if prev.text == text && prev.scope == scope {
				if drop == nil {
					drop = map[ast.Decl]bool{}
				}
				drop[d] = true
			}
		}
	}
	return drop
}

// dupDeclKey returns the kind and the names of a top-level declaration that
// may be removed as a copy of another one, or "" when it is never removed:
// a variable, an import, an init function, a declaration without a name
// other than "_".
func dupDeclKey(d ast.Decl) string {
	switch d := d.(type) {
	case *ast.FuncDecl:
		name := d.Name.Name
		if name == "_" {
			return ""
		}
		if d.Recv == nil {
			if name == "init" {
				return ""
			}
			return "func " + name
		}
		if len(d.Recv.List) != 1 {
			return ""
		}
		base := recvBaseName(d.Recv.List[0].Type)
		if base == "" || base == "_" {
			return ""
		}
		return "method " + base + "." + name
	case *ast.GenDecl:
		if d.Tok != token.CONST && d.Tok != token.TYPE {
			return ""
		}
		var names []string
		named := false
		for _, s := range d.Specs {
			switch s := s.(type) {
			case *ast.ValueSpec:
				for _, n := range s.Names {
					names = append(names, n.Name)
					named = named || n.Name != "_"
				}
			case *ast.TypeSpec:
				names = append(names, s.Name.Name)
				named = named || s.Name.Name != "_"
			}
		}
		if !named {
			return ""
		}
		return d.Tok.String() + " " + strings.Join(names, ",")
	}
	return ""
}

// recvBaseName returns the name of the type of a receiver: T for T, *T,
// T[P] and *T[P]; "" for another expression.
func recvBaseName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}

// fileScope returns what a declaration of f may depend on outside of the
// package scope: the Go version of f and its imports.
func fileScope(f *ast.File) string {
	var imps []string
	for _, s := range f.Imports {
		name := ""
		if s.Name != nil {
			name = s.Name.Name
		}
		imps = append(imps, name+" "+s.Path.Value)
	}
	sort.Strings(imps)
	return f.GoVersion + "\n" + strings.Join(imps, "\n")
}

// dupPlan is what the loaders remove from the files of one package: the
// repeated declarations (dupDecls), and the imports that only they used.
type dupPlan struct {
	decls map[ast.Decl]bool
	specs map[*ast.ImportSpec]bool
}

// planDups returns the dupPlan of files (the files of one package).
// pkgName returns the name of an imported package ("" when unknown).
//
// A file whose declarations are removed may then have unused imports, which
// go/types and gc refuse. Such an import is removed too when the removed
// declarations use its name (a selector x.Name with x not declared in the
// declaration) and the rest of the file has no identifier with that name.
// The removed declarations are copies of declarations kept in files with
// the same imports, which use the import there: the package is still
// imported (its init still runs once), and nothing else changes.
func planDups(fset *token.FileSet, files []*ast.File, pkgName func(path string) string) dupPlan {
	plan := dupPlan{decls: dupDecls(fset, files)}
	if len(plan.decls) == 0 {
		return plan
	}
	for _, f := range files {
		usedDropped := map[string]bool{} // package names used by the removed declarations
		remaining := map[string]bool{}   // identifiers of the rest of the file
		dropped := false
		for _, d := range f.Decls {
			if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
				continue
			}
			if !plan.decls[d] {
				ast.Inspect(d, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						remaining[id.Name] = true
					}
					return true
				})
				continue
			}
			dropped = true
			ast.Inspect(d, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok && x.Obj == nil {
						usedDropped[x.Name] = true
					}
				}
				return true
			})
		}
		if !dropped {
			continue
		}
		for _, s := range f.Imports {
			path, err := strconv.Unquote(s.Path.Value)
			if err != nil {
				continue
			}
			name := ""
			if s.Name != nil {
				name = s.Name.Name
			} else {
				name = pkgName(path)
			}
			if name == "" || name == "_" || name == "." || !usedDropped[name] || remaining[name] {
				continue
			}
			if plan.specs == nil {
				plan.specs = map[*ast.ImportSpec]bool{}
			}
			plan.specs[s] = true
		}
	}
	return plan
}

// dropDupDecls removes from files (the files of one package) what planDups
// returns. The default loader calls it before go/types checks the package.
func dropDupDecls(fset *token.FileSet, files []*ast.File, pkgName func(path string) string) {
	plan := planDups(fset, files, pkgName)
	if len(plan.decls) == 0 {
		return
	}
	for _, f := range files {
		decls := f.Decls[:0]
		for _, d := range f.Decls {
			if plan.decls[d] {
				continue
			}
			if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
				specs := gd.Specs[:0]
				for _, s := range gd.Specs {
					if !plan.specs[s.(*ast.ImportSpec)] {
						specs = append(specs, s)
					}
				}
				if len(specs) == 0 {
					continue
				}
				gd.Specs = specs
			}
			decls = append(decls, d)
		}
		f.Decls = decls
		imps := f.Imports[:0]
		for _, s := range f.Imports {
			if !plan.specs[s] {
				imps = append(imps, s)
			}
		}
		f.Imports = imps
	}
}

// dupSources returns, for the go list loader, the new contents of the files
// of the packages of pkgs (and their dependencies) that have errors and
// repeat declarations: the file with what planDups removes blanked
// (spaces, the line breaks kept, so that the positions of the rest stay the
// same). known holds the contents already changed by an earlier call: a
// file is read from it first. The result holds only new or changed
// contents.
func dupSources(pkgs []*packages.Package, known map[string][]byte) (map[string][]byte, error) {
	out := map[string][]byte{}
	var err error
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if err != nil || len(p.Errors) == 0 {
			return
		}
		names := p.CompiledGoFiles
		if len(names) == 0 {
			names = p.GoFiles
		}
		fset := token.NewFileSet()
		srcs := make([][]byte, len(names))
		files := make([]*ast.File, 0, len(names))
		for i, name := range names {
			src, ok := known[name]
			if !ok {
				if src, err = os.ReadFile(name); err != nil {
					return
				}
			}
			srcs[i] = src
			// With the object resolution: planDups uses it.
			f, perr := parser.ParseFile(fset, name, src, parser.ParseComments)
			if perr != nil {
				return // go/packages reports it
			}
			files = append(files, f)
		}
		plan := planDups(fset, files, func(path string) string {
			if imp := p.Imports[path]; imp != nil {
				return imp.Name
			}
			return ""
		})
		if len(plan.decls) == 0 {
			return
		}
		for i, f := range files {
			var src []byte
			blank := func(from, to token.Pos) {
				if src == nil {
					src = bytes.Clone(srcs[i])
				}
				tf := fset.File(from)
				for o := tf.Offset(from); o < tf.Offset(to); o++ {
					if src[o] != '\n' {
						src[o] = ' '
					}
				}
			}
			for _, d := range f.Decls {
				gd, isGen := d.(*ast.GenDecl)
				switch {
				case plan.decls[d]:
				case isGen && gd.Tok == token.IMPORT:
					all := true
					for _, s := range gd.Specs {
						all = all && plan.specs[s.(*ast.ImportSpec)]
					}
					if !all {
						for _, s := range gd.Specs {
							if plan.specs[s.(*ast.ImportSpec)] {
								blank(s.Pos(), s.End())
							}
						}
						continue
					}
				default:
					continue
				}
				start := d.Pos()
				if doc := declDoc(d); doc != nil {
					start = doc.Pos()
				}
				blank(start, d.End())
			}
			if src != nil {
				out[names[i]] = src
			}
		}
	})
	return out, err
}

// declDoc returns the doc comment of a top-level declaration.
func declDoc(d ast.Decl) *ast.CommentGroup {
	switch d := d.(type) {
	case *ast.FuncDecl:
		return d.Doc
	case *ast.GenDecl:
		return d.Doc
	}
	return nil
}

// writeOverlay writes an overlay file of go list (go help build,
// -overlay): each file of del is deleted, and each file of srcs is
// replaced by its contents (written in a temporary directory, with the
// base name of the file: the compiler reports its errors at the path of
// the copy, and dropSoftErrors matches them to the errors of go/types by
// base name and line). Each file is listed under its path and under its
// real path, since go list may see it through a symbolic link or not. It
// returns the -overlay flag and a function that removes the files it
// wrote.
func writeOverlay(del []string, srcs map[string][]byte) (flag string, cleanup func(), err error) {
	cleanup = func() {}
	tmp, err := os.MkdirTemp("", "yaegi-overlay-")
	if err != nil {
		return "", cleanup, err
	}
	cleanup = func() { os.RemoveAll(tmp) }
	replace := map[string]string{}
	add := func(path, to string) {
		replace[path] = to
		if real, err := filepath.EvalSymlinks(path); err == nil {
			replace[real] = to
		}
	}
	for _, path := range del {
		replace[path] = ""
	}
	i := 0
	for path, src := range srcs {
		dir := filepath.Join(tmp, strconv.Itoa(i))
		i++
		if err := os.Mkdir(dir, 0o700); err != nil {
			cleanup()
			return "", func() {}, err
		}
		to := filepath.Join(dir, filepath.Base(path))
		if err := os.WriteFile(to, src, 0o600); err != nil {
			cleanup()
			return "", func() {}, err
		}
		add(path, to)
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	name := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(name, data, 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return "-overlay=" + name, cleanup, nil
}

// hasPkgErrors reports whether a package of pkgs or of their dependencies
// has errors.
func hasPkgErrors(pkgs []*packages.Package) bool {
	found := false
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		found = found || len(p.Errors) > 0
	})
	return found
}
