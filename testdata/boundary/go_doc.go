// Native boundary of go/doc, go/constant and go/build/constraint: program
// ast.Decl, constant values and constraint.Expr types given to these
// packages (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/constant"
	"go/doc"
	"go/parser"
	"go/token"
	"math/big"
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

// try runs f and prints its panic, if any.
func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			out("%s: panic: %T: %v", name, r, r)
		}
	}()
	f()
}

// hashDecl is a program ast.Decl (it embeds *ast.GenDecl).
type hashDecl struct{ *ast.GenDecl }

// sliceDecl is a program ast.Decl of an unhashable type.
type sliceDecl struct {
	*ast.GenDecl
	tags []string
}

const testSrc = `package p_test

import "fmt"

var V = 1

type T struct{}

func (T) M() {}

func helper() int { return V }

func ExampleF() {
	fmt.Println(V, helper())
	T{}.M()
	// Output: 1 1
}

func ExampleT_M() {
	var t T
	t.M()
}
`

const pkgSrc = "package p\n\n// F is a function.\nfunc F() {}\n"

func parse(fset *token.FileSet, name, src string) *ast.File {
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	return f
}

// addDecls adds program declarations after the declarations of f.
func addDecls(f *ast.File, decls ...ast.Decl) {
	f.Decls = append(f.Decls, decls...)
}

func extra() []ast.Decl {
	gd := &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("W")}}}}
	return []ast.Decl{hashDecl{gd}, sliceDecl{gd, []string{"t"}}}
}

func printExamples(exs []*doc.Example) {
	for _, ex := range exs {
		play := ex.Play != nil
		decls := 0
		if play {
			decls = len(ex.Play.Decls)
		}
		out("example %q suffix=%q output=%q play=%v decls=%d", ex.Name, ex.Suffix, ex.Output, play, decls)
	}
}

type (
	myBool   bool
	myInt64  int64
	myString string
	myInt    int
)

// sliceTag is a program constraint.Expr of an uncomparable type (it
// embeds *constraint.TagExpr).
type sliceTag struct {
	*constraint.TagExpr
	alts []string
}

// hashTag is a program constraint.Expr of a comparable type.
type hashTag struct{ *constraint.TagExpr }

func main() {
	// row: go/doc Examples,NewFromFiles panic map key of interface type: hash of unhashable type
	probe("Examples", func() {
		fset := token.NewFileSet()
		f := parse(fset, "p_test.go", testSrc)
		try("plain", func() { printExamples(doc.Examples(f)) })
		f = parse(fset, "p_test.go", testSrc)
		addDecls(f, extra()...)
		try("program decls", func() { printExamples(doc.Examples(f)) })
		try("NewFromFiles", func() {
			fset := token.NewFileSet()
			tf := parse(fset, "p_test.go", testSrc)
			addDecls(tf, extra()...)
			pf := parse(fset, "p.go", pkgSrc)
			addDecls(pf, extra()...)
			p, err := doc.NewFromFiles(fset, []*ast.File{pf, tf}, "example.com/p")
			if err != nil {
				out("NewFromFiles: %v", err)
				return
			}
			for _, fn := range p.Funcs {
				out("func %s: %q", fn.Name, fn.Doc)
				printExamples(fn.Examples)
			}
			for _, v := range p.Vars {
				out("vars %v", v.Names)
			}
		})
	})
	// row: go/constant Make assert x.(T), T unnamed
	probe("constant.Make", func() {
		vals := []any{
			true, myBool(true), int64(7), myInt64(7), "s", myString("s"),
			big.NewInt(9), big.NewRat(1, 3), big.NewFloat(1.5), myInt(3), 3, 2.5,
		}
		for i, v := range vals {
			c := constant.Make(v)
			out("%d %T: %v %s", i, v, c.Kind(), c.ExactString())
		}
	})
	// row: go/build/constraint PlusBuildLines panic == of two interfaces: comparing uncomparable type
	probe("PlusBuildLines", func() {
		parsed, _ := constraint.Parse("//go:build linux && (amd64 || !cgo)")
		exprs := []struct {
			name string
			x    func() constraint.Expr
		}{
			{"parsed", func() constraint.Expr { return parsed }},
			{"hashTag", func() constraint.Expr { return hashTag{&constraint.TagExpr{Tag: "h"}} }},
			{"and hashTag", func() constraint.Expr {
				return &constraint.AndExpr{X: hashTag{&constraint.TagExpr{Tag: "a"}}, Y: &constraint.TagExpr{Tag: "b"}}
			}},
			{"not hashTag", func() constraint.Expr { return &constraint.NotExpr{X: hashTag{&constraint.TagExpr{Tag: "n"}}} }},
			{"sliceTag", func() constraint.Expr { return sliceTag{&constraint.TagExpr{Tag: "s"}, nil} }},
			{"and sliceTag", func() constraint.Expr {
				return &constraint.AndExpr{X: sliceTag{&constraint.TagExpr{Tag: "a"}, nil}, Y: &constraint.TagExpr{Tag: "b"}}
			}},
			{"or sliceTag", func() constraint.Expr {
				return &constraint.OrExpr{X: &constraint.TagExpr{Tag: "a"}, Y: sliceTag{&constraint.TagExpr{Tag: "b"}, []string{"c"}}}
			}},
		}
		for _, e := range exprs {
			try(e.name, func() {
				lines, err := constraint.PlusBuildLines(e.x())
				out("%s: %q %v", e.name, lines, err)
			})
		}
	})
}
