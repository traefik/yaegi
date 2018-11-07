package interp

import (
	"fmt"
	"log"
	"reflect"
)

// Builtin type defines functions which run at CFG execution
type Builtin func(f *Frame) Builtin

type BuiltinGenerator func(n *Node)

var builtin = [...]BuiltinGenerator{
	Nop:          nop,
	Addr:         addr,
	ArrayLit:     arrayLit,
	Assign:       assign,
	AssignX:      assignX,
	Assign0:      assign0,
	Add:          add,
	And:          and,
	Call:         call,
	Case:         _case,
	CompositeLit: arrayLit,
	Dec:          nop,
	Equal:        equal,
	GetFunc:      getFunc,
	GetIndex:     getIndex,
	Greater:      greater,
	Inc:          inc,
	Land:         land,
	Lor:          lor,
	Lower:        lower,
	Mul:          mul,
	Negate:       negate,
	Not:          not,
	NotEqual:     notEqual,
	Quotient:     quotient,
	Range:        _range,
	Recv:         recv,
	Remain:       remain,
	Return:       _return,
	Send:         send,
	Slice:        slice,
	Slice0:       slice0,
	Star:         deref,
	Sub:          sub,
	TypeAssert:   typeAssert,
}

func (interp *Interpreter) run(n *Node, cf *Frame) {
	var f *Frame
	if cf == nil {
		f = interp.Frame
	} else {
		f = &Frame{anc: cf, data: make([]reflect.Value, n.flen)}
	}
	for i, t := range n.types {
		if t != nil {
			f.data[i] = reflect.New(t).Elem()
		}
	}
	runCfg(n.start, f)
}

/*
func Run(def *Node, cf *Frame, recv *Node, rseq []int, args []*Node, rets []int, fork bool, goroutine bool) {
	//log.Println("run", def.index, def.child[1].ident, "allocate", def.flen)
	// Allocate a new Frame to store local variables
	anc := cf.anc
	if fork {
		anc = cf
	} else if def.frame != nil {
		anc = def.frame
	}
	f := Frame{anc: anc, data: make([]reflect.Value, def.flen)}
	for i, t := range def.types {
		if t != nil {
			f.data[i] = reflect.New(t).Elem()
		}
	}

	// Assign receiver value, if defined (for methods)
	if recv != nil {
		if rseq != nil {
			//f.data[def.child[0].findex] = valueSeq(recv, rseq, cf) // Promoted method
		} else {
			//f.data[def.child[0].findex] = recv.value(cf)
		}
	}

	// Pass func parameters by value: copy each parameter from caller frame
	// Get list of param indices built by FuncType at CFG
	defargs := def.child[2].child[0]
	paramIndex := defargs.val.([]int)
	i := 0
	//for k, arg := range args {
	for _, arg := range args {
		// Variadic: store remaining args in array
		if i < len(defargs.child) && defargs.child[i].typ.variadic {
			//variadic := make([]interface{}, len(args[k:]))
			//for l, a := range args[k:] {
			//variadic[l] = a.value(cf)
			//}
			//f.data[paramIndex[i]] = variadic
			break
		} else {
			log.Println(def.index, i, arg.index)
			//f.data[paramIndex[i]] = arg.value(cf)
			i++
			// Handle multiple results of a function call argmument
			for j := 1; j < arg.fsize; j++ {
				f.data[paramIndex[i]] = cf.data[arg.findex+j]
				i++
			}
		}
	}
	// Handle empty variadic arg
	//if l := len(defargs.child) - 1; len(args) <= l && defargs.child[l].typ.variadic {
	//	f.data[paramIndex[l]] = []interface{}{}
	//}

	// Execute the function body
	if goroutine {
		go runCfg(def.child[3].start, &f)
	} else {
		runCfg(def.child[3].start, &f)
		// Propagate return values to caller frame
		for i, ret := range rets {
			cf.data[ret] = f.data[i]
		}
	}
}
*/

// Functions set to run during execution of CFG

