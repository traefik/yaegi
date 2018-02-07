package interp

import "fmt"

func (i *Interpreter) Run(entry *Node) {
	// Init Frame
	f := &Frame{val: make([]interface{}, i.size)}
	i.frame = f

	// Start execution by runnning entry function and go to next
	for n := entry; n != nil; {
		n.run(n, f)
		if n.fnext == nil || value(n, f).(bool) {
			n = n.tnext
		} else {
			n = n.fnext
		}
	}
}

// Functions set to run during execution of CFG

func value(n *Node, f *Frame) interface{} {
	if n.isConst {
		return n.val
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

func (i *Interpreter) call(n *Node, f *Frame) {
	if n.Child[0].ident == "println" {
		printa(n.Child[1:], f)
		return
	}
	fn := i.def[n.Child[0].ident]
	fmt.Println("call node", fn.index, fn.Child[2].findex)
	panic("function not implemented")
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
