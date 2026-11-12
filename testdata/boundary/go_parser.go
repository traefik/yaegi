// Native boundary of go/parser: program values given as the src of
// ParseFile and ParseExprFrom (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

type (
	Str   string
	Bytes []byte
)

// srcReader is a program io.Reader (pointer receiver).
type srcReader struct{ r *strings.Reader }

func (s *srcReader) Read(p []byte) (int, error) { return s.r.Read(p) }

// strReader is a named string with a Read method (value receiver): it is
// not a string for go/parser, but an io.Reader.
type strReader string

func (s strReader) Read(p []byte) (int, error) {
	n := copy(p, s)
	if n == 0 {
		return 0, io.EOF
	}
	return n, io.EOF
}

// genReader is a generic io.Reader.
type genReader[T any] struct {
	r   *strings.Reader
	tag T
}

func (g genReader[T]) Read(p []byte) (int, error) { return g.r.Read(p) }

const fileSrc = "package p\n\nvar V = 1\n\nfunc F() int { return V }\n"
const exprSrc = "a + b*2"

func srcs(s string) []any {
	var ir io.Reader = &srcReader{strings.NewReader(s)}
	return []any{
		s, []byte(s), Str(s), Bytes(s), &srcReader{strings.NewReader(s)},
		strReader(s), genReader[int]{strings.NewReader(s), 1}, ir, strings.NewReader(s), 42,
	}
}

func main() {
	// row: go/parser ParseFile assert x.(T), T unnamed
	// row: go/parser ParseFile assert x.(io.Reader)
	probe("ParseFile/src", func() {
		for i, src := range srcs(fileSrc) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", src, 0)
			if err != nil {
				out("%d %T: %v", i, src, err)
				continue
			}
			out("%d %T: %s %d decls", i, src, f.Name.Name, len(f.Decls))
		}
		fset := token.NewFileSet()
		_, err := parser.ParseFile(fset, "/nonexistent/boundary/p.go", nil, 0)
		out("nil src: %v", err != nil)
	})
	// row: go/parser ParseExprFrom assert x.(T), T unnamed
	// row: go/parser ParseExprFrom assert x.(io.Reader)
	// row: go/parser ParseExpr assert x.(T), T unnamed
	// row: go/parser ParseExpr assert x.(io.Reader)
	probe("ParseExpr/src", func() {
		for i, src := range srcs(exprSrc) {
			fset := token.NewFileSet()
			x, err := parser.ParseExprFrom(fset, "e.go", src, 0)
			if err != nil {
				out("%d %T: %v", i, src, err)
				continue
			}
			out("%d %T: %T", i, src, x)
		}
		x, err := parser.ParseExpr(exprSrc)
		out("ParseExpr: %T %v", x, err)
		x, err = parser.ParseExpr(string(Str("f(")))
		out("ParseExpr error: %T %v", x, err)
	})
	// The == of parseCallExpr compares two expressions made by the parser;
	// the probe takes the path with parenthesized calls.
	// row: go/parser ParseFile,ParseExpr,ParseExprFrom,ParseDir panic == of two interfaces: comparing uncomparable type
	probe("parseCallExpr", func() {
		src := "package p\nfunc f() { go (g)(); defer h(); go ((k()))\n }\n"
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "c.go", src, parser.AllErrors)
		out("ParseFile: %v", err)
		if f != nil {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.GoStmt:
					out("go %T", n.Call.Fun)
				case *ast.DeferStmt:
					out("defer %T", n.Call.Fun)
				}
				return true
			})
		}
		x, err := parser.ParseExpr("func() { go (f)() }")
		out("ParseExpr: %T %v", x, err)
		x, err = parser.ParseExprFrom(token.NewFileSet(), "", "func() { defer f() }", 0)
		out("ParseExprFrom: %T %v", x, err)
	})
	// The assertion of resolveFile only runs when debugResolve, a constant
	// false, is true: the probe resolves package objects.
	// row: go/parser ParseFile,ParseDir assert x.(interface{Pos() go/token.Pos})
	// row: go/parser ParseFile,ParseDir panic x.(interface{Pos() go/token.Pos}) without ok: *runtime.TypeAssertionError
	probe("resolveFile", func() {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "r.go", fileSrc, 0)
		if err != nil {
			out("%v", err)
			return
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Obj != nil {
				out("%s -> %v %T", id.Name, id.Obj.Kind, id.Obj.Decl)
			}
			return true
		})
		out("unresolved: %d", len(f.Unresolved))
	})
	// row: go/parser ParseDir assert x.(T), T unnamed
	// row: go/parser ParseDir assert x.(io.Reader)
	probe("ParseDir", func() {
		dir, err := os.MkdirTemp("", "boundary-parser")
		if err != nil {
			out("MkdirTemp: %v", err)
			return
		}
		defer os.RemoveAll(dir)
		os.WriteFile(filepath.Join(dir, "a.go"), []byte(fileSrc), 0o600)
		os.WriteFile(filepath.Join(dir, "b.go"), []byte("package p\nvar W = V + go1\n"), 0o600)
		os.WriteFile(filepath.Join(dir, "skip.go"), []byte("package q\n"), 0o600)
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
			return fi.Name() != "skip.go"
		}, parser.ParseComments)
		out("ParseDir: %v", err)
		for name, pkg := range pkgs {
			var files []string
			for fn, f := range pkg.Files {
				files = append(files, fmt.Sprintf("%s:%d", filepath.Base(fn), len(f.Decls)))
			}
			sort.Strings(files)
			out("package %s: %v", name, files)
		}
	})
}
