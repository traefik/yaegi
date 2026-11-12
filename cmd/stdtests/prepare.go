package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const modulePath = "github.com/traefik/yaegi/v2"

// shims are the internal packages that the test files may import: a
// program outside the standard library cannot import them, so they are
// replaced by the packages of cmd/stdtests/shim (same names, same
// behavior for a program built without -race, -asan or -msan).
var shims = map[string]string{
	"internal/testenv": modulePath + "/cmd/stdtests/shim/testenv",
	"internal/race":    modulePath + "/cmd/stdtests/shim/race",
	"internal/asan":    modulePath + "/cmd/stdtests/shim/asan",
	"internal/msan":    modulePath + "/cmd/stdtests/shim/msan",
}

// A Program is the test program of one package: its external test files
// (package xxx_test) as package main, and a main function that runs them
// with package testing.
type Program struct {
	Pkg      string // import path of the package under test
	Dir      string // directory of the program
	PkgDir   string // directory of the package in GOROOT (where it runs)
	Tests    []string
	Examples []string
	Fuzz     []string
	// Excluded are the tests left out because their code does not build
	// outside of the package (names of export_test.go, internal packages),
	// with the reason.
	Excluded map[string]string
}

// Names returns the tests, examples and fuzz targets of the program.
func (p *Program) Names() []string {
	var out []string
	out = append(out, p.Tests...)
	out = append(out, p.Fuzz...)
	out = append(out, p.Examples...)
	return out
}

