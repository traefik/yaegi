package interp

import (
	"fmt"
	"reflect"
	"strings"
)

// set trace to true for debugging the cfg and other processes
var trace = false

func traceIndent(n *node) string {
	return strings.Repeat("  ", n.depth())
}

// tracePrintln works like fmt.Println, with indenting by depth
// and key info on given node.
func tracePrintln(n *node, v ...any) {
	if !trace {
		return
	}
	fmt.Println(append([]any{traceIndent(n), n}, v...)...)
}

// tracePrintTree is particularly useful in post-order for seeing the full
// structure of a given code segment of interest.
func tracePrintTree(n *node, v ...any) {
	if !trace {
		return
	}
	tracePrintln(n, v...)
	n.Walk(func(n *node) bool {
		tracePrintln(n)
		return true
	}, nil)
}

// nodeAddr returns the pointer address of node, short version
func ptrAddr(v any) string {
	p := fmt.Sprintf("%p", v)
	return p[:2] + p[9:] // unique bits
}

// valString returns string rep of given value, showing underlying pointers etc
func valString(v reflect.Value) string {
	s := v.String()
	if v.Kind() == reflect.Func || v.Kind() == reflect.Map || v.Kind() == reflect.Pointer || v.Kind() == reflect.Slice || v.Kind() == reflect.UnsafePointer {
		p := fmt.Sprintf("%#x", v.Pointer())
		ln := len(p)
		s += " " + p[:2] + p[max(2, ln-4):]
	}
	return s
}

func (n *node) String() string {
	s := n.kind.String()
	if n.ident != "" {
		s += " " + n.ident
	}
	s += " " + ptrAddr(n)
	if n.sym != nil {
		s += " sym:" + n.sym.String()
	} else if n.typ != nil {
		s += " typ:" + n.typ.String()
	}
	if n.findex >= 0 {
		s += fmt.Sprintf(" fidx: %d lev: %d", n.findex, n.level)
	}
	if n.start != nil && n.start != n {
		s += fmt.Sprintf(" ->start: %s %s", n.start.kind.String(), ptrAddr(n.start))
	}
	if n.tnext != nil {
		s += fmt.Sprintf(" ->tnext: %s %s", n.tnext.kind.String(), ptrAddr(n.tnext))
	}
	if n.fnext != nil {
		s += fmt.Sprintf(" ->fnext: %s %s", n.fnext.kind.String(), ptrAddr(n.fnext))
	}
	return s
}

func (n *node) depth() int {
	if n.anc != nil {
		return n.anc.depth() + 1
	}
	return 0
}

func (sy *symbol) String() string {
	s := sy.kind.String()
	if sy.typ != nil {
		s += " (" + sy.typ.String() + ")"
	}
	if sy.rval.IsValid() {
		s += " = " + sy.rval.String()
	}
	if sy.index >= 0 {
		s += fmt.Sprintf(" idx: %d", sy.index)
	}
	if sy.node != nil {
		s += " " + sy.node.String()
	}
	return s
}

func (t *itype) String() string {
	if t.str != "" {
		return t.str
	}
	s := t.cat.String()
	if t.name != "" {
		s += " (" + t.name + ")"
	}
	return s
}
