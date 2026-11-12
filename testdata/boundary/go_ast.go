// Native boundary of go/ast: program Node types given to the walkers, and
// program values printed by ast.Print and ast.Fprint (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
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

// try runs f and prints its panic, if any.
func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			out("%s: panic: %T: %v", name, r, r)
		}
	}()
	f()
	out("%s: ok", name)
}

// myNode is a program ast.Node (value receivers).
type myNode struct{ p token.Pos }

func (n myNode) Pos() token.Pos { return n.p }
func (n myNode) End() token.Pos { return n.p + 1 }

// genNode is a generic program ast.Node (pointer receivers).
type genNode[T any] struct{ v T }

func (n *genNode[T]) Pos() token.Pos { return 1 }
func (n *genNode[T]) End() token.Pos { return 2 }

// myExpr is an ast.Expr of the program: it embeds *ast.Ident, which gives
// it the unexported method of ast.Expr.
type myExpr struct {
	*ast.Ident
	tag string
}

// sliceNode is an ast.Node of an unhashable type.
type sliceNode []int

func (n sliceNode) Pos() token.Pos { return 1 }
func (n sliceNode) End() token.Pos { return 2 }

// Color has a String method: ast.Print prints it with %v.
type Color int

func (c Color) String() string { return [...]string{"red", "green"}[c] }

type (
	Str   string
	Bytes []byte
	Pos   token.Pos
)

// Tree is a program struct printed by ast.Fprint.
type Tree struct {
	Name    string
	S       Str
	B       []byte
	NB      Bytes
	C       Color
	P       token.Pos
	MP      Pos
	Any     any
	Kids    []*Tree
	Map     map[Str]Color
	Arr     [2]Str
	Parent  *Tree
	Fn      func()
	Ch      chan int
	private int
}

func file() (*token.FileSet, *ast.File) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "a.go", "package a\n\n// c is a comment\nvar x = y\n", parser.ParseComments)
	if err != nil {
		panic(err)
	}
	return fset, f
}

// withProgramExpr gives a file whose var x has a program myExpr value.
func withProgramExpr() (*token.FileSet, *ast.File) {
	fset, f := file()
	spec := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
	spec.Values[0] = myExpr{spec.Values[0].(*ast.Ident), "t"}
	return fset, f
}

