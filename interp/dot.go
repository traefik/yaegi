package interp

import (
	"fmt"
	"go/ast"
	"os/exec"
	"reflect"
)

// For debug: display an AST in graphviz dot(1) format using dotty(1) co-process
func (n *Node) AstDot() {
	cmd := exec.Command("dotty", "-")
	dotin, err := cmd.StdinPipe()
	if err != nil {
		panic("dotty stdin error")
	}
	cmd.Start()
	fmt.Fprintf(dotin, "digraph ast {\n")
	n.Walk(func(n *Node) {
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
func (n *Node) CfgDot() {
	cmd := exec.Command("dotty", "-")
	dotin, err := cmd.StdinPipe()
	if err != nil {
		panic("dotty stdin error")
	}
	cmd.Start()
	fmt.Fprintf(dotin, "digraph cfg {\n")
	n.Walk(nil, func(n *Node) {
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
