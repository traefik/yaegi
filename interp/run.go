package interp

import "fmt"

func (i *Interpreter) Run(entry *Node) {
	// Init Frame
	f := &Frame{val: make([]interface{}, i.size)}
	i.frame = f

	// Start execution by runnning entry function and go to next
	for n := entry; n != nil; {
		n.run(n, f)
		if n.snext != nil {
			n = n.snext
		} else if n.next[1] == nil && n.next[0] == nil {
			break
		} else if value(n, f).(bool) {
			n = n.next[1]
		} else {
			n = n.next[0]
		}
	}
}

// Functions set to run during execution of CFG

func value(n *Node, f *Frame) interface{} {
	if n.isConst {
		return *n.val
	}
	return f.val[n.findex]

}

func assign(n *Node, f *Frame) {
	f.val[n.findex] = value(n.Child[1], f)
}

func and(n *Node, f *Frame) {
	f.val[n.findex] = value(n.Child[0], f).(int64) & value(n.Child[1], f).(int64)
}

func printa(n []*Node, f *Frame) {
	for _, m := range n {
		fmt.Printf("%v", value(m, f))
	}
	fmt.Println("")
}

func call(n *Node, f *Frame) {
	switch n.Child[0].ident {
	case "println":
		printa(n.Child[1:], f)
	default:
		panic("function not implemented")
	}
}

func equal(n *Node, f *Frame) {
	f.val[n.findex] = value(n.Child[0], f).(int64) == value(n.Child[1], f).(int64)
}

func inc(n *Node, f *Frame) {
	f.val[n.findex] = value(n.Child[0], f).(int64) + 1
}

func lower(n *Node, f *Frame) {
	f.val[n.findex] = value(n.Child[0], f).(int64) < value(n.Child[1], f).(int64)
}

func nop(n *Node, i *Frame) {}
