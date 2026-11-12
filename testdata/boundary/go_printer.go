// Native boundary of go/printer: program values given as the node of
// Fprint, and program ast.Expr types inside a tree (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
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
	Decls []ast.Decl
	Stmts []ast.Stmt
	Str   string
)

// myNode is a program ast.Node that is no Expr, Stmt, Decl or Spec.
type myNode struct{}

func (myNode) Pos() token.Pos { return token.NoPos }
func (myNode) End() token.Pos { return token.NoPos }

// hashExpr is a program ast.Expr of a hashable type (it embeds *ast.Ident).
type hashExpr struct{ *ast.Ident }

// sliceExpr is a program ast.Expr of an unhashable type.
type sliceExpr struct {
	*ast.BasicLit
	tags []string
}

// genExpr is a generic program ast.Expr.
type genExpr[T any] struct {
	*ast.Ident
	v T
}

// fprint prints node with printer.Fprint and with a Config, and prints the
// panic of each.
func fprint(name string, fset *token.FileSet, node any) {
	for _, cfg := range []*printer.Config{nil, {Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					out("%s cfg=%v: panic: %T: %v", name, cfg != nil, r, r)
				}
			}()
			var sb strings.Builder
			var err error
			if cfg == nil {
				err = printer.Fprint(&sb, fset, node)
			} else {
				err = cfg.Fprint(&sb, fset, node)
			}
			out("%s cfg=%v: %v %q", name, cfg != nil, err, sb.String())
		}()
	}
}

func main() {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "a.go", "package a\n\n// F does it.\nfunc F() {\n\tx := 1\n\t_ = x\n}\n", parser.ParseComments)
	if err != nil {
		panic(err)
	}
	body := f.Decls[0].(*ast.FuncDecl).Body.List

	// row: go/printer Fprint,(*Config).Fprint assert x.(T), T unnamed
	probe("Fprint/slices", func() {
		fprint("[]ast.Decl", fset, f.Decls)
		fprint("Decls", fset, Decls(f.Decls))
		fprint("[]ast.Stmt", fset, body)
		fprint("Stmts", fset, Stmts(body))
		fprint("Str", fset, Str("x"))
		fprint("string", fset, "x")
	})
	// row: go/printer Fprint,(*Config).Fprint assert x.(go/ast.Node)
	probe("Fprint/Node", func() {
		fprint("file", fset, f)
		fprint("CommentedNode", fset, &printer.CommentedNode{Node: f.Decls[0], Comments: f.Comments})
		fprint("CommentedNode myNode", fset, &printer.CommentedNode{Node: myNode{}, Comments: f.Comments})
		fprint("CommentedNode Str", fset, &printer.CommentedNode{Node: Str("x"), Comments: f.Comments})
		fprint("myNode", fset, myNode{})
		fprint("hashExpr", fset, hashExpr{ast.NewIdent("h")})
		fprint("genExpr", fset, genExpr[int]{ast.NewIdent("g"), 1})
		fprint("CommentedNode hashExpr", fset, &printer.CommentedNode{Node: hashExpr{ast.NewIdent("h")}, Comments: f.Comments})
	})
	// row: go/printer Fprint,(*Config).Fprint panic map key of interface type: hash of unhashable type
	probe("nodeSize", func() {
		lit := func(elts ...ast.Expr) *ast.CompositeLit {
			return &ast.CompositeLit{Type: ast.NewIdent("T"), Elts: elts}
		}
		for _, c := range []struct {
			name string
			node func() any
		}{
			{"idents", func() any { return lit(ast.NewIdent("a"), ast.NewIdent("b")) }},
			{"hashExpr", func() any { return lit(ast.NewIdent("a"), hashExpr{ast.NewIdent("b")}) }},
			{"sliceExpr", func() any { return lit(sliceExpr{&ast.BasicLit{Kind: token.INT, Value: "1"}, nil}) }},
			{"key sliceExpr", func() any {
				return lit(&ast.KeyValueExpr{Key: sliceExpr{&ast.BasicLit{Kind: token.INT, Value: "1"}, nil}, Value: ast.NewIdent("v")})
			}},
			{"call args", func() any {
				return &ast.CallExpr{Fun: ast.NewIdent("f"), Args: []ast.Expr{
					ast.NewIdent("a"), sliceExpr{&ast.BasicLit{Kind: token.INT, Value: "2"}, []string{"t"}},
				}}
			}},
		} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						out("%s: build panic: %T: %v", c.name, r, r)
					}
				}()
				n := c.node()
				fprint(c.name, token.NewFileSet(), n)
			}()
		}
	})
}