func main() {
	// row: go/ast Walk,Inspect,Preorder,PreorderStack,NewCommentMap,CommentMap.Filter panic panic message with the type name
	probe("Walk/unexpected", func() {
		nodes := []ast.Node{myNode{3}, &genNode[int]{1}, sliceNode{1}, myExpr{ast.NewIdent("e"), "x"}}
		for i, n := range nodes {
			try(fmt.Sprintf("Walk %d", i), func() {
				ast.Walk(visitor{}, n)
			})
			try(fmt.Sprintf("Inspect %d", i), func() {
				ast.Inspect(n, func(ast.Node) bool { return true })
			})
			try(fmt.Sprintf("Preorder %d", i), func() {
				for m := range ast.Preorder(n) {
					out("Preorder %d: %T", i, m)
				}
			})
			try(fmt.Sprintf("PreorderStack %d", i), func() {
				ast.PreorderStack(n, nil, func(m ast.Node, stack []ast.Node) bool { return true })
			})
		}
		_, f := withProgramExpr()
		try("Inspect file", func() {
			ast.Inspect(f, func(n ast.Node) bool {
				if n != nil {
					out("Inspect file: %T", n)
				}
				return true
			})
		})
		try("Inspect file false", func() {
			ast.Inspect(f, func(n ast.Node) bool {
				_, isSpec := n.(*ast.ValueSpec)
				return !isSpec
			})
		})
		fset, f := withProgramExpr()
		try("NewCommentMap", func() {
			cm := ast.NewCommentMap(fset, f, f.Comments)
			out("NewCommentMap: %d", len(cm))
		})
		fset, f = file()
		cm := ast.NewCommentMap(fset, f, f.Comments)
		try("Filter", func() {
			out("Filter: %d", len(cm.Filter(myExpr{ast.NewIdent("z"), ""})))
		})
		try("Filter file", func() {
			_, f2 := withProgramExpr()
			out("Filter file: %d", len(cm.Filter(f2)))
		})
	})
	// row: go/ast CommentMap.Update panic map key of interface type: hash of unhashable type
	probe("CommentMap.Update", func() {
		fset, f := file()
		cm := ast.NewCommentMap(fset, f, f.Comments)
		decl := f.Decls[0]
		n := cm.Update(decl, myNode{5})
		out("myNode: %T %d %d", n, len(cm[myNode{5}]), len(cm[decl]))
		n = cm.Update(myNode{5}, &genNode[string]{"g"})
		out("genNode: %T %d", n, len(cm))
		try("unhashable absent", func() { cm.Update(myNode{9}, sliceNode{1}) })
		try("unhashable", func() {
			for k := range cm {
				if _, ok := k.(*genNode[string]); ok {
					cm.Update(k, sliceNode{1, 2})
				}
			}
		})
		out("len after: %d", len(cm))
	})
	// row: go/ast Fprint,Print reflect *
	// row: go/ast Fprint,Print assert x.(T), T unnamed
	// row: go/ast Fprint,Print panic map key of interface type: hash of unhashable type
	probe("Fprint", func() {
		fset := token.NewFileSet()
		tf := fset.AddFile("t.go", -1, 100)
		tf.SetLines([]int{0, 10, 20})
		root := &Tree{
			Name: "root", S: "s", B: []byte("bytes"), NB: Bytes("named"), C: 1,
			P: tf.Pos(12), MP: Pos(tf.Pos(12)), Any: Str("in any"),
			Map: map[Str]Color{"k": 0}, Arr: [2]Str{"a", "b"},
			private: 7,
		}
		kid := &Tree{Name: "kid", Parent: root, Any: []any{Bytes("x"), Color(0), 3}}
		root.Kids = []*Tree{kid, nil}
		var sb strings.Builder
		err := ast.Fprint(&sb, fset, root, nil)
		out("Fprint nil filter: %v\n%s", err, sb.String())
		sb.Reset()
		err = ast.Fprint(&sb, nil, root, ast.NotNilFilter)
		out("Fprint NotNilFilter: %v\n%s", err, sb.String())
		sb.Reset()
		err = ast.Fprint(&sb, fset, []any{Str("a"), Bytes("b"), map[string]any{"m": genNode[Color]{1}}}, func(name string, v reflect.Value) bool {
			return name != "S"
		})
		out("Fprint any: %v\n%s", err, sb.String())
		sb.Reset()
		err = ast.Fprint(&sb, nil, nil, nil)
		out("Fprint nil: %v %q", err, sb.String())
		out("Print:")
		err = ast.Print(fset, kid)
		out("Print err: %v", err)
	})
	// row: go/ast NotNilFilter reflect *
	probe("NotNilFilter", func() {
		var nilTree *Tree
		var nilMap map[Str]Color
		var nilSlice Bytes
		var nilFn func()
		var nilCh chan Color
		var iface any
		var nodeIface ast.Node = myNode{1}
		vals := []reflect.Value{
			reflect.ValueOf(nilTree), reflect.ValueOf(&Tree{}), reflect.ValueOf(nilMap),
			reflect.ValueOf(map[Str]Color{}), reflect.ValueOf(nilSlice), reflect.ValueOf(Bytes{}),
			reflect.ValueOf(nilFn), reflect.ValueOf(nilCh), reflect.ValueOf(&iface).Elem(),
			reflect.ValueOf(func() {}), reflect.ValueOf(Tree{}), reflect.ValueOf(Str("")),
			reflect.ValueOf(Color(0)), reflect.ValueOf(genNode[int]{}),
		}
		for i, v := range vals {
			out("%d %v: %v", i, v.Kind(), ast.NotNilFilter("f", v))
		}
		try("interface holding a program node", func() {
			v := reflect.ValueOf(&nodeIface).Elem()
			out("%v: %v", v.Kind(), ast.NotNilFilter("f", v))
		})
	})
}

// visitor is a program ast.Visitor.
type visitor struct{}

func (v visitor) Visit(n ast.Node) ast.Visitor {
	if n != nil {
		out("Visit %T", n)
	}
	return v
}