// prepare writes the test program of pkg into dir. It reads the external
// test files of the package in goroot, renames their package to main,
// replaces the imports of shims, and removes the declarations that do not
// type check: they use names that only export_test.go (a file of the
// package itself) defines, or an internal package. The tests that lose
// their code this way are excluded, with the first error as the reason.
func prepare(root, goroot, pkg, dir string) (*Program, error) {
	pkgDir := filepath.Join(goroot, "src", filepath.FromSlash(pkg))
	ctx := build.Default
	ctx.GOROOT = goroot
	ctx.CgoEnabled = false // as yaegi v2: no cgo
	bp, err := ctx.ImportDir(pkgDir, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", pkg, err)
	}
	if len(bp.XTestGoFiles) == 0 {
		return nil, fmt.Errorf("%s: no external test files", pkg)
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	// The test files keep their package (xxx_test, as %T prints it) in
	// the directory xtest; the main package in dir imports it.
	xdir := filepath.Join(dir, "xtest")
	if err := os.MkdirAll(xdir, 0o755); err != nil {
		return nil, err
	}

	p := &Program{Pkg: pkg, Dir: dir, PkgDir: pkgDir, Excluded: map[string]string{}}
	// removed maps a removed top-level name (or import name) to the reason.
	removed := map[string]string{}
	fset := token.NewFileSet()
	var files []*ast.File
	var names []string
	for _, name := range bp.XTestGoFiles {
		src, err := os.ReadFile(filepath.Join(pkgDir, name))
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(f.Imports); i++ {
			spec := f.Imports[i]
			path, _ := strconv.Unquote(spec.Path.Value)
			if s, ok := shims[path]; ok {
				spec.Path.Value = strconv.Quote(s)
				if spec.Name == nil {
					spec.Name = ast.NewIdent(filepath.Base(path))
				}
				continue
			}
			if isInternal(path) {
				// Cannot be imported here: remove the import, the
				// declarations that use it go with the next type check.
				local := filepath.Base(path)
				if spec.Name != nil {
					local = spec.Name.Name
				}
				removed[local] = "imports " + path + " (internal, no shim in cmd/stdtests)"
				deleteImport(f, spec)
				i--
			}
		}
		files = append(files, f)
		// go build leaves out the files named *_test.go.
		names = append(names, strings.TrimSuffix(name, "_test.go")+"_xtest.go")
	}

	// Example outputs, as go test reads them (go/doc).
	examples := map[string]*doc.Example{}
	for _, f := range files {
		for _, ex := range doc.Examples(f) {
			examples["Example"+ex.Name] = ex
		}
	}

	// Type check and remove what does not build, until it builds.
	for round := 0; ; round++ {
		if round > 50 {
			return nil, fmt.Errorf("%s: the test files still do not type check after %d rounds", pkg, round)
		}
		if err := writeFiles(xdir, fset, names, files); err != nil {
			return nil, err
		}
		errs, err := typeErrors(xdir)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", pkg, err)
		}
		if len(errs) == 0 {
			break
		}
		// Read the files again: the positions of the errors are the ones
		// of the written files.
		fset = token.NewFileSet()
		for i, name := range names {
			f, err := parser.ParseFile(fset, filepath.Join(xdir, name), nil, parser.ParseComments)
			if err != nil {
				return nil, err
			}
			files[i] = f
		}
		progress := false
		for _, e := range errs {
			if pruneError(fset, files, names, e, removed) {
				progress = true
			}
		}
		if !progress {
			return nil, fmt.Errorf("%s: cannot remove the declaration of a type error: %v", pkg, errs[0])
		}
	}

	// The tests, fuzz targets and examples that are left.
	var testMain bool
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			n := fd.Name.Name
			switch {
			case n == "TestMain" && isTestMain(fd):
				testMain = true
			case isTestName(n, "Test") && hasParam(fd, "T"):
				p.Tests = append(p.Tests, n)
			case isTestName(n, "Fuzz") && hasParam(fd, "F"):
				p.Fuzz = append(p.Fuzz, n)
			case isTestName(n, "Example") && len(fd.Type.Params.List) == 0:
				if ex := examples[n]; ex != nil && (ex.Output != "" || ex.EmptyOutput) {
					p.Examples = append(p.Examples, n)
				}
			}
		}
	}
	for n, why := range removed {
		if isTestName(n, "Test") || isTestName(n, "Fuzz") || isTestName(n, "Example") {
			if ex, ok := examples[n]; isTestName(n, "Example") && (!ok || (ex.Output == "" && !ex.EmptyOutput)) {
				continue // an example without output is never run
			}
			p.Excluded[n] = why
		}
	}

	var b bytes.Buffer
	b.WriteString("// Code generated by cmd/stdtests. DO NOT EDIT.\n\npackage main\n\nimport (\n")
	b.WriteString("\t\"testing\"\n\n\t\"" + modulePath + "/cmd/stdtests/testmain\"\n")
	rel, err := filepath.Rel(root, xdir)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "\txtest %q\n)\n\n", modulePath+"/"+filepath.ToSlash(rel))
	b.WriteString("func main() {\n\ttestmain.Main(" + strconv.Quote(pkg) + ",\n")
	b.WriteString("\t\t[]testing.InternalTest{\n")
	for _, n := range p.Tests {
		fmt.Fprintf(&b, "\t\t\t{Name: %q, F: xtest.%s},\n", n, n)
	}
	b.WriteString("\t\t},\n\t\t[]testing.InternalFuzzTarget{\n")
	for _, n := range p.Fuzz {
		fmt.Fprintf(&b, "\t\t\t{Name: %q, Fn: xtest.%s},\n", n, n)
	}
	b.WriteString("\t\t},\n\t\t[]testing.InternalExample{\n")
	for _, n := range p.Examples {
		ex := examples[n]
		fmt.Fprintf(&b, "\t\t\t{Name: %q, F: xtest.%s, Output: %q, Unordered: %v},\n", n, n, ex.Output, ex.Unordered)
	}
	b.WriteString("\t\t},\n")
	if testMain {
		b.WriteString("\t\txtest.TestMain,\n")
	} else {
		b.WriteString("\t\tnil,\n")
	}
	b.WriteString("\t)\n}\n")
	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("%s: generated main: %v", pkg, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zz_stdtests_main.go"), src, 0o644); err != nil {
		return nil, err
	}
	errs, err := typeErrors(dir)
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: generated program: %v", pkg, errs[0])
	}
	return p, nil
}

func isInternal(path string) bool {
	return strings.HasPrefix(path, "internal/") || strings.Contains(path, "/internal/") || strings.HasSuffix(path, "/internal")
}

// isTestName is the rule of go test: prefix, then nothing or a character
// that is not a lower case letter.
func isTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	c := name[len(prefix)]
	return !('a' <= c && c <= 'z')
}

// hasParam reports whether fd takes one parameter of type *testing.<typ>.
func hasParam(fd *ast.FuncDecl, typ string) bool {
	ps := fd.Type.Params.List
	if len(ps) != 1 || len(ps[0].Names) > 1 || fd.Type.Results != nil {
		return false
	}
	star, ok := ps[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == typ
}

func isTestMain(fd *ast.FuncDecl) bool { return hasParam(fd, "M") }

func deleteImport(f *ast.File, spec *ast.ImportSpec) {
	for i, s := range f.Imports {
		if s == spec {
			f.Imports = append(f.Imports[:i], f.Imports[i+1:]...)
			break
		}
	}
	for di, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, s := range gd.Specs {
			if s == spec {
				gd.Specs = append(gd.Specs[:i], gd.Specs[i+1:]...)
				if len(gd.Specs) == 0 {
					f.Decls = append(f.Decls[:di], f.Decls[di+1:]...)
				}
				return
			}
		}
	}
}

