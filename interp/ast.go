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
	ast.Inspect(f, func(node ast.Node) bool {
		anc = st.top()
		switch node.(type) {
		case nil:
			anc = st.pop()
		case *ast.RangeStmt:
			index++
			var i, j interface{}
			// Insert an extra node for handling CFG
			nod0 := &Node{anc: anc, index: index, val: &i}
			nod0.Start = nod0
			anc.Child = append(anc.Child, nod0)
			index++
			nod1 := &Node{anc: nod0, index: index, anode: &node, val: &j}
			nod1.Start = nod1
			nod0.Child = append(nod0.Child, nod1)
			st.push(nod1)
		default:
			index++
			var i interface{}
			nod := &Node{anc: anc, index: index, anode: &node, val: &i}
			nod.Start = nod
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