// runCfg executes a node AST by walking its CFG and running node builtin at each step
func runCfg(n *Node, f *Frame) {
	for exec := n.exec; exec != nil; {
		exec = exec(f)
	}
}

func typeAssert(n *Node) {
	value := genValue(n.child[0])
	i := n.findex
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f)
		return next
	}
}

func convert(n *Node) {
	value := genValue(n.child[1])
	i := n.findex
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f)
		return next
	}
}

func convertFuncBin(n *Node) {
	i := n.findex
	fun := reflect.MakeFunc(n.child[0].typ.rtype, n.child[1].wrapNode)
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = fun
		return next
	}
}

func convertBin(n *Node) {
	i := n.findex
	value := genValue(n.child[1])
	typ := n.child[0].typ.TypeOf()
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = reflect.ValueOf(value(f)).Convert(typ)
		return next
	}
}

// assignX implements multiple value assignement
func assignX(n *Node) {
	l := len(n.child) - 1
	b := n.child[l].findex
	s := n.child[:l]
	next := getExec(n.tnext)
	values := make([]func(*Frame) reflect.Value, l)
	for i, c := range s {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		for i, value := range values {
			value(f).Set(f.data[b+i])
		}
		return next
	}
}

// Indirect assign
func indirectAssign(n *Node) {
	i := n.findex
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		//*(f.data[i].(*interface{})) = value(f)
		f.data[i].Elem().Set(value(f))
		return next
	}
}

// assign implements single value assignement
func assign(n *Node) {
	value := genValue(n)
	value1 := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		value(f).Set(value1(f))
		return next
	}
}

// assign0 implements assignement of zero value
func assign0(n *Node) {
	l := len(n.child) - 1
	zero := n.typ.zero
	s := n.child[:l]
	next := getExec(n.tnext)
	values := make([]func(*Frame) reflect.Value, l)
	for i, c := range s {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		for _, v := range values {
			v(f).Set(zero())
		}
		return next
	}
}

//func assignField(n *Node) Builtin {
//	i := n.findex
//	value := genValue(n.child[1])
//	next := getExec(n.tnext)
//
//	log.Println(n.index, "gen assignField")
//	return func(f *Frame) Builtin {
//		//(*f.data[i].(*interface{})) = value(f)
//		f.data[i].Set(value(f))
//		log.Println(n.index, "in assignField", f.data[i], value(f), i, f.data, f.data[2], f.data[3])
//		return next
//	}
//}

func assignPtrField(n *Node) {
	//i := n.findex
	//value := n.child[1].value
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		//(*f.data[i].(*interface{})) = value(f)
		return next
	}
}

func assignMap(n *Node) {
	value := genValue(n.child[0].child[0])  // map
	value0 := genValue(n.child[0].child[1]) // key
	value1 := genValue(n.child[1])          // value
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		value(f).SetMapIndex(value0(f), value1(f))
		return next
	}
}

func and(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(value0(f).Int() & value1(f).Int())
		return next
	}
}

/* Optimized version of and()
func and(n *Node) Builtin {
	i := n.findex
	i0, v0, r0 := getValue(n.child[0])
	i1, v1, r1 := getValue(n.child[1])
	value0 := n.child[0].value
	//value1 := n.child[1].value
	next := getExec(n.tnext)

	if r0 && r1 {
		return func(f *Frame) Builtin {
			f.data[i].SetInt(f.data[i0].Int() & f.data[i1].Int())
			return next
		}
	} else if r0 {
		iv1 := v1.Int()
		return func(f *Frame) Builtin {
			//f.data[i].SetInt(f.data[i0].Int() & iv1)
			f.data[i].SetInt(value0(f).Int() & iv1)
			return next
		}
	} else if r1 {
		iv0 := v0.Int()
		return func(f *Frame) Builtin {
			f.data[i].SetInt(iv0 & f.data[i1].Int())
			return next
		}
	} else {
		v := v0.Int() & v1.Int()
		return func(f *Frame) Builtin {
			f.data[i].SetInt(v)
			return next
		}
	}
}
*/

