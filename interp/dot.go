package interp

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// For debug: display an AST in graphviz dot(1) format using dotty(1) co-process
func (n *Node) AstDot(out io.WriteCloser) {
	fmt.Fprintf(out, "digraph ast {\n")
	n.Walk(func(n *Node) bool {
		var label string
		switch n.kind {
		case BasicLit, Ident:
			label = strings.Replace(n.ident, "\"", "\\\"", -1)
		default:
			if n.action != Nop {
				label = n.action.String()
			} else {
				label = n.kind.String()
			}
		}
		fmt.Fprintf(out, "%d [label=\"%d: %s\"]\n", n.index, n.index, label)
		if n.anc != nil {
			fmt.Fprintf(out, "%d -> %d\n", n.anc.index, n.index)
		}
		return true
	}, nil)
	fmt.Fprintf(out, "}\n")
}

// For debug: display a CFG in graphviz dot(1) format using dotty(1) co-process
func (n *Node) CfgDot(out io.WriteCloser) {
	fmt.Fprintf(out, "digraph cfg {\n")
	n.Walk(nil, func(n *Node) {
		if n.kind == BasicLit || n.kind == Ident || n.tnext == nil {
			return
		}
		var label string
		if n.action == Nop {
			label = "nop: end_" + n.kind.String()
		} else {
			label = n.action.String()
		}
		fmt.Fprintf(out, "%d [label=\"%d: %v %d\"]\n", n.index, n.index, label, n.findex)
		if n.fnext != nil {
			fmt.Fprintf(out, "%d -> %d [color=green]\n", n.index, n.tnext.index)
			fmt.Fprintf(out, "%d -> %d [color=red]\n", n.index, n.fnext.index)
		} else if n.tnext != nil {
			fmt.Fprintf(out, "%d -> %d\n", n.index, n.tnext.index)
		}
	})
	fmt.Fprintf(out, "}\n")
}

// Dotty() returns an output stream to a dotty(1) co-process where to write data in .dot format
func Dotty() io.WriteCloser {
	cmd := exec.Command("dotty", "-")
	dotin, err := cmd.StdinPipe()
	if err != nil {
		panic("dotty stdin error")
	}
	cmd.Start()
	return dotin
}
