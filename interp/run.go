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
		// FIXME: nil types are forbidden and should be detected at compile time (CFG)
		if t != nil && i < len(f.data) {
			f.data[i] = reflect.New(t).Elem()
		}
	}
	//log.Println(n.index, "run", n.start.index)
	runCfg(n.start, f)
}

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
	i := n.findex
	var value func(*Frame) reflect.Value
	if n.child[1].typ.cat == FuncT {
		value = genNodeWrapper(n.child[1])
	} else {
		value = genValue(n.child[1])
	}
	typ := n.child[0].typ.TypeOf()
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).Convert(typ)
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
			//log.Println(n.index, "in assignX", i, value(f), f.data[b+i], b)
			if f.data[b+i].IsValid() {
				value(f).Set(f.data[b+i])
			}
		}
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

// assign0 implements assignement of zero value, as in a var statement
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
	var result []reflect.Value
	if n.frame == nil {
		n.frame = n.interp.Frame
	}
	log.Println(n.index, "in wrapNode", def.index, n.frame)
	frame := Frame{anc: n.frame, data: make([]reflect.Value, def.flen)}

	// If fucnction is a method, set its receiver data in the frame
	if len(def.child[0].child) > 0 {
		//frame.data[def.child[0].findex] = n.recv.node.value(n.frame)
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

func genNodeWrapper(n *Node) func(*Frame) reflect.Value {
	def := n.val.(*Node)
	setExec(def.child[3].start)
	start := def.child[3].start
	var receiver func(*Frame) reflect.Value

	if n.recv != nil {
		receiver = genValueRecv(n)
	}

	return func(f *Frame) reflect.Value {
		return reflect.MakeFunc(n.typ.TypeOf(), func(in []reflect.Value) []reflect.Value {
			var result []reflect.Value
			frame := Frame{anc: f, data: make([]reflect.Value, def.flen)}
			i := 0

			if receiver != nil {
				frame.data[def.framepos[0]] = receiver(f)
				i++
			}

			// Unwrap input arguments from their reflect value and store them in the frame
			for _, arg := range in {
				frame.data[def.framepos[i]] = arg
				i++
			}

			// Interpreter code execution
			runCfg(start, &frame)

			// Wrap output results in reflect values and return them
			if len(def.child[2].child) > 1 {
				if fieldList := def.child[2].child[1]; fieldList != nil {
					result = make([]reflect.Value, len(fieldList.child))
					for i := range fieldList.child {
						result[i] = frame.data[i]
					}
				}
			}
			return result
		})
	}
}

// FIXME: handle case where func return a boolean
func call(n *Node) {
	goroutine := n.anc.kind == GoStmt
	method := n.child[0].recv != nil
	var values []func(*Frame) reflect.Value
	if method {
		// Compute method receiver value
		values = append(values, genValueRecv(n.child[0]))
	}
	variadic := variadicPos(n)
	next := getExec(n.tnext)
	value := genValue(n.child[0])
	child := n.child[1:]

	// compute input argument value functions
	for i, c := range child {
		if isRegularCall(c) {
			for j := range c.child[0].typ.ret {
				ind := c.findex + j
				values = append(values, func(f *Frame) reflect.Value { return f.data[ind] })
			}
		} else {
			if c.kind == BasicLit {
				var argType reflect.Type
				if variadic >= 0 && i >= variadic {
					argType = n.child[0].typ.arg[variadic].TypeOf()
				} else {
					argType = n.child[0].typ.arg[i].TypeOf()
				}
				if argType != nil && argType.Kind() != reflect.Interface {
					c.val = reflect.ValueOf(c.val).Convert(argType)
				}
			}
			values = append(values, genValue(c))
		}
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
		var vararg reflect.Value

		// Init local frame values
		for i, t := range def.types {
			if t != nil {
				nf.data[i] = reflect.New(t).Elem()
			}
		}

		// Init variadic argument vector
		if variadic >= 0 {
			fi := def.framepos[variadic]
			nf.data[fi] = reflect.New(reflect.SliceOf(def.types[fi])).Elem()
			vararg = nf.data[fi]
		}

		// Copy input parameters from caller
		for i, v := range values {
			src := v(f)
			if method && i == 0 {
				dest := nf.data[def.framepos[i]]
				// Accomodate to receiver type
				ks, kd := src.Kind(), dest.Kind()
				if ks != kd {
					if kd == reflect.Ptr {
						dest.Set(src.Addr())
					} else {
						dest.Set(src.Elem())
					}
				} else {
					dest.Set(src)
				}
			} else if variadic >= 0 && i >= variadic {
				vararg.Set(reflect.Append(vararg, src))
			} else {
				nf.data[def.framepos[i]].Set(src)
			}
		}

		// Execute function body
		if goroutine {
			go runCfg(def.child[3].start, &nf)
		} else {
			runCfg(def.child[3].start, &nf)
			// Propagate return values to caller frame
			//log.Println(n.index, "call rets:", ret, nf.data[:len(ret)])
			for i, r := range ret {
				f.data[r] = nf.data[i]
			}
		}
		return next
	}
}

// FIXME: handle case where func return a boolean
// Call a function from a bin import, accessible through reflect
func callBin(n *Node) {
	next := getExec(n.tnext)
	child := n.child[1:]
	value := genValue(n.child[0])
	var values []func(*Frame) reflect.Value
	funcType := n.child[0].typ.rtype
	variadic := -1
	if funcType.IsVariadic() {
		variadic = funcType.NumIn() - 1
	}
	receiverOffset := 0
	if n.child[0].recv != nil {
		receiverOffset = 1
	}

	for i, c := range child {
		if isRegularCall(c) {
			// Handle nested function calls: pass returned values as arguments
			for j := range c.child[0].typ.ret {
				ind := c.findex + j
				values = append(values, func(f *Frame) reflect.Value { return f.data[ind] })
			}
		} else {
			if c.kind == BasicLit {
				// Convert literal value (untyped) to function argument type (if not an interface{})
				var argType reflect.Type
				if variadic >= 0 && i >= variadic {
					argType = funcType.In(variadic).Elem()
				} else {
					argType = funcType.In(i + receiverOffset)
				}
				if argType != nil && argType.Kind() != reflect.Interface {
					c.val = reflect.ValueOf(c.val).Convert(argType)
				}
				if !reflect.ValueOf(c.val).IsValid() { //  Handle "nil"
					c.val = reflect.Zero(argType)
				}
			}
			// FIXME: nil types are forbidden and should be handled at compile time (CFG)
			if c.typ != nil && c.typ.cat == FuncT {
				values = append(values, genNodeWrapper(c))
			} else {
				values = append(values, genValue(c))
			}
		}
	}
	l := len(values)
	fsize := n.child[0].fsize

	if n.anc.kind == GoStmt {
		// Execute function in a goroutine, discard results
		n.exec = func(f *Frame) Builtin {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			go value(f).Call(in)
			return next
		}
	} else {
		n.exec = func(f *Frame) Builtin {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			//log.Println(n.index, "callbin", value(f).Type(), in)
			v := value(f).Call(in)
			//log.Println(n.index, "callBin, res:", v, fsize, n.findex)
			for i := 0; i < fsize; i++ {
				f.data[n.findex+i] = v[i]
			}
			return next
		}
	}
}

func getPtrIndex(n *Node) {
	i := n.findex
	next := getExec(n.tnext)
	fi := n.child[1].val.(int)
	value := genValue(n.child[0])

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).Elem().Field(fi)
		return next
	}
}