func not(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(!value(f).Bool())
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			if !value(f).Bool() {
				return tnext
			}
			return fnext
		}
	}
}

func addr(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).Addr()
		return next
	}
}

func deref(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).Elem()
		return next
	}
}

func _println(n *Node) {
	child := n.child[1:]
	next := getExec(n.tnext)
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		for i, value := range values {
			if i > 0 {
				fmt.Printf(" ")
			}
			fmt.Printf("%v", value(f))

			// Handle multiple results of a function call argmument
			for j := 1; j < child[i].fsize; j++ {
				fmt.Printf(" %v", f.data[child[i].findex+j])
			}
		}
		fmt.Println("")
		return next
	}
}

func _panic(n *Node) {
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		log.Panic("in _panic")
		return next
	}
}

// wrapNode wraps a call to an interpreter node in a function that can be called from runtime
func (n *Node) wrapNode(in []reflect.Value) []reflect.Value {
	def := n.val.(*Node)
	//log.Println(n.index, "in wrapNode", def.index, n.frame)
	var result []reflect.Value
	if n.frame == nil {
		n.frame = n.interp.Frame
	}
	frame := Frame{anc: n.frame, data: make([]reflect.Value, def.flen)}

	// If fucnction is a method, set its receiver data in the frame
	if len(def.child[0].child) > 0 {
		//frame.data[def.child[0].findex] = n.recv.value(n.frame)
	}

	// Unwrap input arguments from their reflect value and store them in the frame
	i := 0
	for _, arg := range in {
		frame.data[def.framepos[i]] = arg
		i++
	}

	// Interpreter code execution
	runCfg(def.child[3].start, &frame)

	// Wrap output results in reflect values and return them
	if len(def.child[2].child) > 1 {
		if fieldList := def.child[2].child[1]; fieldList != nil {
			result = make([]reflect.Value, len(fieldList.child))
			for i := range fieldList.child {
				result[i] = reflect.ValueOf(frame.data[i])
			}
		}
	}
	return result
}

func call2(n *Node) {
	next := getExec(n.tnext)
	value := genValue(n.child[0])
	child := n.child[1:]
	goroutine := n.anc.kind == GoStmt

	// Compute input argument value functions
	var values []func(*Frame) reflect.Value
	for _, c := range child {
		values = append(values, genValue(c))
	}

	// compute frame indexes for return values
	ret := make([]int, len(n.child[0].typ.ret))
	for i := range n.child[0].typ.ret {
		ret[i] = n.findex + i
	}

	n.exec = func(f *Frame) Builtin {
		def := value(f).Interface().(*Node)
		in := make([]reflect.Value, len(child))
		if def.frame != nil {
			f = def.frame
		}
		for i, v := range values {
			in[i] = v(f)
		}
		out := def.fun(f, in, goroutine)
		log.Println(n.index, "out:", out, ret, f.data)
		// Propagate return values to caller frame
		for i, r := range ret {
			log.Println(n.index, out[i], r)
			f.data[r] = out[i]
		}
		return next
	}
}

