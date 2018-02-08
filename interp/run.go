package interp

import "fmt"

// Run a Go function
func Run(def *Node, uf *Frame, args []*Node) {
	body := def.Child[2]
	param := def.Child[1].Child[0]

	// Init new Frame
	f := &Frame{val: make([]interface{}, def.findex), up: uf}
	if uf != nil {
		uf.down = f
	}

	// Pass func parameters as value: copy each func parameter from old to new frame
	// Frame locations were pre-computed during cfg
	for i := 0; i < len(args); i++ {
		f.val[param.Child[i].findex] = uf.val[args[i].findex]
	}

	// Start execution by runnning entry function and go to next
	for n := body.Start; n != nil; {
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
	// FIXME: resolve fn during compile, not exec ?
	if fn := i.def[n.Child[0].ident]; fn != nil {
		Run(fn, f, n.Child[1:])
	} else {
		panic("function not found")
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
