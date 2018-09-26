package interp

import (
	"fmt"
	"log"
	"reflect"
	"time"
)

// Builtin type defines functions which run at CFG execution
type Builtin func(f *Frame)

type BuiltinGenerator func(n *Node) Builtin

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
	CallF:        call,
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

// Run a Go function
func Run(def *Node, cf *Frame, recv *Node, rseq []int, args []*Node, rets []int, fork bool, goroutine bool) {
	//log.Println("run", def.index, def.child[1].ident, "allocate", def.flen)
	// Allocate a new Frame to store local variables
	anc := cf.anc
	if fork {
		anc = cf
	} else if def.frame != nil {
		anc = def.frame
	}
	f := Frame{anc: anc, data: make([]interface{}, def.flen)}

	// Assign receiver value, if defined (for methods)
	if recv != nil {
		if rseq != nil {
			f.data[def.child[0].findex] = valueSeq(recv, rseq, cf) // Promoted method
		} else {
			f.data[def.child[0].findex] = recv.value(cf)
		}
	}

	// Pass func parameters by value: copy each parameter from caller frame
	// Get list of param indices built by FuncType at CFG
	defargs := def.child[2].child[0]
	paramIndex := defargs.val.([]int)
	i := 0
	for k, arg := range args {
		// Variadic: store remaining args in array
		if i < len(defargs.child) && defargs.child[i].typ.variadic {
			variadic := make([]interface{}, len(args[k:]))
			for l, a := range args[k:] {
				variadic[l] = a.value(cf)
			}
			f.data[paramIndex[i]] = variadic
			break
		} else {
			f.data[paramIndex[i]] = arg.value(cf)
			i++
			// Handle multiple results of a function call argmument
			for j := 1; j < arg.fsize; j++ {
				f.data[paramIndex[i]] = cf.data[arg.findex+j]
				i++
			}
		}
	}
	// Handle empty variadic arg
	if l := len(defargs.child) - 1; len(args) <= l && defargs.child[l].typ.variadic {
		f.data[paramIndex[l]] = []interface{}{}
	}

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

// Functions set to run during execution of CFG

// Run by walking the CFG and running node builtin at each step
func runCfg(n *Node, f *Frame) {
	for n != nil {
		n.exec(f)
		if n.fnext == nil || n.value(f).(bool) {
			n = n.tnext
		} else {
			n = n.fnext
		}
	}
}

func typeAssert(n *Node) Builtin {
	value := n.child[0].value
	i := n.findex
	return func(f *Frame) { f.data[i] = value(f) }
}

func convert(n *Node) Builtin {
	value := n.child[1].value
	i := n.findex
	return func(f *Frame) { f.data[i] = value(f) }
}

func convertFuncBin(n *Node) Builtin {
	i := n.findex
	fun := reflect.MakeFunc(n.child[0].typ.rtype, n.child[1].wrapNode).Interface()
	return func(f *Frame) { f.data[i] = fun }
}

func convertBin(n *Node) Builtin {
	i := n.findex
	value := n.child[1].value
	typ := n.child[0].typ.TypeOf()
	return func(f *Frame) { f.data[i] = reflect.ValueOf(value(f)).Convert(typ).Interface() }
}

// assignX implements multiple value assignement
func assignX(n *Node) Builtin {
	l := len(n.child) - 1
	b := n.child[l].findex
	s := n.child[:l]
	return func(f *Frame) {
		for i, c := range s {
			*c.pvalue(f) = f.data[b+i]
		}
	}
}

// Indirect assign
func indirectAssign(n *Node) Builtin {
	i := n.findex
	value := n.child[1].value
	return func(f *Frame) { *(f.data[i].(*interface{})) = value(f) }
}

// assign implements single value assignement
func assign(n *Node) Builtin {
	pvalue := n.pvalue
	value := n.child[1].value
	return func(f *Frame) { *pvalue(f) = value(f) }
}

// assign0 implements assignement of zero value
func assign0(n *Node) Builtin {
	l := len(n.child) - 1
	z := n.typ.zero()
	s := n.child[:l]
	return func(f *Frame) {
		for _, c := range s {
			*c.pvalue(f) = z
		}
	}
}

func assignField(n *Node) Builtin {
	i := n.findex
	value := n.child[1].value
	return func(f *Frame) { (*f.data[i].(*interface{})) = value(f) }
}

func assignPtrField(n *Node) Builtin {
	i := n.findex
	value := n.child[1].value
	return func(f *Frame) { (*f.data[i].(*interface{})) = value(f) }
}

func assignMap(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].child[1].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i].(map[interface{}]interface{})[value0(f)] = value1(f) }
}

func and(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) & value1(f).(int) }
}

