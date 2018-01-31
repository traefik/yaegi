package interp

import "fmt"

func RunCfg(entry *Node) {
	sym = make(map[string]*interface{}) // FIXME: should be done elsewhere
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

// Functions set to run during execution of CFG

var sym map[string]*interface{} // FIXME: should be part of interpreter

func assign(n *Node) {
	name := n.Child[0].ident   // symbol name is in the expr LHS
	sym[name] = n.Child[1].val // Set symbol value
	n.Child[0].val = sym[name]
	n.val = sym[name]
}

func cond_branch(n *Node) {
	if (*n.val).(bool) {
		n.snext = n.next[1]
	} else {
		n.snext = n.next[0]
	}
}

func and(n *Node) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = sym[child.ident]
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

func call(n *Node) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = sym[child.ident]
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

func equal(n *Node) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = sym[child.ident]
		}
	}
	*n.val = (*n.Child[0].val).(int64) == (*n.Child[1].val).(int64)
}

func inc(n *Node) {
	n.Child[0].val = sym[n.Child[0].ident]
	*n.Child[0].val = (*n.Child[0].val).(int64) + 1
	*n.val = *n.Child[0].val
}

func lower(n *Node) {
	for _, child := range n.Child {
		if child.ident != "" {
			child.val = sym[child.ident]
		}
	}
	*n.val = (*n.Child[0].val).(int64) < (*n.Child[1].val).(int64)
}

func nop(n *Node) {}