// FIXME: handle case where func return a boolean
func call(n *Node) {
	goroutine := n.anc.kind == GoStmt
	method := n.child[0].recv != nil
	var values []func(*Frame) reflect.Value
	//var recv *Node
	//var rseq []int

	//if n.child[0].kind == SelectorExpr && n.child[0].typ.cat != SrcPkgT && n.child[0].typ.cat != BinPkgT {
	//	// compute method object receiver
	//	recv = n.child[0].recv
	//	//rseq = n.child[0].child[1].val.([]int)
	//	log.Println(n.index, "recv typ", recv.typ.cat, n.child[0].typ.cat)
	//	//if recv.typ.cat == StructT {
	//	//	values = append(values, genValuePtr(recv))
	//	//} else {
	//	values = append(values, genValue(recv))
	//	//}
	//}

	// Compute method receiver value
	if method {
		values = append(values, genValueRecv(n.child[0]))
	}

	next := getExec(n.tnext)
	value := genValue(n.child[0])
	child := n.child[1:]
	// compute input argument value functions
	for _, c := range child {
		values = append(values, genValue(c))
	}

	// compute frame indexes for return values
	ret := make([]int, len(n.child[0].typ.ret))
	for i := range n.child[0].typ.ret {
		ret[i] = n.findex + i
	}

	n.exec = func(f *Frame) Builtin {
		def := value(f).Interface().(*Node)
		anc := f
		// Get closure frame context (if any)
		if def.frame != nil {
			anc = def.frame
		}
		nf := Frame{anc: anc, data: make([]reflect.Value, def.flen)}

		// Init local frame values
		for i, t := range def.types {
			if t != nil {
				nf.data[i] = reflect.New(t).Elem()
			}
		}
		// copy input parameters from caller
		for i, v := range values {
			log.Println(n.index, i, def.framepos[i], nf.data[def.framepos[i]].Kind(), v(f).Kind())
			nf.data[def.framepos[i]].Set(v(f))
		}

		// Execute function body
		if goroutine {
			go runCfg(def.child[3].start, &nf)
		} else {
			runCfg(def.child[3].start, &nf)
			// Propagate return values to caller frame
			for i, r := range ret {
				f.data[r] = nf.data[i]
			}
		}
		return next
	}
}

// Same as callBin, but for handling f(g()) where g returns multiple values
// FIXME: handle case where func return a boolean
func callBinX(n *Node) {
	next := getExec(n.tnext)
	value := genValue(n.child[0])

	n.exec = func(f *Frame) Builtin {
		l := n.child[1].fsize
		b := n.child[1].findex
		in := make([]reflect.Value, l)
		for i := 0; i < l; i++ {
			in[i] = reflect.ValueOf(f.data[b+i])
		}
		fun := value(f)
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i]
		}
		return next
	}
}

// Call a function from a bin import, accessible through reflect
// FIXME: handle case where func return a boolean
func callDirectBin(n *Node) {
	next := getExec(n.tnext)
	child := n.child[1:]
	value := genValue(n.child[0])
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		in := make([]reflect.Value, len(n.child)-1)
		for i, v := range values {
			if child[i].kind == Rvalue {
				in[i] = v(f)
				child[i].frame = f
			} else {
				in[i] = v(f)
			}
		}
		fun := value(f)
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i]
		}
		return next
	}
}

// FIXME: handle case where func return a boolean
// Call a function from a bin import, accessible through reflect
func callBin(n *Node) {
	next := getExec(n.tnext)
	child := n.child[1:]
	l := len(child)
	value := genValue(n.child[0])
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		in := make([]reflect.Value, l)
		for i, v := range values {
			in[i] = v(f)
		}
		v := value(f).Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i]
		}
		return next
	}
}

// Call a method defined by an interface type on an object returned by a bin import, through reflect.
// In that case, the method func value can be resolved only at execution from the actual value
// of node, not during CFG.
func callBinInterfaceMethod(n *Node, f *Frame) {}

// Call a method on an object returned by a bin import function, through reflect
// FIXME: handle case where func return a boolean
func callBinMethod(n *Node) {
	next := getExec(n.tnext)
	child := n.child[1:]
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}
	rvalue := genValue(n.child[0].child[0])

	n.exec = func(f *Frame) Builtin {
		fun := n.child[0].rval
		in := make([]reflect.Value, len(n.child))
		//val := n.child[0].child[0].value(f)
		//switch val.(type) {
		//case reflect.Value:
		//	in[0] = val.(reflect.Value)
		//default:
		//	in[0] = reflect.ValueOf(val)
		//}
		in[0] = rvalue(f)
		for i, c := range n.child[1:] {
			if c.kind == Rvalue {
				//in[i+1] = c.value(f).(reflect.Value)
				in[i+1] = values[i](f)
				c.frame = f
			} else {
				//in[i+1] = reflect.ValueOf(c.value(f))
				in[i+1] = values[i](f)
			}
		}
		//log.Println(n.index, "in callBinMethod", n.ident, in)
		if !fun.IsValid() {
			fun = in[0].MethodByName(n.child[0].child[1].ident)
			in = in[1:]
		}
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i]
		}
		return next
	}
}