func not(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	return func(f *Frame) { f.data[i] = !value(f).(bool) }
}

func addr(n *Node) Builtin {
	i := n.findex
	pvalue := n.child[0].pvalue
	return func(f *Frame) { f.data[i] = pvalue(f) }
}

func deref(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	return func(f *Frame) { f.data[i] = *(value(f).(*interface{})) }
}

func _println(n *Node) Builtin {
	child := n.child[1:]
	return func(f *Frame) {
		for i, m := range child {
			if i > 0 {
				fmt.Printf(" ")
			}
			fmt.Printf("%v", m.value(f))

			// Handle multiple results of a function call argmument
			for j := 1; j < m.fsize; j++ {
				fmt.Printf(" %v", f.data[m.findex+j])
			}
		}
		fmt.Println("")
	}
}

//func _panic(n *Node, f *Frame) {
//	log.Panic("in _panic")
//}
func _panic(n *Node) Builtin {
	return func(f *Frame) { log.Panic("in _panic") }
}

// wrapNode wraps a call to an interpreter node in a function that can be called from runtime
func (n *Node) wrapNode(in []reflect.Value) []reflect.Value {
	def := n.val.(*Node)
	//log.Println(n.index, "in wrapNode", def.index, n.frame)
	var result []reflect.Value
	if n.frame == nil {
		n.frame = n.interp.Frame
	}
	frame := Frame{anc: n.frame, data: make([]interface{}, def.flen)}

	// If fucnction is a method, set its receiver data in the frame
	if len(def.child[0].child) > 0 {
		frame.data[def.child[0].findex] = n.recv.value(n.frame)
	}

	// Unwrap input arguments from their reflect value and store them in the frame
	paramIndex := def.child[2].child[0].val.([]int)
	i := 0
	for _, arg := range in {
		frame.data[paramIndex[i]] = arg.Interface()
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

//func call(n *Node, f *Frame) {
//	// TODO: method detection should be done at CFG, and handled in a separate callMethod()
//	var recv *Node
//	var rseq []int
//	var forkFrame bool
//
//	if n.action == CallF {
//		forkFrame = true
//	}
//
//	if n.child[0].kind == SelectorExpr && n.child[0].typ.cat != SrcPkgT {
//		recv = n.child[0].recv
//		rseq = n.child[0].child[1].val.([]int)
//	}
//	fn := n.child[0].value(f).(*Node)
//	var ret []int
//	if len(fn.child[2].child) > 1 {
//		if fieldList := fn.child[2].child[1]; fieldList != nil {
//			ret = make([]int, len(fieldList.child))
//			for i := range fieldList.child {
//				ret[i] = n.findex + i
//			}
//		}
//	}
//	Run(fn, f, recv, rseq, n.child[1:], ret, forkFrame, false)
//}
func call(n *Node) Builtin {
	return func(f *Frame) {
		var recv *Node
		var rseq []int
		var forkFrame bool

		if n.action == CallF {
			forkFrame = true
		}

		if n.child[0].kind == SelectorExpr && n.child[0].typ.cat != SrcPkgT {
			recv = n.child[0].recv
			rseq = n.child[0].child[1].val.([]int)
		}
		fn := n.child[0].value(f).(*Node)
		var ret []int
		if len(fn.child[2].child) > 1 {
			if fieldList := fn.child[2].child[1]; fieldList != nil {
				ret = make([]int, len(fieldList.child))
				for i := range fieldList.child {
					ret[i] = n.findex + i
				}
			}
		}
		Run(fn, f, recv, rseq, n.child[1:], ret, forkFrame, false)
	}
}

// Same as call(), but execute function in a goroutine
//func callGoRoutine(n *Node, f *Frame) {
//	// TODO: method detection should be done at CFG, and handled in a separate callMethod()
//	var recv *Node
//	var rseq []int
//	var forkFrame bool
//
//	if n.action == CallF {
//		forkFrame = true
//	}
//
//	if n.child[0].kind == SelectorExpr {
//		recv = n.child[0].recv
//		rseq = n.child[0].child[1].val.([]int)
//	}
//	fn := n.child[0].value(f).(*Node)
//	var ret []int
//	if len(fn.child[2].child) > 1 {
//		if fieldList := fn.child[2].child[1]; fieldList != nil {
//			ret = make([]int, len(fieldList.child))
//			for i := range fieldList.child {
//				ret[i] = n.findex + i
//			}
//		}
//	}
//	Run(fn, f, recv, rseq, n.child[1:], ret, forkFrame, true)
//}
func callGoRoutine(n *Node) Builtin {
	return func(f *Frame) {
		var recv *Node
		var rseq []int
		var forkFrame bool

		if n.action == CallF {
			forkFrame = true
		}

		if n.child[0].kind == SelectorExpr {
			recv = n.child[0].recv
			rseq = n.child[0].child[1].val.([]int)
		}
		fn := n.child[0].value(f).(*Node)
		var ret []int
		if len(fn.child[2].child) > 1 {
			if fieldList := fn.child[2].child[1]; fieldList != nil {
				ret = make([]int, len(fieldList.child))
				for i := range fieldList.child {
					ret[i] = n.findex + i
				}
			}
		}
		Run(fn, f, recv, rseq, n.child[1:], ret, forkFrame, true)
	}
}

// Same as callBin, but for handling f(g()) where g returns multiple values
//func callBinX(n *Node, f *Frame) {
//	l := n.child[1].fsize
//	b := n.child[1].findex
//	in := make([]reflect.Value, l)
//	for i := 0; i < l; i++ {
//		in[i] = reflect.ValueOf(f.data[b+i])
//	}
//	fun := n.child[0].value(f).(reflect.Value)
//	v := fun.Call(in)
//	for i := 0; i < n.fsize; i++ {
//		f.data[n.findex+i] = v[i].Interface()
//	}
//}
func callBinX(n *Node) Builtin {
	return func(f *Frame) {
		l := n.child[1].fsize
		b := n.child[1].findex
		in := make([]reflect.Value, l)
		for i := 0; i < l; i++ {
			in[i] = reflect.ValueOf(f.data[b+i])
		}
		fun := n.child[0].value(f).(reflect.Value)
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i].Interface()
		}
	}
}

