package interp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/traefik/yaegi/stdlib"
)

func TestCompileAST(t *testing.T) {
	i := New(Options{})
	file, err := parser.ParseFile(i.FileSet(), "_.go", `
		package main

		import "fmt"

		type Foo struct{}

		var foo Foo
		const bar = "asdf"

		func main() {
			fmt.Println(1)
		}
	`, 0)
	if err != nil {
		panic(err)
	}
	if len(file.Imports) != 1 || len(file.Decls) != 5 {
		panic("wrong number of imports or decls")
	}

	dType := file.Decls[1].(*ast.GenDecl)
	dVar := file.Decls[2].(*ast.GenDecl)
	dConst := file.Decls[3].(*ast.GenDecl)
	dFunc := file.Decls[4].(*ast.FuncDecl)

	if dType.Tok != token.TYPE {
		panic("decl[1] is not a type")
	}
	if dVar.Tok != token.VAR {
		panic("decl[2] is not a var")
	}
	if dConst.Tok != token.CONST {
		panic("decl[3] is not a const")
	}

	cases := []struct {
		desc string
		node ast.Node
		skip string
	}{
		{desc: "file", node: file, skip: "temporary ignore"},
		{desc: "import", node: file.Imports[0]},
		{desc: "type", node: dType},
		{desc: "var", node: dVar, skip: "not supported"},
		{desc: "const", node: dConst},
		{desc: "func", node: dFunc},
		{desc: "block", node: dFunc.Body},
		{desc: "expr", node: dFunc.Body.List[0]},
	}

	_ = i.Use(stdlib.Symbols)

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			if c.skip != "" {
				t.Skip(c.skip)
			}

			i := i
			if _, ok := c.node.(*ast.File); ok {
				i = New(Options{})
				_ = i.Use(stdlib.Symbols)
			}
			_, err := i.CompileAST(c.node)
			if err != nil {
				t.Fatalf("Failed to compile %s: %v", c.desc, err)
			}
		})
	}
}
