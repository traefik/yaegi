package interp

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// Ast(src) parses src string containing Go code and generates the corresponding AST.
// The AST root node is returned.
func Ast(src string) *Node {
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		panic(err)
	}
	//ast.Print(fset, f)

	index := 0
	var root *Node
	var anc *Node
	var st nodestack
	// Populate our own private ast from go ast. A stack of ancestor nodes
	// is used to keep track of curent ancestor for each depth level
	ast.Inspect(f, func(n ast.Node) bool {
		anc = st.top()
		switch n.(type) {
		case nil:
			anc = st.pop()
		default:
			index++
			var i interface{}
			nod := &Node{anc: anc, index: index, anode: &n, val: &i}
			if anc == nil {
				root = nod
			} else {
				anc.Child = append(anc.Child, nod)
			}
			st.push(nod)
		}
		return true
	})
	return root
}

type nodestack []*Node

func (s *nodestack) push(v *Node) {
	*s = append(*s, v)
}

func (s *nodestack) pop() *Node {
	l := len(*s) - 1
	res := (*s)[l]
	*s = (*s)[:l]
	return res
}

func (s *nodestack) top() *Node {
	l := len(*s)
	if l > 0 {
		return (*s)[l-1]
	}
	return nil
}
