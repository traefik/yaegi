package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"reflect"
	"strconv"
)

// Function to run at CFG execution
type runfun func(n *node)

// Structure for AST and CFG
type node struct {
	child []*node      // child subtrees
	anc   *node        // ancestor
	start *node        // entry point in subtree (CFG)
	snext *node        // successor (CFG)
	next  [2]*node     // conditional successors, for false and for true (CFG)
	index int          // node index (dot display)
	run   runfun       // function to run at CFG execution
	val   *interface{} // pointer on generic value (CFG execution)
	ident string       // set if node is a var or func
	isnop bool         // node is a no op
	anode *ast.Node    // original ast node (temporary, will be removed)
}

// Interpreter execution state
type interp struct {
	entry *node // Execution entry point
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
	println("wire_child", reflect.TypeOf(*n.anode).String(), n.index)
	for _, child := range n.child {
		if !child.is_leaf() {
			n.start = child.start
		}
	}
	if n.start == nil {
		println("fix self start", n.index)
		n.start = n
	}
	for i := 1; i < len(n.child); i++ {
		n.child[i-1].snext = n.child[i].start
	}
	for i := len(n.child) - 1; i >= 0; i-- {
		if !n.child[i].is_leaf() {
			println("wire next of", n.child[i].index, "to parent", n.index)
			n.child[i].snext = n
			break
		}
	}
}

// Generate a CFG from AST (wiring successors in AST)
func ast_to_cfg(root *node) {
	walk(root, nil, func(n *node) {
		switch x := (*n.anode).(type) {
		case *ast.BlockStmt:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.child[len(n.child)-1].val
			fmt.Println("block", n.index, n.start, n.snext)
		case *ast.IncDecStmt:
			wire_child(n)
			switch x.Tok {
			case token.INC:
				n.run = inc
			}
		case *ast.AssignStmt:
			n.run = assign
			wire_child(n)
		case *ast.ExprStmt:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.child[len(n.child)-1].val
		case *ast.ParenExpr:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.child[len(n.child)-1].val
		case *ast.BinaryExpr:
			wire_child(n)
			switch x.Op {
			case token.AND:
				n.run = and
			case token.EQL:
				n.run = equal
			case token.LSS:
				n.run = lower
			}
		case *ast.CallExpr:
			wire_child(n)
			n.run = call
		case *ast.IfStmt:
			n.isnop = true
			n.run = nop
			n.start = n.child[0].start
			n.child[1].snext = n
			println("if nchild:", len(n.child))
			if len(n.child) == 3 {
				n.child[2].snext = n
			}
			n.child[0].next[1] = n.child[1].start
			if len(n.child) == 3 {
				n.child[0].next[0] = n.child[2].start
			} else {
				n.child[0].next[0] = n
			}
		case *ast.ForStmt:
			n.isnop = true
			n.run = nop
			// FIXME: works only if for node has 4 children
			n.start = n.child[0].start
			n.child[0].snext = n.child[1].start
			n.child[1].next[0] = n
			n.child[1].next[1] = n.child[3].start
			n.child[3].snext = n.child[2].start
			n.child[2].snext = n.child[1].start
		case *ast.BasicLit:
			// FIXME: values must be converted to int or float if possible
			if v, err := strconv.ParseInt(x.Value, 0, 0); err == nil {
				*n.val = v
			} else {
				*n.val = x.Value
			}
		case *ast.Ident:
			n.ident = x.Name
		default:
			println("unknown type:", reflect.TypeOf(*n.anode).String())
		}
	})
}

// optimisation: rewire CFG to skip nop nodes
func optim_cfg(root *node) {
	walk(root, nil, func(n *node) {
		for s := n.snext; s != nil && s.snext != nil; s = s.snext {
			n.snext = s
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
		if n.next[1] != nil {
			fmt.Fprintf(dotin, "%d -> %d [color=green]\n", n.index, n.next[1].index)
		}
		if n.next[0] != nil {
			fmt.Fprintf(dotin, "%d -> %d [color=red]\n", n.index, n.next[0].index)
		}
		if n.next[0] == nil && n.next[1] == nil && n.snext != nil {
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

// Functions run during execution of CFG
func assign(n *node) {
	name := n.child[0].ident   // symbol name is in the expr LHS
	sym[name] = n.child[1].val // Set symbol value
	n.child[0].val = sym[name]
	n.val = sym[name]
	fmt.Println(name, "=", *n.child[1].val, ":", *n.val)
}

func cond_branch(n *node) {
	if (*n.val).(bool) {
		n.snext = n.next[1]
	} else {
		n.snext = n.next[0]
	}
}

func and(n *node) {
	for _, child := range n.child {
		if child.ident != "" {
			child.val = sym[child.ident]
		}
	}
	*n.val = (*n.child[0].val).(int64) & (*n.child[1].val).(int64)
}

func printa(n []*node) {
	for _, m := range n {
		fmt.Printf("%v", *m.val)
	}
	fmt.Println("")
}

func call(n *node) {
	for _, child := range n.child {
		if child.ident != "" {
			child.val = sym[child.ident]
		}
	}
	switch n.child[0].ident {
	case "println":
		printa(n.child[1:])
	default:
		panic("function not implemented")
	}
}

func equal(n *node) {
	for _, child := range n.child {
		if child.ident != "" {
			child.val = sym[child.ident]
		}
	}
	*n.val = (*n.child[0].val).(int64) == (*n.child[1].val).(int64)
}

func inc(n *node) {
	n.child[0].val = sym[n.child[0].ident]
	*n.child[0].val = (*n.child[0].val).(int64) + 1
	*n.val = *n.child[0].val
}

func lower(n *node) {
	for _, child := range n.child {
		if child.ident != "" {
			child.val = sym[child.ident]
		}
	}
	*n.val = (*n.child[0].val).(int64) < (*n.child[1].val).(int64)
}

func nop(n *node) {}

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
			var i interface{}
			nod := &node{anc: anc, index: index, anode: &n, val: &i}
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

func run_cfg(entry *node) {
	for n := entry; n != nil; {
		n.run(n)
		if n.snext != nil {
			n = n.snext
		} else if n.next[1] == nil && n.next[0] == nil {
			break
		} else if (*n.val).(bool) {
			n = n.next[1]
		} else {
			n = n.next[0]
		}
	}
}

// Symbol table (aka variables), just a global one to start.
// It should be organized in hierarchical scopes and frames
// and belong to an interpreter context.
var sym map[string]*interface{}

func main() {
	const src = `
package main

func main() {
	for a := 0; a < 20000000; a++ {
	//for a := 0; a < 20000; a++ {
		if (a & 0x8ffff) == 0x80000 {
		//if (a & 0x8ff) == 0x800 {
			println(a)
		}
	}
} `

	sym = make(map[string]*interface{})
	root := src_to_ast(src)
	cfg_entry := root.child[1].child[2] // FIXME: entry point should be resolved from 'main' name
	//astdot(root)
	ast_to_cfg(cfg_entry)
	//cfgdot(cfg_entry)
	optim_cfg(cfg_entry)
	//cfgdot(cfg_entry)
	run_cfg(cfg_entry.start)
}
