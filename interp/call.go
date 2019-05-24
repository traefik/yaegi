package interp

import (
	"reflect"
	"unsafe"
)

func call2(n *Node) {
	goroutine := n.anc.kind == GoStmt
	//var method bool
	//value := genValue(n.child[0])
	var values []func(*Frame) reflect.Value
	if n.child[0].recv != nil {
		// Compute method receiver value
		values = append(values, genValueRecv(n.child[0]))
		//method = true
	} else if n.child[0].action == Method {
		// add a place holder for interface method receiver
		values = append(values, nil)
		//method = true
	}
	numRet := len(n.child[0].typ.ret)
	variadic := variadicPos(n)
	child := n.child[1:]
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)

	// compute input argument value functions
	for i, c := range child {
		switch {
		case isBinCall(c):
			// Handle nested function calls: pass returned values as arguments
			numOut := c.child[0].typ.rtype.NumOut()
			for j := 0; j < numOut; j++ {
				ind := c.findex + j
				values = append(values, func(f *Frame) reflect.Value { return f.data[ind] })
			}
		case isRegularCall(c):
			// Arguments are return values of a nested function call
			for j := range c.child[0].typ.ret {
				ind := c.findex + j
				values = append(values, func(f *Frame) reflect.Value { return f.data[ind] })
			}
		default:
			if c.kind == BasicLit {
				var argType reflect.Type
				if variadic >= 0 && i >= variadic {
					argType = n.child[0].typ.arg[variadic].TypeOf()
				} else {
					argType = n.child[0].typ.arg[i].TypeOf()
				}
				convertLiteralValue(c, argType)
			}
			if len(n.child[0].typ.arg) > i && n.child[0].typ.arg[i].cat == InterfaceT {
				values = append(values, genValueInterface(c))
			} else {
				values = append(values, genValue(c))
			}
		}
	}

	rtypes := n.child[0].typ.ret
	rvalues := make([]func(*Frame) reflect.Value, len(rtypes))
	switch n.anc.kind {
	case DefineX, AssignXStmt:
		for i := range rvalues {
			c := n.anc.child[i]
			if c.ident != "_" {
				rvalues[i] = genValue(c)
			}
		}
	default:
		for i := range rtypes {
			j := n.findex + i
			rvalues[i] = func(f *Frame) reflect.Value { return f.data[j] }
		}
	}

	//log.Println("call", n.child[0].ident, n.child[0].val.(*Node).types)

	def := n.child[0].val.(*Node)
	n.exec = func(f *Frame) Builtin {
		//def := value(f).Interface().(*Node)
		anc := f
		lints := [5]int{}
		_ = lints
		// Get closure frame context (if any)
		//if def.frame != nil {
		//	anc = def.frame
		//}
		rvals := [6]reflect.Value{}
		rv := rvals[:]
		_ = rv
		//nf := Frame{anc: anc, data: make([]reflect.Value, len(def.types))}
		nf := Frame{anc: anc, data: rv}
		//var vararg reflect.Value

		// Init return values
		for i, v := range rvalues {
			if v != nil {
				nf.data[i] = v(f)
			} else {
				nf.data[i] = reflect.New(def.types[i]).Elem()
			}
		}

		// Init local frame values
		for i, t := range def.types[numRet:] {
			nf.data[numRet+i] = reflect.NewAt(t, unsafe.Pointer(&lints[i])).Elem()
			//nf.data[numRet+i] = reflect.New(t).Elem()
		}

		// Init variadic argument vector
		if variadic >= 0 {
			//vararg = nf.data[numRet+variadic]
		}

		// Copy input parameters from caller
		dest := nf.data[numRet:]
		for i, v := range values {
			switch {
			/*
				case method && i == 0:
					// compute receiver
					var src reflect.Value
					if v == nil {
						src = def.recv.val
						if len(def.recv.index) > 0 {
							if src.Kind() == reflect.Ptr {
								src = src.Elem().FieldByIndex(def.recv.index)
							} else {
								src = src.FieldByIndex(def.recv.index)
							}
						}
					} else {
						src = v(f)
					}
					// Accommodate to receiver type
					d := dest[0]
					if ks, kd := src.Kind(), d.Kind(); ks != kd {
						if kd == reflect.Ptr {
							d.Set(src.Addr())
						} else {
							d.Set(src.Elem())
						}
					} else {
						d.Set(src)
					}
				case variadic >= 0 && i >= variadic:
					vararg.Set(reflect.Append(vararg, v(f)))
			*/
			default:
				dest[i].Set(v(f))
			}
		}

		// Execute function body
		if goroutine {
			go runCfg(def.child[3].start, &nf)
			return tnext
		}
		runCfg(def.child[3].start, &nf)

		// Handle branching according to boolean result
		if fnext != nil && !nf.data[0].Bool() {
			return fnext
		}
		return tnext
	}
}
