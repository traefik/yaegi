package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"reflect"
)

// Structure for AST and CFG
type node struct {
	children []*node   // child subtrees
	anc      *node     // ancestor
	snext    *node     // successor (CFG)
	next     [2]*node  // conditional successor (CFG)
	index    int       // node index (dot display)
	anode    *ast.Node // original ast node (temporary, will be removed)
	//sym   *sym     // symbol
}

// Walk AST in depth first order, call 'in' function at node entry and
// 'out' function at node exit
func walk(n *node, in func(n *node), out func(n *node)) {
	if in != nil {
		in(n)
	}
	for _, child := range n.children {
		walk(child, in, out)
	}
	if out != nil {
		out(n)
	}
}

// For debug: display an AST in graphviz dot(1) format using dotty(1) co-process
func astdot(root *node) {
	cmd := exec.Command("dotty", "-")
	dotin, err := cmd.StdinPipe()
	if err != nil {
		panic("dotty stdin error")
	}
	cmd.Start()
	fmt.Fprintf(dotin, "digraph ast {\n")
	walk(root, func(n *node) {
		var label string
		switch x := (*n.anode).(type) {
		case *ast.BasicLit:
			label = x.Value
		case *ast.Ident:
			label = x.Name
		case *ast.BinaryExpr:
			label = x.Op.String()
		case *ast.IncDecStmt:
			label = x.Tok.String()
		case *ast.AssignStmt:
			label = x.Tok.String()
		default:
			label = reflect.TypeOf(*n.anode).String()
		}
		fmt.Fprintf(dotin, "%d [label=\"%s\"]\n", n.index, label)
		if n.anc != nil {
			fmt.Fprintf(dotin, "%d -> %d\n", n.anc.index, n.index)
		}
		fmt.Printf("%v : %v\n", reflect.TypeOf(*n.anode), reflect.ValueOf(*n.anode))
	}, nil)
	fmt.Fprintf(dotin, "}")
}

type nodestack []*node

func (s *nodestack) push(v *node) {
	*s = append(*s, v)
}

func (s *nodestack) pop() *node {
	l := len(*s) - 1
	res := (*s)[l]
	*s = (*s)[:l]
	return res
}

func (s *nodestack) top() *node {
	l := len(*s)
	if l > 0 {
		return (*s)[l-1]
	}
	return nil
}

// Parse src string containing go code and generate AST. Returns the root node
func src_to_ast(src string) *node {
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		panic(err)
	}
	//ast.Print(fset, f)
	index := 0
	var root *node
	var anc *node
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
			nod := &node{anc: anc, index: index, anode: &n}
			if anc == nil {
				root = nod
			} else {
				anc.children = append(anc.children, nod)
			}
			st.push(nod)
		}
		return true
	})
	return root
}

// Main function
func main() {
	const src = `
package main

func main() {
	for a := 0; a < 20000; a++ {
		if (a & 0x8ff) == 0x800 {
			println(a)
		}
	}
}
`

	root := src_to_ast(src)
	astdot(root)
}