// Call a function from a bin import, accessible through reflect
//func callDirectBin(n *Node, f *Frame) {
//	in := make([]reflect.Value, len(n.child)-1)
//	for i, c := range n.child[1:] {
//		if c.kind == Rvalue {
//			in[i] = c.value(f).(reflect.Value)
//			c.frame = f
//		} else {
//			in[i] = reflect.ValueOf(c.value(f))
//		}
//	}
//	fun := reflect.ValueOf(n.child[0].value(f))
//	v := fun.Call(in)
//	for i := 0; i < n.fsize; i++ {
//		f.data[n.findex+i] = v[i].Interface()
//	}
//}
func callDirectBin(n *Node) Builtin {
	return func(f *Frame) {
		in := make([]reflect.Value, len(n.child)-1)
		for i, c := range n.child[1:] {
			if c.kind == Rvalue {
				in[i] = c.value(f).(reflect.Value)
				c.frame = f
			} else {
				in[i] = reflect.ValueOf(c.value(f))
			}
		}
		fun := reflect.ValueOf(n.child[0].value(f))
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i].Interface()
		}
	}
}

// Call a function from a bin import, accessible through reflect
//func callBin(n *Node, f *Frame) {
//	in := make([]reflect.Value, len(n.child)-1)
//	for i, c := range n.child[1:] {
//		v := c.value(f)
//		if c.typ.cat == ValueT {
//			if v == nil {
//				in[i] = reflect.New(c.typ.rtype).Elem()
//			} else {
//				if w, ok := v.(reflect.Value); ok {
//					in[i] = w
//				} else {
//					in[i] = reflect.ValueOf(v)
//				}
//			}
//			c.frame = f
//		} else {
//			if v == nil {
//				in[i] = reflect.ValueOf(c.typ.zero())
//			} else {
//				if w, ok := v.(reflect.Value); ok {
//					in[i] = w
//				} else {
//					in[i] = reflect.ValueOf(v)
//				}
//			}
//		}
//	}
//	fun := n.child[0].value(f).(reflect.Value)
//	//log.Println(n.index, "in callBin", in)
//	v := fun.Call(in)
//	for i := 0; i < n.fsize; i++ {
//		f.data[n.findex+i] = v[i].Interface()
//	}
//}
func callBin(n *Node) Builtin {
	return func(f *Frame) {
		in := make([]reflect.Value, len(n.child)-1)
		for i, c := range n.child[1:] {
			v := c.value(f)
			if c.typ.cat == ValueT {
				if v == nil {
					in[i] = reflect.New(c.typ.rtype).Elem()
				} else {
					if w, ok := v.(reflect.Value); ok {
						in[i] = w
					} else {
						in[i] = reflect.ValueOf(v)
					}
				}
				c.frame = f
			} else {
				if v == nil {
					in[i] = reflect.ValueOf(c.typ.zero())
				} else {
					if w, ok := v.(reflect.Value); ok {
						in[i] = w
					} else {
						in[i] = reflect.ValueOf(v)
					}
				}
			}
		}
		fun := n.child[0].value(f).(reflect.Value)
		//log.Println(n.index, "in callBin", in)
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i].Interface()
		}
	}
}