func writeFiles(dir string, fset *token.FileSet, names []string, files []*ast.File) error {
	for i, f := range files {
		var b bytes.Buffer
		if err := format.Node(&b, fset, f); err != nil {
			return fmt.Errorf("%s: %v", names[i], err)
		}
		if err := os.WriteFile(filepath.Join(dir, names[i]), b.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// A typeError is an error of the type check of the program.
type typeError struct {
	File      string
	Line, Col int
	Msg       string
}

func (e typeError) String() string { return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg) }

var (
	posRE     = regexp.MustCompile(`^(.*\.go):(\d+):(\d+)$`)
	listErrRE = regexp.MustCompile(`^(?:\./)?(\S+\.go):(\d+):(\d+): (.*)$`)
)

// typeErrors type checks the program in dir with go/packages.
func typeErrors(dir string) ([]typeError, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Dir:  dir,
		Env:  append(os.Environ(), "CGO_ENABLED=0"),
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("go/packages: %d packages", len(pkgs))
	}
	var out []typeError
	seen := map[typeError]bool{}
	add := func(e typeError) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	for _, e := range pkgs[0].Errors {
		if m := posRE.FindStringSubmatch(e.Pos); m != nil {
			line, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			add(typeError{File: m[1], Line: line, Col: col, Msg: e.Msg})
			continue
		}
		// An error of go list (the compiler): "# pkg" and one line per
		// error, "./file.go:line:col: message".
		n := 0
		for _, l := range strings.Split(e.Msg, "\n") {
			m := listErrRE.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			line, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			add(typeError{File: m[1], Line: line, Col: col, Msg: m[4]})
			n++
		}
		if n == 0 {
			return nil, fmt.Errorf("error without a position: %v", e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

var undefRE = regexp.MustCompile(`^undefined: ([A-Za-z_][A-Za-z_0-9]*)(?:\.([A-Za-z_][A-Za-z_0-9]*))?`)

// pruneError removes the top-level declaration (or import) where the error
// e is, and records its names in removed with a reason. It reports whether
// it removed something.
func pruneError(fset *token.FileSet, files []*ast.File, names []string, e typeError, removed map[string]string) bool {
	var f *ast.File
	for i, n := range names {
		if filepath.Base(e.File) == n {
			f = files[i]
		}
	}
	if f == nil {
		return false
	}
	tf := fset.File(f.Pos())
	if e.Line < 1 || e.Line > tf.LineCount() {
		return false
	}
	pos := tf.LineStart(e.Line) + token.Pos(e.Col-1)

	reason := e.Msg
	if m := undefRE.FindStringSubmatch(e.Msg); m != nil {
		if why, ok := removed[m[1]]; ok && m[2] == "" {
			reason = why
		} else if why, ok := removed[m[1]]; ok && m[2] != "" && strings.HasPrefix(why, "imports ") {
			reason = why
		} else if m[2] != "" {
			reason = "uses " + m[1] + "." + m[2] + " (not exported: export_test.go)"
		} else {
			reason = "uses " + m[1] + " (not exported: export_test.go, dot import)"
		}
	}

	for di, d := range f.Decls {
		if pos < d.Pos() || pos >= d.End() {
			continue
		}
		switch d := d.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil {
				name = recvName(d) + "." + name
			}
			if _, ok := removed[name]; !ok {
				removed[name] = reason
			}
			f.Decls = append(f.Decls[:di], f.Decls[di+1:]...)
			return true
		case *ast.GenDecl:
			for si, s := range d.Specs {
				if pos < s.Pos() || pos >= s.End() {
					continue
				}
				switch s := s.(type) {
				case *ast.ImportSpec:
					// "imported and not used": drop the import.
					deleteImport(f, s)
					return true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if _, ok := removed[n.Name]; !ok {
							removed[n.Name] = reason
						}
					}
				case *ast.TypeSpec:
					if _, ok := removed[s.Name.Name]; !ok {
						removed[s.Name.Name] = reason
					}
				}
				d.Specs = append(d.Specs[:si], d.Specs[si+1:]...)
				if len(d.Specs) == 0 {
					f.Decls = append(f.Decls[:di], f.Decls[di+1:]...)
				}
				return true
			}
		}
		return false
	}
	return false
}

func recvName(fd *ast.FuncDecl) string {
	t := fd.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch t := t.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return "?"
}
