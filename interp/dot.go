package interp

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
)

// astDot displays an AST in graphviz dot(1) format using dotty(1) co-process
func (n *node) astDot(out io.Writer, name string) {
	fmt.Fprintf(out, "digraph ast {\n")
	fmt.Fprintf(out, "labelloc=\"t\"\n")
	fmt.Fprintf(out, "label=\"%s\"\n", name)
	n.Walk(func(n *node) bool {
		var label string
		switch n.kind {
		case basicLit, identExpr:
			label = strings.Replace(n.ident, "\"", "\\\"", -1)
		default:
			if n.action != aNop {
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

// cfgDot displays a CFG in graphviz dot(1) format using dotty(1) co-process
func (n *node) cfgDot(out io.Writer) {
	fmt.Fprintf(out, "digraph cfg {\n")
	n.Walk(nil, func(n *node) {
		if n.kind == basicLit || n.kind == identExpr || n.tnext == nil {
			return
		}
		var label string
		if n.action == aNop {
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

// dotX returns an output stream to a dot(1) co-process where to write data in .dot format
func dotX() io.WriteCloser {
	cmd := exec.Command("dotty", "-")
	//cmd := exec.Command("dot", "-T", "xlib")
	dotin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		log.Fatal(err)
	}
	return dotin
}