// Call a method defined by an interface type on an object returned by a bin import, through reflect.
// In that case, the method func value can be resolved only at execution from the actual value
// of node, not during CFG.
func callBinInterfaceMethod(n *Node, f *Frame) {
}

// Call a method on an object returned by a bin import function, through reflect
//func callBinMethod(n *Node, f *Frame) {
//	fun := n.child[0].rval
//	in := make([]reflect.Value, len(n.child))
//	val := n.child[0].child[0].value(f)
//	switch val.(type) {
//	case reflect.Value:
//		in[0] = val.(reflect.Value)
//	default:
//		in[0] = reflect.ValueOf(val)
//	}
//	for i, c := range n.child[1:] {
//		if c.kind == Rvalue {
//			in[i+1] = c.value(f).(reflect.Value)
//			c.frame = f
//		} else {
//			in[i+1] = reflect.ValueOf(c.value(f))
//		}
//	}
//	//log.Println(n.index, "in callBinMethod", n.ident, in)
//	if !fun.IsValid() {
//		fun = in[0].MethodByName(n.child[0].child[1].ident)
//		in = in[1:]
//	}
//	v := fun.Call(in)
//	for i := 0; i < n.fsize; i++ {
//		f.data[n.findex+i] = v[i].Interface()
//	}
//}
func callBinMethod(n *Node) Builtin {
	return func(f *Frame) {
		fun := n.child[0].rval
		in := make([]reflect.Value, len(n.child))
		val := n.child[0].child[0].value(f)
		switch val.(type) {
		case reflect.Value:
			in[0] = val.(reflect.Value)
		default:
			in[0] = reflect.ValueOf(val)
		}
		for i, c := range n.child[1:] {
			if c.kind == Rvalue {
				in[i+1] = c.value(f).(reflect.Value)
				c.frame = f
			} else {
				in[i+1] = reflect.ValueOf(c.value(f))
			}
		}
		//log.Println(n.index, "in callBinMethod", n.ident, in)
		if !fun.IsValid() {
			fun = in[0].MethodByName(n.child[0].child[1].ident)
			in = in[1:]
		}
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i].Interface()
		}
	}
}

// Same as callBinMethod, but for handling f(g()) where g returns multiple values
//func callBinMethodX(n *Node, f *Frame) {
//	fun := n.child[0].value(f).(reflect.Value)
//	l := n.child[1].fsize
//	b := n.child[1].findex
//	in := make([]reflect.Value, l+1)
//	in[0] = reflect.ValueOf(n.child[0].child[0].value(f))
//	for i := 0; i < l; i++ {
//		in[i+1] = reflect.ValueOf(f.data[b+i])
//	}
//	v := fun.Call(in)
//	for i := 0; i < n.fsize; i++ {
//		f.data[n.findex+i] = v[i].Interface()
//	}
//}
func callBinMethodX(n *Node) Builtin {
	return func(f *Frame) {
		fun := n.child[0].value(f).(reflect.Value)
		l := n.child[1].fsize
		b := n.child[1].findex
		in := make([]reflect.Value, l+1)
		in[0] = reflect.ValueOf(n.child[0].child[0].value(f))
		for i := 0; i < l; i++ {
			in[i+1] = reflect.ValueOf(f.data[b+i])
		}
		v := fun.Call(in)
		for i := 0; i < n.fsize; i++ {
			f.data[n.findex+i] = v[i].Interface()
		}
	}
}

//func getPtrIndexAddr(n *Node, f *Frame) {
//	a := (*n.child[0].value(f).(*interface{})).([]interface{})
//	f.data[n.findex] = &a[n.child[1].value(f).(int)]
//}
func getPtrIndexAddr(n *Node) Builtin {
	return func(f *Frame) {
		a := (*n.child[0].value(f).(*interface{})).([]interface{})
		f.data[n.findex] = &a[n.child[1].value(f).(int)]
	}
}

//func getIndexAddr(n *Node, f *Frame) {
//	a := n.child[0].value(f).([]interface{})
//	f.data[n.findex] = &a[n.child[1].value(f).(int)]
//}
func getIndexAddr(n *Node) Builtin {
	return func(f *Frame) {
		a := n.child[0].value(f).([]interface{})
		f.data[n.findex] = &a[n.child[1].value(f).(int)]
	}
}