// Same as callBinMethod, but for handling f(g()) where g returns multiple values
// FIXME: handle case where func return a boolean
//func callBinMethodX(n *Node) Builtin {
//	next := getExec(n.tnext)
//
//	return func(f *Frame) Builtin {
//		//fun := n.child[0].value(f).(reflect.Value)
//		fun := n.child[0].value(f)
//		l := n.child[1].fsize
//		b := n.child[1].findex
//		in := make([]reflect.Value, l+1)
//		in[0] = reflect.ValueOf(n.child[0].child[0].value(f))
//		for i := 0; i < l; i++ {
//			in[i+1] = reflect.ValueOf(f.data[b+i])
//		}
//		v := fun.Call(in)
//		for i := 0; i < n.fsize; i++ {
//			f.data[n.findex+i] = v[i]
//		}
//		return next
//	}
//}

func getPtrIndexAddr(n *Node) {
	//i := n.findex
	//value0 := n.child[0].value
	//value1 := n.child[1].value
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		log.Println(n.index, "in getPtrIndexAddr")
		//a := (*value0(f).(*interface{})).([]interface{})
		//f.data[i] = &a[value1(f).(int)]
		return next
	}
}

func getIndexAddr(n *Node) {
	//i := n.findex
	//value0 := n.child[0].value
	//value1 := n.child[1].value
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		log.Println(n.index, "in getIndexAddr")
		//a := value0(f).([]interface{})
		//f.data[i] = &a[value1(f).(int)]
		return next
	}
}

func getPtrIndex(n *Node) {
	i := n.findex
	//value0 := n.child[0].value
	//value1 := n.child[1].value
	next := getExec(n.tnext)
	fi := n.child[1].val.(int)
	value := genValue(n.child[0])

	n.exec = func(f *Frame) Builtin {
		// if error, fallback to getIndex, to make receiver methods work both with pointers and objects
		//if a, ok := value0(f).(*interface{}); ok {
		//	f.data[i] = (*a).([]interface{})[value1(f).(int)]
		//} else {
		//	a := value0(f).([]interface{})
		//	f.data[i] = a[value1(f).(int)]
		//}
		f.data[i] = value(f).Elem().Field(fi)
		return next
	}
}

func getPtrIndexBin(n *Node) {
	//i := n.findex
	//fi := n.val.([]int)
	//value := n.child[0].value
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		log.Println(n.index, "in getPtrIndexBin")
		//a := reflect.ValueOf(value(f)).Elem()
		//f.data[i] = a.FieldByIndex(fi).Interface()
		return next
	}
}

func getIndexBinMethod(n *Node) {
	//i := n.findex
	//value := n.child[0].value
	//ident := n.child[1].ident
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		log.Println(n.index, "in getIndexBinMethod")
		//a := reflect.ValueOf(value(f))
		//f.data[i] = a.MethodByName(ident)
		return next
	}
}

func getIndexBin(n *Node) {
	i := n.findex
	fi := n.val.([]int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		a := reflect.ValueOf(value(f))
		f.data[i] = a.FieldByIndex(fi)
		return next
	}
}

/*
func getIndex(n *Node) Builtin {
	//i := n.findex
	//value0 := n.child[0].value
	//value1 := n.child[1].value
	next := getExec(n.tnext)

	return func(f *Frame) Builtin {
		//a := value0(f).([]interface{})
		//f.data[i] = a[value1(f).(int)]
		return next
	}
}
*/

func getIndex(n *Node) {
	i := n.findex
	next := getExec(n.tnext)
	fi := n.child[1].val.(int)
	value := genValue(n.child[0])

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).Field(fi)
		return next
	}
}

func getIndexArray(n *Node) {
	i := n.findex
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value0(f).Index(int(value1(f).Int()))
		return next
	}
}

