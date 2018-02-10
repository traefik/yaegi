package interp

import "fmt"

// Run a Go function
func Run(def *Node, cf *Frame, args []*Node, rets []int) {
	// Allocate a new Frame to store local variables
	f := Frame(make([]interface{}, def.findex))

	// Pass func parameters by value: copy each parameter from caller frame
	param := def.Child[1].Child[0].Child
	for i, arg := range args {
		f[param[i].findex] = value(arg, cf)
	}

	// Execute by walking the CFG and running node func at each step
	body := def.Child[2]
	for n := body.Start; n != nil; {
		n.run(n, &f)
		if n.fnext == nil || value(n, &f).(bool) {
			n = n.tnext
		} else {
			n = n.fnext
		}
	}

	// Propagate return values to caller frame
	for i, ret := range rets {
		(*cf)[ret] = f[i]
	}
}

// Functions set to run during execution of CFG

func value(n *Node, f *Frame) interface{} {
	if n.isConst {
		return n.val
	}
	return (*f)[n.findex]

}

func assign(n *Node, f *Frame) {
	(*f)[n.findex] = value(n.Child[1], f)
}

func and(n *Node, f *Frame) {
	(*f)[n.findex] = value(n.Child[0], f).(int64) & value(n.Child[1], f).(int64)
}

func printa(n []*Node, f *Frame) {
	for _, m := range n {
		fmt.Printf("%v", value(m, f))
	}
	fmt.Println("")
}

func (interp *Interpreter) call(n *Node, f *Frame) {
	if n.Child[0].ident == "println" {
		printa(n.Child[1:], f)
		return
	}
	// FIXME: resolve fn during compile, not exec ?
	if fn := interp.def[n.Child[0].ident]; fn != nil {
		var rets []int
		if len(fn.Child[1].Child) > 1 {
			if fieldList := fn.Child[1].Child[1]; fieldList != nil {
				rets = make([]int, len(fieldList.Child))
				for i, _ := range fieldList.Child {
					rets[i] = n.findex + i
				}
			}
		}
		Run(fn, f, n.Child[1:], rets)
	} else {
		panic("function not found")
	}
}

func add(n *Node, f *Frame) {
	(*f)[n.findex] = value(n.Child[0], f).(int64) + value(n.Child[1], f).(int64)
}

func equal(n *Node, f *Frame) {
	(*f)[n.findex] = value(n.Child[0], f).(int64) == value(n.Child[1], f).(int64)
}

func inc(n *Node, f *Frame) {
	(*f)[n.findex] = value(n.Child[0], f).(int64) + 1
}

func lower(n *Node, f *Frame) {
	(*f)[n.findex] = value(n.Child[0], f).(int64) < value(n.Child[1], f).(int64)
}

func nop(n *Node, i *Frame) {}

func _return(n *Node, f *Frame) {
	for i, c := range n.Child {
		(*f)[i] = (*f)[c.findex]
	}
}