//func getPtrIndex(n *Node, f *Frame) {
//	// if error, fallback to getIndex, to make receiver methods work both with pointers and objects
//	if a, ok := n.child[0].value(f).(*interface{}); ok {
//		f.data[n.findex] = (*a).([]interface{})[n.child[1].value(f).(int)]
//	} else {
//		getIndex(n, f)
//	}
//}
func getPtrIndex(n *Node) Builtin {
	return func(f *Frame) {
		if a, ok := n.child[0].value(f).(*interface{}); ok {
			f.data[n.findex] = (*a).([]interface{})[n.child[1].value(f).(int)]
		} else {
			a := n.child[0].value(f).([]interface{})
			f.data[n.findex] = a[n.child[1].value(f).(int)]
		}
	}
}

//func getPtrIndexBin(n *Node, f *Frame) {
//	a := reflect.ValueOf(n.child[0].value(f)).Elem()
//	f.data[n.findex] = a.FieldByIndex(n.val.([]int)).Interface()
//}
func getPtrIndexBin(n *Node) Builtin {
	return func(f *Frame) {
		a := reflect.ValueOf(n.child[0].value(f)).Elem()
		f.data[n.findex] = a.FieldByIndex(n.val.([]int)).Interface()
	}
}

//func getIndexBinMethod(n *Node, f *Frame) {
//	a := reflect.ValueOf(n.child[0].value(f))
//	f.data[n.findex] = a.MethodByName(n.child[1].ident)
//}
func getIndexBinMethod(n *Node) Builtin {
	return func(f *Frame) {
		a := reflect.ValueOf(n.child[0].value(f))
		f.data[n.findex] = a.MethodByName(n.child[1].ident)
	}
}

//func getIndexBin(n *Node, f *Frame) {
//	a := reflect.ValueOf(n.child[0].value(f))
//	f.data[n.findex] = a.FieldByIndex(n.val.([]int))
//}
func getIndexBin(n *Node) Builtin {
	return func(f *Frame) {
		a := reflect.ValueOf(n.child[0].value(f))
		f.data[n.findex] = a.FieldByIndex(n.val.([]int))
	}
}

//func getIndex(n *Node, f *Frame) {
//	a := n.child[0].value(f).([]interface{})
//	f.data[n.findex] = a[n.child[1].value(f).(int)]
//}
func getIndex(n *Node) Builtin {
	return func(f *Frame) {
		a := n.child[0].value(f).([]interface{})
		f.data[n.findex] = a[n.child[1].value(f).(int)]
	}
}

//func getIndexMap(n *Node, f *Frame) {
//	m := n.child[0].value(f).(map[interface{}]interface{})
//	if f.data[n.findex], f.data[n.findex+1] = m[n.child[1].value(f)]; !f.data[n.findex+1].(bool) {
//		// Force a zero value if key is not present in map
//		f.data[n.findex] = n.child[0].typ.val.zero()
//	}
//}
func getIndexMap(n *Node) Builtin {
	return func(f *Frame) {
		m := n.child[0].value(f).(map[interface{}]interface{})
		if f.data[n.findex], f.data[n.findex+1] = m[n.child[1].value(f)]; !f.data[n.findex+1].(bool) {
			// Force a zero value if key is not present in map
			f.data[n.findex] = n.child[0].typ.val.zero()
		}
	}
}

//func getFunc(n *Node, f *Frame) {
//	node := *n
//	node.val = &node
//	frame := *f
//	node.frame = &frame
//	f.data[n.findex] = &node
//	n.frame = &frame
//}
func getFunc(n *Node) Builtin {
	return func(f *Frame) {
		node := *n
		node.val = &node
		frame := *f
		node.frame = &frame
		f.data[n.findex] = &node
		n.frame = &frame
	}
}

//func getMap(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(map[interface{}]interface{})
//}
func getMap(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f).(map[interface{}]interface{}) }
}

//func getPtrIndexSeq(n *Node, f *Frame) {
//	a := (*n.child[0].value(f).(*interface{})).([]interface{})
//	seq := n.child[1].value(f).([]int)
//	l := len(seq) - 1
//	for _, i := range seq[:l] {
//		a = a[i].([]interface{})
//	}
//	f.data[n.findex] = a[seq[l]]
//}
func getPtrIndexSeq(n *Node) Builtin {
	return func(f *Frame) {
		a := (*n.child[0].value(f).(*interface{})).([]interface{})
		seq := n.child[1].value(f).([]int)
		l := len(seq) - 1
		for _, i := range seq[:l] {
			a = a[i].([]interface{})
		}
		f.data[n.findex] = a[seq[l]]
	}
}