// getIndexMap retrieves map value from index
func getIndexMap(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0]) // map
	value1 := genValue(n.child[1]) // index
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value0(f).MapIndex(value1(f))
		return next
	}
}

// getIndexMap2 retrieves map value from index and set status
func getIndexMap2(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])     // map
	value1 := genValue(n.child[1])     // index
	value2 := genValue(n.anc.child[1]) // status
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value0(f).MapIndex(value1(f))
		value2(f).SetBool(f.data[i].IsValid())
		return next
	}
}

func getFunc(n *Node) {
	i := n.findex
	next := getExec(n.tnext)
	genFun(n)

	n.exec = func(f *Frame) Builtin {
		frame := *f
		node := *n
		node.val = &node
		node.frame = &frame
		f.data[i] = reflect.ValueOf(&node)
		return next
	}
}

func getIndexSeq(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	index := n.child[1].val.([]int)
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).FieldByIndex(index)
		return next
	}
}

//func valueSeq(n *Node, seq []int, f *Frame) interface{} {
//	a := f.data[n.findex].([]interface{})
//	l := len(seq) - 1
//	for _, i := range seq[:l] {
//		a = a[i].([]interface{})
//	}
//	return a[seq[l]]
//}

func mul(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(value0(f).Int() * value1(f).Int())
		return next
	}
}

func quotient(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(value0(f).Int() / value1(f).Int())
		return next
	}
}

func remain(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(value0(f).Int() % value1(f).Int())
		return next
	}
}

func negate(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(-value(f).Int())
		return next
	}
}

func add(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(value0(f).Int() + value1(f).Int())
		return next
	}
}

func sub(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(value0(f).Int() - value1(f).Int())
		return next
	}
}

func equal(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value0(f).Int() == value1(f).Int())
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			if value0(f).Int() == value1(f).Int() {
				return tnext
			}
			return fnext
		}
	}
}

func notEqual(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value0(f).Int() != value1(f).Int())
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			if value0(f).Int() != value1(f).Int() {
				return tnext
			}
			return fnext
		}
	}
}

func indirectInc(n *Node) {
	//i := n.findex
	//value := n.child[0].value
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		//*(f.data[i].(*interface{})) = value(f).(int) + 1
		return next
	}
}

func inc(n *Node) {
	value := genValue(n)
	value0 := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		value(f).SetInt(value0(f).Int() + 1)
		return next
	}
}

func greater(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value0(f).Int() > value1(f).Int())
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			if value0(f).Int() > value1(f).Int() {
				return tnext
			}
			return fnext
		}
	}
}

// TODO: avoid always forced execution of 2nd expression member
func land(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			var v bool
			if v = value0(f).Bool(); v {
				v = value1(f).Bool()
			}
			f.data[i].SetBool(v)
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			var v bool
			if v = value0(f).Bool(); v {
				v = value1(f).Bool()
			}
			if v {
				return tnext
			}
			return fnext
		}
	}
}

// TODO: avoid always forced execution of 2nd expression member
func lor(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			var v bool
			if v = value0(f).Bool(); !v {
				v = value1(f).Bool()
			}
			f.data[i].SetBool(v)
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			var v bool
			if v = value0(f).Bool(); !v {
				v = value1(f).Bool()
			}
			f.data[i].SetBool(v)
			if v {
				return tnext
			}
			return fnext
		}
	}
}

func lower(n *Node) {
	i := n.findex
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value0(f).Int() < value1(f).Int())
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			if value0(f).Int() < value1(f).Int() {
				return tnext
			}
			return fnext
		}
	}
}

func nop(n *Node) {
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		return next
	}
}

// TODO: optimize return according to nb of child
func _return(n *Node) {
	child := n.child
	next := getExec(n.tnext)
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		for i, value := range values {
			f.data[i] = value(f)
		}
		return next
	}
}