func getIndexBinMethod(n *Node) {
	i := n.findex
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).Method(m)
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
	index := n.val.([]int)
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).FieldByIndex(index)
		return next
	}
}

func getPtrIndexSeq(n *Node) {
	i := n.findex
	fi := n.val.([]int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		f.data[i] = value(f).Elem().FieldByIndex(fi)
		return next
	}
}

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
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child[1:]
	zero := n.typ.zero
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		// FIXME: do automatic type conversion for literal values
		values[i] = genValue(c)
	}

	if n.typ.size > 0 {
		// Fixed size array
		n.exec = func(f *Frame) Builtin {
			a := zero()
			for i, v := range values {
				a.Index(i).Set(v(f))
			}
			value(f).Set(a)
			return next
		}
	} else {
		// Slice
		n.exec = func(f *Frame) Builtin {
			a := zero()
			for _, v := range values {
				a = reflect.Append(a, v(f))
			}
			value(f).Set(a)
			return next
		}
	}
}

func mapLit(n *Node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child[1:]
	typ := n.typ.TypeOf()
	keys := make([]func(*Frame) reflect.Value, len(child))
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		// FIXME: do automatic type conversion for literal values
		keys[i] = genValue(c.child[0])
		values[i] = genValue(c.child[1])
	}

	n.exec = func(f *Frame) Builtin {
		m := reflect.MakeMap(typ)
		for i, k := range keys {
			m.SetMapIndex(k(f), values[i](f))
		}
		value(f).Set(m)
		return next
	}
}

// compositeLit creates a struct object
func compositeLit(n *Node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child[1:]
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		if c.kind == BasicLit {
			// Automatic type conversion for literal values
			fieldType := n.typ.field[i].typ.TypeOf()
			if fieldType != nil && fieldType.Kind() != reflect.Interface {
				c.val = reflect.ValueOf(c.val).Convert(fieldType)
			}
		}
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		a := n.typ.zero()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		value(f).Set(a)
		return next
	}
}

// compositeSparse creates a struct Object, filling fields from sparse key-values
func compositeSparse(n *Node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child[1:]
	values := make(map[int]func(*Frame) reflect.Value)
	for _, c := range child {
		if c.child[1].kind == BasicLit {
			// Automatic type conversion for literal values
			fieldType := n.typ.field[c.findex].typ.TypeOf()
			if fieldType != nil && fieldType.Kind() != reflect.Interface {
				c.child[1].val = reflect.ValueOf(c.child[1].val).Convert(fieldType)
			}
		}
		values[c.findex] = genValue(c.child[1])
	}

	n.exec = func(f *Frame) Builtin {
		a := n.typ.zero()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		value(f).Set(a)
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