//func getIndexSeq(n *Node, f *Frame) {
//	a := n.child[0].value(f).([]interface{})
//	seq := n.child[1].value(f).([]int)
//	l := len(seq) - 1
//	for _, i := range seq[:l] {
//		a = a[i].([]interface{})
//	}
//	f.data[n.findex] = a[seq[l]]
//}
func getIndexSeq(n *Node) Builtin {
	return func(f *Frame) {
		a := n.child[0].value(f).([]interface{})
		seq := n.child[1].value(f).([]int)
		l := len(seq) - 1
		for _, i := range seq[:l] {
			a = a[i].([]interface{})
		}
		f.data[n.findex] = a[seq[l]]
	}
}

func valueSeq(n *Node, seq []int, f *Frame) interface{} {
	a := f.data[n.findex].([]interface{})
	l := len(seq) - 1
	for _, i := range seq[:l] {
		a = a[i].([]interface{})
	}
	return a[seq[l]]
}

//func mul(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(int) * n.child[1].value(f).(int)
//}
func mul(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f).(int) * n.child[1].value(f).(int) }
}

//func quotient(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(int) / n.child[1].value(f).(int)
//}
func quotient(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f).(int) / n.child[1].value(f).(int) }
}

//func remain(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(int) % n.child[1].value(f).(int)
//}
func remain(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f).(int) % n.child[1].value(f).(int) }
}

//func negate(n *Node, f *Frame) {
//	f.data[n.findex] = -n.child[0].value(f).(int)
//}
func negate(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = -n.child[0].value(f).(int) }
}

//func add(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(int) + n.child[1].value(f).(int)
//}
func add(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f).(int) + n.child[1].value(f).(int) }
}

//func sub(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(int) - n.child[1].value(f).(int)
//}
func sub(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f).(int) - n.child[1].value(f).(int) }
}

//func equal(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f) == n.child[1].value(f)
//}
func equal(n *Node) Builtin {
	value0 := n.child[0].value
	value1 := n.child[1].value
	i := n.findex
	return func(f *Frame) { f.data[i] = value0(f) == value1(f) }
}

//func notEqual(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f) != n.child[1].value(f)
//}
func notEqual(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f) != n.child[1].value(f) }
}

//func indirectInc(n *Node, f *Frame) {
//	*(f.data[n.findex].(*interface{})) = n.child[0].value(f).(int) + 1
//}
func indirectInc(n *Node) Builtin {
	return func(f *Frame) { *(f.data[n.findex].(*interface{})) = n.child[0].value(f).(int) + 1 }
}

//func inc(n *Node, f *Frame) {
//	*n.pvalue(f) = n.child[0].value(f).(int) + 1
//}
func inc(n *Node) Builtin {
	pvalue := n.pvalue
	value := n.child[0].value
	return func(f *Frame) { *pvalue(f) = value(f).(int) + 1 }
}

//func greater(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(int) > n.child[1].value(f).(int)
//}
func greater(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.child[0].value(f).(int) > n.child[1].value(f).(int) }
}

//func land(n *Node, f *Frame) {
//	if v := n.child[0].value(f).(bool); v {
//		f.data[n.findex] = n.child[1].value(f).(bool)
//	} else {
//		f.data[n.findex] = v
//	}
//}
func land(n *Node) Builtin {
	return func(f *Frame) {
		if v := n.child[0].value(f).(bool); v {
			f.data[n.findex] = n.child[1].value(f).(bool)
		} else {
			f.data[n.findex] = v
		}
	}
}

//func lor(n *Node, f *Frame) {
//	if v := n.child[0].value(f).(bool); v {
//		f.data[n.findex] = v
//	} else {
//		f.data[n.findex] = n.child[1].value(f).(bool)
//	}
//}
func lor(n *Node) Builtin {
	return func(f *Frame) {
		if v := n.child[0].value(f).(bool); v {
			f.data[n.findex] = v
		} else {
			f.data[n.findex] = n.child[1].value(f).(bool)
		}
	}
}

//func lower(n *Node, f *Frame) {
//	f.data[n.findex] = n.child[0].value(f).(int) < n.child[1].value(f).(int)
//}
func lower(n *Node) Builtin {
	value0 := n.child[0].value
	value1 := n.child[1].value
	i := n.findex
	return func(f *Frame) { f.data[i] = value0(f).(int) < value1(f).(int) }
}

//func nop(n *Node, f *Frame) {}
func _nop(f *Frame) {}

func nop(n *Node) Builtin {
	return _nop
}

//func _return(n *Node, f *Frame) {
//	for i, c := range n.child {
//		f.data[i] = c.value(f)
//	}
//}
func _return(n *Node) Builtin {
	return func(f *Frame) {
		for i, c := range n.child {
			f.data[i] = c.value(f)
		}
	}
}