func arrayLit(n *Node) {
	ind := n.findex
	next := getExec(n.tnext)
	child := n.child[1:]
	zero := n.typ.zero
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	if n.typ.size > 0 {
		// Fixed size array
		n.exec = func(f *Frame) Builtin {
			a := zero()
			for i, v := range values {
				a.Index(i).Set(v(f))
			}
			f.data[ind] = a
			return next
		}
	} else {
		// Slice
		n.exec = func(f *Frame) Builtin {
			a := zero()
			for _, v := range values {
				a = reflect.Append(a, v(f))
			}
			f.data[ind] = a
			return next
		}
	}
}

func mapLit(n *Node) {
	ind := n.findex
	next := getExec(n.tnext)
	child := n.child[1:]
	typ := n.typ.TypeOf()
	keys := make([]func(*Frame) reflect.Value, len(child))
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		keys[i] = genValue(c.child[0])
		values[i] = genValue(c.child[1])
	}

	n.exec = func(f *Frame) Builtin {
		m := reflect.MakeMap(typ)
		for i, k := range keys {
			m.SetMapIndex(k(f), values[i](f))
		}
		f.data[ind] = m
		return next
	}
}

// compositeLit creates a struct object
func compositeLit(n *Node) {
	ind := n.findex
	next := getExec(n.tnext)
	child := n.child[1:]
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		a := n.typ.zero()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		f.data[ind] = a
		return next
	}
}

// compositeSparse creates a struct Object, filling fields from sparse key-values
func compositeSparse(n *Node) {
	ind := n.findex
	next := getExec(n.tnext)
	child := n.child[1:]
	values := make(map[int]func(*Frame) reflect.Value)
	for _, c := range child {
		values[c.findex] = genValue(c.child[1])
	}

	n.exec = func(f *Frame) Builtin {
		a := n.typ.zero()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		f.data[ind] = a
		return next
	}
}

func empty(n *Node) {}

func _range(n *Node) {
	index0 := n.child[0].findex   // array index location in frame
	index1 := n.child[1].findex   // array value location in frame
	value := genValue(n.child[2]) // array
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		a := value(f)
		v0 := f.data[index0]
		v0.SetInt(v0.Int() + 1)
		i := int(v0.Int())
		if i >= a.Len() {
			return fnext
		}
		f.data[index1].Set(a.Index(i))
		return tnext
	}

	// Init sequence
	next := n.exec
	n.child[0].exec = func(f *Frame) Builtin {
		f.data[index0].SetInt(-1)
		return next
	}
}

func rangeMap(n *Node) {
	index0 := n.child[0].findex   // array index location in frame
	index1 := n.child[1].findex   // array value location in frame
	value := genValue(n.child[2]) // array
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)
	// TODO: move i and keys to frame
	var i int
	var keys []reflect.Value

	n.exec = func(f *Frame) Builtin {
		a := value(f)
		i++
		if i >= a.Len() {
			return fnext
		}
		f.data[index0].Set(keys[i])
		f.data[index1].Set(a.MapIndex(keys[i]))
		return tnext
	}

	// Init sequence
	next := n.exec
	n.child[0].exec = func(f *Frame) Builtin {
		keys = value(f).MapKeys()
		i = -1
		return next
	}
}

func _case(n *Node) {
	//i := n.findex
	value0 := genValue(n.anc.anc.child[0])
	value1 := genValue(n.child[0])
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)

	n.exec = func(f *Frame) Builtin {
		if value0(f).Interface() == value1(f).Interface() {
			return tnext
		}
		return fnext
	}
}

// TODO: handle variable number of arguments to append
func _append(n *Node) {
	i := n.findex
	value0 := genValue(n.child[1])
	value1 := genValue(n.child[2])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = reflect.Append(value0(f), value1(f))
		return next
	}
}

func _cap(n *Node) {
	i := n.findex
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(int64(value(f).Cap()))
		return next
	}
}

func _len(n *Node) {
	i := n.findex
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i].SetInt(int64(value(f).Len()))
		return next
	}
}

