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

func call(n *Node) Builtin {
	var recv *Node
	var rseq []int
	var forkFrame bool
	var ret []int
	var goroutine bool

	if n.action == CallF {
		forkFrame = true
	}

	if n.anc.kind == GoStmt {
		goroutine = true
	}

	if n.child[0].kind == SelectorExpr && n.child[0].typ.cat != SrcPkgT {
		recv = n.child[0].recv
		rseq = n.child[0].child[1].val.([]int)
	}

	value := n.child[0].value

	return func(f *Frame) {
		fn := value(f).(*Node)
		if len(fn.child[2].child) > 1 {
			if fieldList := fn.child[2].child[1]; fieldList != nil {
				ret = make([]int, len(fieldList.child))
				for i := range fieldList.child {
					ret[i] = n.findex + i
				}
			}
		}
		Run(fn, f, recv, rseq, n.child[1:], ret, forkFrame, goroutine)
	}
}

// Same as callBin, but for handling f(g()) where g returns multiple values
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
func callBinInterfaceMethod(n *Node, f *Frame) {}

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

func getPtrIndexAddr(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) {
		a := (*value0(f).(*interface{})).([]interface{})
		f.data[i] = &a[value1(f).(int)]
	}
}

func getIndexAddr(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) {
		a := value0(f).([]interface{})
		f.data[i] = &a[value1(f).(int)]
	}
}

func getPtrIndex(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) {
		// if error, fallback to getIndex, to make receiver methods work both with pointers and objects
		if a, ok := value0(f).(*interface{}); ok {
			f.data[i] = (*a).([]interface{})[value1(f).(int)]
		} else {
			a := value0(f).([]interface{})
			f.data[i] = a[value1(f).(int)]
		}
	}
}

func getPtrIndexBin(n *Node) Builtin {
	i := n.findex
	fi := n.val.([]int)
	value := n.child[0].value
	return func(f *Frame) {
		a := reflect.ValueOf(value(f)).Elem()
		f.data[i] = a.FieldByIndex(fi).Interface()
	}
}

func getIndexBinMethod(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	ident := n.child[1].ident
	return func(f *Frame) {
		a := reflect.ValueOf(value(f))
		f.data[i] = a.MethodByName(ident)
	}
}

func getIndexBin(n *Node) Builtin {
	i := n.findex
	fi := n.val.([]int)
	value := n.child[0].value
	return func(f *Frame) {
		a := reflect.ValueOf(value(f))
		f.data[i] = a.FieldByIndex(fi)
	}
}

func getIndex(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) {
		a := value0(f).([]interface{})
		f.data[i] = a[value1(f).(int)]
	}
}

func getIndexMap(n *Node) Builtin {
	i := n.findex
	i1 := i + 1
	value0 := n.child[0].value
	value1 := n.child[1].value
	z := n.child[0].typ.val.zero()
	return func(f *Frame) {
		m := value0(f).(map[interface{}]interface{})
		if f.data[i], f.data[i1] = m[value1(f)]; !f.data[i1].(bool) {
			// Force a zero value if key is not present in map
			f.data[i] = z
		}
	}
}

func getFunc(n *Node) Builtin {
	i := n.findex
	return func(f *Frame) {
		node := *n
		node.val = &node
		frame := *f
		node.frame = &frame
		f.data[i] = &node
		n.frame = &frame
	}
}

func getMap(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	return func(f *Frame) { f.data[i] = value(f).(map[interface{}]interface{}) }
}

func getIndexSeq(n *Node) Builtin {
	ind := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) {
		a := value0(f).([]interface{})
		seq := value1(f).([]int)
		l := len(seq) - 1
		for _, i := range seq[:l] {
			a = a[i].([]interface{})
		}
		f.data[ind] = a[seq[l]]
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

func mul(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) * value1(f).(int) }
}

func quotient(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) / value1(f).(int) }
}

func remain(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) % value1(f).(int) }
}

func negate(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	return func(f *Frame) { f.data[i] = -value(f).(int) }
}

func add(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) + value1(f).(int) }
}

func sub(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) - value1(f).(int) }
}

func equal(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f) == value1(f) }
}

func notEqual(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f) != value1(f) }
}

func indirectInc(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	return func(f *Frame) { *(f.data[i].(*interface{})) = value(f).(int) + 1 }
}

func inc(n *Node) Builtin {
	pvalue := n.pvalue
	value := n.child[0].value
	return func(f *Frame) { *pvalue(f) = value(f).(int) + 1 }
}

func greater(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) > value1(f).(int) }
}

func land(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) {
		if v := value0(f).(bool); v {
			f.data[i] = value1(f).(bool)
		} else {
			f.data[i] = v
		}
	}
}

func lor(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) {
		if v := value0(f).(bool); v {
			f.data[i] = v
		} else {
			f.data[i] = value1(f).(bool)
		}
	}
}