// create an array of litteral values
//func arrayLit(n *Node, f *Frame) {
//	a := make([]interface{}, len(n.child)-1)
//	for i, c := range n.child[1:] {
//		a[i] = c.value(f)
//	}
//	f.data[n.findex] = a
//}
func arrayLit(n *Node) Builtin {
	return func(f *Frame) {
		a := make([]interface{}, len(n.child)-1)
		for i, c := range n.child[1:] {
			a[i] = c.value(f)
		}
		f.data[n.findex] = a
	}
}

// Create a map of litteral values
//func mapLit(n *Node, f *Frame) {
//	m := make(map[interface{}]interface{})
//	for _, c := range n.child[1:] {
//		m[c.child[0].value(f)] = c.child[1].value(f)
//	}
//	f.data[n.findex] = m
//}
func mapLit(n *Node) Builtin {
	return func(f *Frame) {
		m := make(map[interface{}]interface{})
		for _, c := range n.child[1:] {
			m[c.child[0].value(f)] = c.child[1].value(f)
		}
		f.data[n.findex] = m
	}
}

// Create a struct object
//func compositeLit(n *Node, f *Frame) {
//	l := len(n.typ.field)
//	a := n.typ.zero().([]interface{})
//	for i := 0; i < l; i++ {
//		if i < len(n.child[1:]) {
//			c := n.child[i+1]
//			a[i] = c.value(f)
//		} else {
//			a[i] = n.typ.field[i].typ.zero()
//		}
//	}
//	f.data[n.findex] = a
//}
func compositeLit(n *Node) Builtin {
	return func(f *Frame) {
		l := len(n.typ.field)
		a := n.typ.zero().([]interface{})
		for i := 0; i < l; i++ {
			if i < len(n.child[1:]) {
				c := n.child[i+1]
				a[i] = c.value(f)
			} else {
				a[i] = n.typ.field[i].typ.zero()
			}
		}
		f.data[n.findex] = a
	}
}

// Create a struct Object, filling fields from sparse key-values
//func compositeSparse(n *Node, f *Frame) {
//	a := n.typ.zero().([]interface{})
//	for _, c := range n.child[1:] {
//		// index from key was pre-computed during CFG
//		a[c.findex] = c.child[1].value(f)
//	}
//	f.data[n.findex] = a
//}
func compositeSparse(n *Node) Builtin {
	return func(f *Frame) {
		a := n.typ.zero().([]interface{})
		for _, c := range n.child[1:] {
			// index from key was pre-computed during CFG
			a[c.findex] = c.child[1].value(f)
		}
		f.data[n.findex] = a
	}
}

//func _range(n *Node, f *Frame) {
//	i := 0
//	index := n.child[0].findex
//	if f.data[index] != nil {
//		i = f.data[index].(int) + 1
//	}
//	a := n.child[2].value(f).([]interface{})
//	if i >= len(a) {
//		f.data[n.findex] = false
//		return
//	}
//	f.data[index] = i
//	f.data[n.child[1].findex] = a[i]
//	f.data[n.findex] = true
//}
func _range(n *Node) Builtin {
	return func(f *Frame) {
		i := 0
		index := n.child[0].findex
		if f.data[index] != nil {
			i = f.data[index].(int) + 1
		}
		a := n.child[2].value(f).([]interface{})
		if i >= len(a) {
			f.data[n.findex] = false
			return
		}
		f.data[index] = i
		f.data[n.child[1].findex] = a[i]
		f.data[n.findex] = true
	}
}

//func _case(n *Node, f *Frame) {
//	f.data[n.findex] = n.anc.anc.child[0].value(f) == n.child[0].value(f)
//}
func _case(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = n.anc.anc.child[0].value(f) == n.child[0].value(f) }
}

// TODO: handle variable number of arguments to append
//func _append(n *Node, f *Frame) {
//	a := n.child[1].value(f).([]interface{})
//	f.data[n.findex] = append(a, n.child[2].value(f))
//}
func _append(n *Node) Builtin {
	return func(f *Frame) {
		a := n.child[1].value(f).([]interface{})
		f.data[n.findex] = append(a, n.child[2].value(f))
	}
}

//func _len(n *Node, f *Frame) {
//	a := n.child[1].value(f).([]interface{})
//	f.data[n.findex] = len(a)
//}
func _len(n *Node) Builtin {
	return func(f *Frame) {
		a := n.child[1].value(f).([]interface{})
		f.data[n.findex] = len(a)
	}
}