// _make allocates and initializes a slice, a map or a chan.
func _make(n *Node) {
	i := n.findex
	next := getExec(n.tnext)
	typ := n.child[1].typ.TypeOf()

	switch n.child[1].typ.cat {
	case ArrayT:
		value := genValue(n.child[2])

		switch len(n.child) {
		case 3:
			n.exec = func(f *Frame) Builtin {
				len := int(value(f).Int())
				f.data[i] = reflect.MakeSlice(typ, len, len)
				return next
			}
		case 4:
			value1 := genValue(n.child[3])
			n.exec = func(f *Frame) Builtin {
				f.data[i] = reflect.MakeSlice(typ, int(value(f).Int()), int(value1(f).Int()))
				return next
			}
		}

	case ChanT:
		switch len(n.child) {
		case 2:
			n.exec = func(f *Frame) Builtin {
				f.data[i] = reflect.MakeChan(typ, 0)
				return next
			}
		case 3:
			value := genValue(n.child[2])
			n.exec = func(f *Frame) Builtin {
				f.data[i] = reflect.MakeChan(typ, int(value(f).Int()))
				return next
			}
		}

	case MapT:
		switch len(n.child) {
		case 2:
			n.exec = func(f *Frame) Builtin {
				f.data[i] = reflect.MakeMap(typ)
				return next
			}
		case 3:
			value := genValue(n.child[2])
			n.exec = func(f *Frame) Builtin {
				f.data[i] = reflect.MakeMapWithSize(typ, int(value(f).Int()))
				return next
			}
		}
	}
}

// recv reads from a channel
func recv(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i], _ = value(f).Recv()
		return next
	}
}

// Write to a channel
func send(n *Node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])

	n.exec = func(f *Frame) Builtin {
		value0(f).Send(value1(f))
		return next
	}
}

// slice expression: array[low:high:max]
func slice(n *Node) {
	i := n.findex
	next := getExec(n.tnext)
	value0 := genValue(n.child[0]) // array
	value1 := genValue(n.child[1]) // low (if 2 or 3 args) or high (if 1 arg)

	switch len(n.child) {
	case 2:
		n.exec = func(f *Frame) Builtin {
			a := value0(f)
			f.data[i] = a.Slice(int(value1(f).Int()), a.Len())
			return next
		}
	case 3:
		value2 := genValue(n.child[2]) // max

		n.exec = func(f *Frame) Builtin {
			a := value0(f)
			f.data[i] = a.Slice(int(value1(f).Int()), int(value2(f).Int()))
			return next
		}
	case 4:
		value2 := genValue(n.child[2])
		value3 := genValue(n.child[3])

		n.exec = func(f *Frame) Builtin {
			a := value0(f)
			f.data[i] = a.Slice3(int(value1(f).Int()), int(value2(f).Int()), int(value3(f).Int()))
			return next
		}
	}
}

// slice expression, no low value: array[:high:max]
func slice0(n *Node) {
	i := n.findex
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])

	switch len(n.child) {
	case 1:
		n.exec = func(f *Frame) Builtin {
			a := value0(f)
			f.data[i] = a.Slice(0, a.Len())
			return next
		}
	case 2:
		value1 := genValue(n.child[1])
		n.exec = func(f *Frame) Builtin {
			a := value0(f)
			f.data[i] = a.Slice(0, int(value1(f).Int()))
			return next
		}
	case 3:
		value1 := genValue(n.child[1])
		value2 := genValue(n.child[2])
		n.exec = func(f *Frame) Builtin {
			a := value0(f)
			f.data[i] = a.Slice3(0, int(value1(f).Int()), int(value2(f).Int()))
			return next
		}
	}
}

func isNil(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value(f).IsNil())
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			if value(f).IsNil() {
				return tnext
			}
			return fnext
		}
	}
}

func isNotNil(n *Node) {
	i := n.findex
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext == nil {
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(!value(f).IsNil())
			return tnext
		}
	} else {
		fnext := getExec(n.fnext)

		n.exec = func(f *Frame) Builtin {
			if value(f).IsNil() {
				return fnext
			}
			return tnext
		}
	}
}