func lower(n *Node) Builtin {
	i := n.findex
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { f.data[i] = value0(f).(int) < value1(f).(int) }
}

func _nop(f *Frame) {}

func nop(n *Node) Builtin {
	return _nop
}

// TODO: optimize return according to nb of child
func _return(n *Node) Builtin {
	child := n.child
	return func(f *Frame) {
		for i, c := range child {
			f.data[i] = c.value(f)
		}
	}
}

func arrayLit(n *Node) Builtin {
	ind := n.findex
	l := len(n.child) - 1
	child := n.child[1:]
	return func(f *Frame) {
		a := make([]interface{}, l)
		for i, c := range child {
			a[i] = c.value(f)
		}
		f.data[ind] = a
	}
}

func mapLit(n *Node) Builtin {
	i := n.findex
	return func(f *Frame) {
		m := make(map[interface{}]interface{})
		for _, c := range n.child[1:] {
			m[c.child[0].value(f)] = c.child[1].value(f)
		}
		f.data[i] = m
	}
}

// compositeLit creates a struct object
func compositeLit(n *Node) Builtin {
	ind := n.findex
	l := len(n.typ.field)
	return func(f *Frame) {
		a := n.typ.zero().([]interface{})
		for i := 0; i < l; i++ {
			if i < len(n.child[1:]) {
				c := n.child[i+1]
				a[i] = c.value(f)
			} else {
				a[i] = n.typ.field[i].typ.zero()
			}
		}
		f.data[ind] = a
	}
}

// compositeSparse creates a struct Object, filling fields from sparse key-values
func compositeSparse(n *Node) Builtin {
	i := n.findex
	child := n.child[1:]
	return func(f *Frame) {
		a := n.typ.zero().([]interface{})
		for _, c := range child {
			// index from key was pre-computed during CFG
			a[c.findex] = c.child[1].value(f)
		}
		f.data[i] = a
	}
}

func _range(n *Node) Builtin {
	ind := n.findex
	index0 := n.child[0].findex
	index1 := n.child[1].findex
	value := n.child[2].value
	return func(f *Frame) {
		i := 0
		if f.data[index0] != nil {
			i = f.data[index0].(int) + 1
		}
		a := value(f).([]interface{})
		if i >= len(a) {
			f.data[ind] = false
			return
		}
		f.data[index0] = i
		f.data[index1] = a[i]
		f.data[ind] = true
	}
}

func _case(n *Node) Builtin {
	i := n.findex
	value0 := n.anc.anc.child[0].value
	value1 := n.child[0].value
	return func(f *Frame) { f.data[i] = value0(f) == value1(f) }
}

// TODO: handle variable number of arguments to append
func _append(n *Node) Builtin {
	i := n.findex
	value0 := n.child[1].value
	value1 := n.child[2].value
	return func(f *Frame) { f.data[i] = append(value0(f).([]interface{}), value1(f)) }
}

func _len(n *Node) Builtin {
	i := n.findex
	value := n.child[1].value
	return func(f *Frame) { f.data[i] = len(value(f).([]interface{})) }
}

// _make allocates and initializes a slice, a map or a chan.
func _make(n *Node) Builtin {
	i := n.findex
	value0 := n.child[1].value
	var value1 func(*Frame) interface{}
	if len(n.child) > 2 {
		value1 = n.child[2].value
	}
	return func(f *Frame) {
		typ := value0(f).(*Type)
		switch typ.cat {
		case ArrayT:
			f.data[i] = make([]interface{}, value1(f).(int))
		case ChanT:
			f.data[i] = make(chan interface{})
		case MapT:
			f.data[i] = make(map[interface{}]interface{})
		}
	}
}

// recv reads from a channel
func recv(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	return func(f *Frame) { f.data[i] = <-value(f).(chan interface{}) }
}

// Write to a channel
func send(n *Node) Builtin {
	value0 := n.child[0].value
	value1 := n.child[1].value
	return func(f *Frame) { value0(f).(chan interface{}) <- value1(f) }
}

// slice expression
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
func sleep(n *Node) Builtin {
	return func(f *Frame) {
		duration := time.Duration(n.child[1].value(f).(int))
		time.Sleep(duration * time.Millisecond)
	}
}

func isNil(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	if n.child[0].kind == Rvalue {
		return func(f *Frame) { f.data[i] = value(f).(reflect.Value).IsNil() }
	} else {
		return func(f *Frame) { f.data[i] = value(f) == nil }
	}
}

func isNotNil(n *Node) Builtin {
	i := n.findex
	value := n.child[0].value
	if n.child[0].kind == Rvalue {
		return func(f *Frame) { f.data[i] = !value(f).(reflect.Value).IsNil() }
	} else {
		return func(f *Frame) { f.data[i] = value(f) != nil }
	}
}