// Allocates and initializes a slice, a map or a chan.
//func _make(n *Node, f *Frame) {
//	typ := n.child[1].value(f).(*Type)
//	switch typ.cat {
//	case ArrayT:
//		f.data[n.findex] = make([]interface{}, n.child[2].value(f).(int))
//	case ChanT:
//		f.data[n.findex] = make(chan interface{})
//	case MapT:
//		f.data[n.findex] = make(map[interface{}]interface{})
//	}
//}
func _make(n *Node) Builtin {
	return func(f *Frame) {
		typ := n.child[1].value(f).(*Type)
		switch typ.cat {
		case ArrayT:
			f.data[n.findex] = make([]interface{}, n.child[2].value(f).(int))
		case ChanT:
			f.data[n.findex] = make(chan interface{})
		case MapT:
			f.data[n.findex] = make(map[interface{}]interface{})
		}
	}
}

// Read from a channel
//func recv(n *Node, f *Frame) {
//	f.data[n.findex] = <-n.child[0].value(f).(chan interface{})
//}
func recv(n *Node) Builtin {
	return func(f *Frame) { f.data[n.findex] = <-n.child[0].value(f).(chan interface{}) }
}

// Write to a channel
//func send(n *Node, f *Frame) {
//	n.child[0].value(f).(chan interface{}) <- n.child[1].value(f)
//}
func send(n *Node) Builtin {
	return func(f *Frame) { n.child[0].value(f).(chan interface{}) <- n.child[1].value(f) }
}

// slice expression
//func slice(n *Node, f *Frame) {
//	a := n.child[0].value(f).([]interface{})
//	switch len(n.child) {
//	case 2:
//		f.data[n.findex] = a[n.child[1].value(f).(int):]
//	case 3:
//		f.data[n.findex] = a[n.child[1].value(f).(int):n.child[2].value(f).(int)]
//	case 4:
//		f.data[n.findex] = a[n.child[1].value(f).(int):n.child[2].value(f).(int):n.child[3].value(f).(int)]
//	}
//}
func slice(n *Node) Builtin {
	return func(f *Frame) {
		a := n.child[0].value(f).([]interface{})
		switch len(n.child) {
		case 2:
			f.data[n.findex] = a[n.child[1].value(f).(int):]
		case 3:
			f.data[n.findex] = a[n.child[1].value(f).(int):n.child[2].value(f).(int)]
		case 4:
			f.data[n.findex] = a[n.child[1].value(f).(int):n.child[2].value(f).(int):n.child[3].value(f).(int)]
		}
	}
}

// slice expression, no low value
//func slice0(n *Node, f *Frame) {
//	a := n.child[0].value(f).([]interface{})
//	switch len(n.child) {
//	case 1:
//		f.data[n.findex] = a[:]
//	case 2:
//		f.data[n.findex] = a[0:n.child[1].value(f).(int)]
//	case 3:
//		f.data[n.findex] = a[0:n.child[1].value(f).(int):n.child[2].value(f).(int)]
//	}
//}
func slice0(n *Node) Builtin {
	return func(f *Frame) {
		a := n.child[0].value(f).([]interface{})
		switch len(n.child) {
		case 1:
			f.data[n.findex] = a[:]
		case 2:
			f.data[n.findex] = a[0:n.child[1].value(f).(int)]
		case 3:
			f.data[n.findex] = a[0:n.child[1].value(f).(int):n.child[2].value(f).(int)]
		}
	}
}

// Temporary, for debugging purppose
//func sleep(n *Node, f *Frame) {
//	duration := time.Duration(n.child[1].value(f).(int))
//	time.Sleep(duration * time.Millisecond)
//}
func sleep(n *Node) Builtin {
	return func(f *Frame) {
		duration := time.Duration(n.child[1].value(f).(int))
		time.Sleep(duration * time.Millisecond)
	}
}

//func isNil(n *Node, f *Frame) {
//	if n.child[0].kind == Rvalue {
//		f.data[n.findex] = n.child[0].value(f).(reflect.Value).IsNil()
//	} else {
//		f.data[n.findex] = n.child[0].value(f) == nil
//	}
//}
func isNil(n *Node) Builtin {
	return func(f *Frame) {
		if n.child[0].kind == Rvalue {
			f.data[n.findex] = n.child[0].value(f).(reflect.Value).IsNil()
		} else {
			f.data[n.findex] = n.child[0].value(f) == nil
		}
	}
}

//func isNotNil(n *Node, f *Frame) {
//	if n.child[0].kind == Rvalue {
//		f.data[n.findex] = !n.child[0].value(f).(reflect.Value).IsNil()
//	} else {
//		f.data[n.findex] = n.child[0].value(f) != nil
//	}
//}
func isNotNil(n *Node) Builtin {
	return func(f *Frame) {
		if n.child[0].kind == Rvalue {
			f.data[n.findex] = !n.child[0].value(f).(reflect.Value).IsNil()
		} else {
			f.data[n.findex] = n.child[0].value(f) != nil
		}
	}
}
