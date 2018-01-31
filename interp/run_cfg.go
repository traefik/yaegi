package interp

import "fmt"

func (i *Interpreter) RunCfg(entry *Node) {
	for n := entry; n != nil; {
		n.run(n, i)
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

// Functions set to run during execution of CFG

//var sym map[string]*interface{} // FIXME: should be part of interpreter

func assign(n *Node, i *Interpreter) {
	name := n.Child[0].ident     // symbol name is in the expr LHS
	i.sym[name] = n.Child[1].val // Set symbol value
	n.Child[0].val = i.sym[name]
	n.val = i.sym[name]
}

func cond_branch(n *Node, i *Interpreter) {
	if (*n.val).(bool) {
		n.snext = n.next[1]
	} else {
		n.snext = n.next[0]
	}
}

func and(n *Node, i *Interpreter) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = i.sym[child.ident]
		}
	}
	*n.val = (*n.Child[0].val).(int64) & (*n.Child[1].val).(int64)
}

func printa(n []*Node) {
	for _, m := range n {
		fmt.Printf("%v", *m.val)
	}
	fmt.Println("")
}

func call(n *Node, i *Interpreter) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = i.sym[child.ident]
		}
	}
	// Only builtin println is supported
	switch n.Child[0].ident {
	case "println":
		printa(n.Child[1:])
	default:
		panic("function not implemented")
	}
}

func equal(n *Node, i *Interpreter) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = i.sym[child.ident]
		}
	}
	*n.val = (*n.Child[0].val).(int64) == (*n.Child[1].val).(int64)
}

func inc(n *Node, i *Interpreter) {
	n.Child[0].val = i.sym[n.Child[0].ident]
	*n.Child[0].val = (*n.Child[0].val).(int64) + 1
	*n.val = *n.Child[0].val
}

func lower(n *Node, i *Interpreter) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = i.sym[child.ident]
		}
	}
	*n.val = (*n.Child[0].val).(int64) < (*n.Child[1].val).(int64)
}

func nop(n *Node, i *Interpreter) {}
