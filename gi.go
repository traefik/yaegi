package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"reflect"
)

type nodetype int

const (
	Undef nodetype = iota
	BasicLit
	Ident
	BinaryExpr
	IncDecStmt
	AssignStmt
)

// Structure for AST and CFG
type node struct {
	child []*node   // child subtrees
	anc   *node     // ancestor
	start *node     // entry point in subtree (CFG)
	snext *node     // successor (CFG)
	next  [2]*node  // conditional successor (CFG)
	index int       // node index (dot display)
	typ   nodetype  // node type
	anode *ast.Node // original ast node (temporary, will be removed)
	//sym   *sym     // symbol
}

// Returns true if node is a leaf in the AST
func (n *node) is_leaf() bool {
	return len((*n).child) == 0
}

// Walk AST in depth first order, call 'in' function at node entry and
// 'out' function at node exit
func walk(n *node, in func(n *node), out func(n *node)) {
	if in != nil {
		in(n)
	}
	for _, child := range n.child {
		walk(child, in, out)
	}
	if out != nil {
		out(n)
	}
}

// Wire AST nodes of sequential blocks
func wire_child(n *node) {
	println("wire_child", reflect.TypeOf(*n.anode).String())
	for _, child := range n.child {
		if !child.is_leaf() {
			n.start = child.start
		}
	}
	if n.start == nil {
		n.start = n
	}
	for i := 1; i < len(n.child); i++ {
		n.child[i-1].snext = n.child[i].start
	}
	for i := len(n.child) - 1; i >= 0; i-- {
		if !n.child[i].is_leaf() {
			n.child[i].snext = n
			break
		}
	}
}

// Generate a CFG from AST (wiring successors in AST)
func ast_to_cfg(root *node) {
	walk(root, nil, func(n *node) {
		switch (*n.anode).(type) {
		case *ast.BlockStmt:
			wire_child(n)
		case *ast.IncDecStmt:
			wire_child(n)
		case *ast.AssignStmt:
			wire_child(n)
		case *ast.ExprStmt:
			wire_child(n)
		case *ast.ParenExpr:
			wire_child(n)
		case *ast.BinaryExpr:
			wire_child(n)
		case *ast.CallExpr:
			wire_child(n)
		case *ast.IfStmt:
			n.start = n.child[0].start
			n.child[1].snext = n
			println("if nchild:", len(n.child))
			if len(n.child) == 3 {
				n.child[2].snext = n
			}
			// CFG: add a conditional branch node to resolve the snext at execution
			// The node is not chained in the AST, only reachable through snext
			nod := &node{}
			n.child[0].snext = nod
			nod.next[1] = n.child[1].start
			if len(n.child) == 3 {
				nod.next[0] = n.child[2].start
			} else {
				nod.next[0] = n
			}
		case *ast.ForStmt:
			// FIXME: works only if for node has 4 children
			n.start = n.child[0].start
			n.child[0].snext = n.child[1].start
			nod := &node{}
			n.child[1].snext = nod
			nod.next[0] = n
			nod.next[1] = n.child[3].start
			n.child[3].snext = n.child[2].start
			n.child[2].snext = n.child[1].start
		case *ast.BasicLit:
		case *ast.Ident:
		default:
			println("unknown type:", reflect.TypeOf(*n.anode).String())
		}
	})
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
		fmt.Fprintf(dotin, "%d [label=\"%d: %s\"]\n", n.index, n.index, label)
		if n.anc != nil {
			fmt.Fprintf(dotin, "%d -> %d\n", n.anc.index, n.index)
		}
		//fmt.Printf("%v : %v\n", reflect.TypeOf(*n.anode), reflect.ValueOf(*n.anode))
	}, nil)
	fmt.Fprintf(dotin, "}")
}

// For debug: display a CFG in graphviz dot(1) format using dotty(1) co-process
func cfgdot(root *node) {
	cmd := exec.Command("dotty", "-")
	dotin, err := cmd.StdinPipe()
	if err != nil {
		panic("dotty stdin error")
	}
	cmd.Start()
	fmt.Fprintf(dotin, "digraph cfg {\n")
	walk(root, nil, func(n *node) {
		switch (*n.anode).(type) {
		case *ast.BasicLit:
			return
		case *ast.Ident:
			return
		}
		fmt.Fprintf(dotin, "%d [label=\"%d\"]\n", n.index, n.index)
		if n.snext == nil {
			return
		}
		if n.snext.next[1] != nil {
			fmt.Fprintf(dotin, "%d -> %d [color=green]\n", n.index, n.snext.next[1].index)
		}
		if n.snext.next[0] != nil {
			fmt.Fprintf(dotin, "%d -> %d [color=red]\n", n.index, n.snext.next[0].index)
		}
		if n.snext.next[0] == nil && n.snext.next[1] == nil {
			fmt.Fprintf(dotin, "%d -> %d [color=purple]\n", n.index, n.snext.index)
		}
	})
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
				anc.child = append(anc.child, nod)
			}
			st.push(nod)
		}
		return true
	})
	return root
}

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
	cfg_entry := root.child[1].child[2]
	astdot(root)
	ast_to_cfg(cfg_entry)
	cfgdot(cfg_entry)
}
