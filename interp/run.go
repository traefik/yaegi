package interp

//go:generate go run ../cmd/genop/genop.go

import (
	"fmt"
	"reflect"
)

// Builtin type defines functions which run at CFG execution
type Builtin func(f *Frame) Builtin

// BuiltinGenerator type defines a builtin generator function
type BuiltinGenerator func(n *Node)

var builtin = [...]BuiltinGenerator{
	Nop:          nop,
	Addr:         addr,
	Assign:       assign,
	AssignX:      assignX,
	Add:          add,
	AddAssign:    addAssign,
	And:          and,
	AndAssign:    andAssign,
	AndNot:       andnot,
	AndNotAssign: andnotAssign,
	Call:         call,
	Case:         _case,
	CompositeLit: arrayLit,
	Dec:          dec,
	Defer:        _defer,
	Equal:        equal,
	GetFunc:      getFunc,
	Greater:      greater,
	GreaterEqual: greaterEqual,
	Inc:          inc,
	Land:         land,
	Lor:          lor,
	Lower:        lower,
	LowerEqual:   lowerEqual,
	Mul:          mul,
	MulAssign:    mulAssign,
	Negate:       negate,
	Not:          not,
	NotEqual:     notEqual,
	Or:           or,
	OrAssign:     orAssign,
	Quo:          quo,
	QuoAssign:    quoAssign,
	Range:        _range,
	Recv:         recv,
	Rem:          rem,
	RemAssign:    remAssign,
	Return:       _return,
	Send:         send,
	Shl:          shl,
	ShlAssign:    shlAssign,
	Shr:          shr,
	ShrAssign:    shrAssign,
	Slice:        slice,
	Slice0:       slice0,
	Star:         deref,
	Sub:          sub,
	SubAssign:    subAssign,
	TypeAssert:   typeAssert,
	Xor:          xor,
	XorAssign:    xorAssign,
}

type valueInterface struct {
	node  *Node
	value reflect.Value
}

func (interp *Interpreter) run(n *Node, cf *Frame) {
	var f *Frame
	if cf == nil {
		f = interp.Frame
	} else {
		f = &Frame{anc: cf, data: make([]reflect.Value, len(n.types))}
	}

	for i, t := range n.types {
		f.data[i] = reflect.New(t).Elem()
	}
	runCfg(n.start, f)
}

// Functions set to run during execution of CFG

// runCfg executes a node AST by walking its CFG and running node builtin at each step
func runCfg(n *Node, f *Frame) {
	defer func() {
		f.recovered = recover()
		for _, val := range f.deferred {
			val[0].Call(val[1:])
		}
		if f.recovered != nil {
			panic(f.recovered)
		}
	}()

	for exec := n.exec; exec != nil; {
		exec = exec(f)
	}
}

func typeAssert(n *Node) {
	value := genValue(n.child[0])
	i := n.findex
	next := getExec(n.tnext)

	switch {
	case n.child[0].typ.cat == ValueT:
		n.exec = func(f *Frame) Builtin {
			f.data[i].Set(value(f).Elem())
			return next
		}
	case n.child[1].typ.cat == InterfaceT:
		n.exec = func(f *Frame) Builtin {
			v := value(f).Interface().(valueInterface)
			f.data[i] = reflect.ValueOf(valueInterface{v.node, v.value})
			return next
		}
	default:
		n.exec = func(f *Frame) Builtin {
			v := value(f).Interface().(valueInterface)
			f.data[i].Set(v.value)
			return next
		}
	}
}

func typeAssert2(n *Node) {
	value := genValue(n.child[0])      // input value
	value0 := genValue(n.anc.child[0]) // returned result
	value1 := genValue(n.anc.child[1]) // returned status
	next := getExec(n.tnext)

	switch {
	case n.child[0].typ.cat == ValueT:
		n.exec = func(f *Frame) Builtin {
			if value(f).IsValid() && !value(f).IsNil() {
				value0(f).Set(value(f).Elem())
			}
			value1(f).SetBool(true)
			return next
		}
	case n.child[1].typ.cat == InterfaceT:
		n.exec = func(f *Frame) Builtin {
			v, ok := value(f).Interface().(valueInterface)
			value0(f).Set(reflect.ValueOf(valueInterface{v.node, v.value}))
			value1(f).SetBool(ok)
			return next
		}
	default:
		n.exec = func(f *Frame) Builtin {
			v, ok := value(f).Interface().(valueInterface)
			value0(f).Set(v.value)
			value1(f).SetBool(ok)
			return next
		}
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

// assignX implements multiple value assignment
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
			if f.data[b+i].IsValid() {
				value(f).Set(f.data[b+i])
			}
		}
		return next
	}
}

// assignX2 implements multiple value assignment for expression where type is defined
func assignX2(n *Node) {
	l := len(n.child) - 2
	b := n.child[l].findex
	s := n.child[:l]
	next := getExec(n.tnext)
	values := make([]func(*Frame) reflect.Value, l)
	for i, c := range s {
		values[i] = genValue(c)
	}

	n.exec = func(f *Frame) Builtin {
		for i, value := range values {
			if f.data[b+i].IsValid() {
				value(f).Set(f.data[b+i])
			}
		}
		return next
	}
}

// assign implements single value assignment
func assign(n *Node) {
	next := getExec(n.tnext)
	value := genValue(n)
	dest, src := n.child[0], n.lastChild()
	var value1 func(*Frame) reflect.Value

	switch {
	case n.child[0].typ.cat == InterfaceT:
		value1 = genValueInterface(src)
	case dest.typ.cat == ValueT && src.typ.cat == FuncT:
		value1 = genNodeWrapper(src)
	default:
		value1 = genValue(src)
	}

	n.exec = func(f *Frame) Builtin {
		value(f).Set(value1(f))
		return next
	}
}

func assignMap(n *Node) {
	value := genValue(n.child[0].child[0]) // map
	var value0, value1 func(*Frame) reflect.Value

	if n.child[0].child[1].typ.cat == InterfaceT { // key
		value0 = genValueInterface(n.child[0].child[1])
	} else {
		value0 = genValue(n.child[0].child[1])
	}

	if n.child[1].typ.cat == InterfaceT { // value
		value1 = genValueInterface(n.child[1])
	} else {
		value1 = genValue(n.child[1])
	}
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		value(f).SetMapIndex(value0(f), value1(f))
		return next
	}
}

func not(n *Node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if !value(f).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(!value(f).Bool())
			return tnext
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
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value(f).Elem().Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i] = value(f).Elem()
			return tnext
		}
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
		}
		fmt.Println("")
		return next
	}
}

func _recover(n *Node) {
	tnext := getExec(n.tnext)
	i := n.findex
	var err error
	nilErr := reflect.ValueOf(&err).Elem()

	n.exec = func(f *Frame) Builtin {
		if f.anc.recovered == nil {
			f.data[i] = nilErr
		} else {
			f.data[i] = reflect.ValueOf(f.anc.recovered)
			f.anc.recovered = nil
		}
		return tnext
	}
}

func _panic(n *Node) {
	value := genValue(n.child[1])

	n.exec = func(f *Frame) Builtin {
		panic(value(f))
	}
}

func genNodeWrapper(n *Node) func(*Frame) reflect.Value {
	def := n.val.(*Node)
	setExec(def.child[3].start)
	start := def.child[3].start
	numRet := len(def.typ.ret)
	var receiver func(*Frame) reflect.Value

	if n.recv != nil {
		receiver = genValueRecv(n)
	}

	return func(f *Frame) reflect.Value {
		if n.frame != nil { // Use closure context if defined
			f = n.frame
		}
		return reflect.MakeFunc(n.typ.TypeOf(), func(in []reflect.Value) []reflect.Value {
			// Allocate and init local frame. All values to be settable and addressable.
			frame := Frame{anc: f, data: make([]reflect.Value, len(def.types))}
			d := frame.data
			for i, t := range def.types {
				d[i] = reflect.New(t).Elem()
			}

			// Copy method receiver as first argument, if defined
			if receiver != nil {
				d[numRet].Set(receiver(f))
				d = d[numRet+1:]
			} else {
				d = d[numRet:]
			}

			// Copy function input arguments in local frame
			for i, arg := range in {
				d[i].Set(arg)
			}

			// Interpreter code execution
			runCfg(start, &frame)

			result := frame.data[:numRet]
			for i, r := range result {
				if v, ok := r.Interface().(*Node); ok {
					result[i] = genNodeWrapper(v)(f)
				}
			}
			return result
		})
	}
}

func _defer(n *Node) {
	tnext := getExec(n.tnext)
	values := make([]func(*Frame) reflect.Value, len(n.child[0].child))
	var method func(*Frame) reflect.Value

	for i, c := range n.child[0].child {
		if c.typ.cat == FuncT {
			values[i] = genNodeWrapper(c)
		} else {
			if c.recv != nil {
				// defer a method on a binary obj
				mi := c.val.(int)
				m := genValue(c.child[0])
				method = func(f *Frame) reflect.Value { return m(f).Method(mi) }
			}
			values[i] = genValue(c)
		}
	}

	if method != nil {
		n.exec = func(f *Frame) Builtin {
			val := make([]reflect.Value, len(values))
			val[0] = method(f)
			for i, v := range values[1:] {
				val[i+1] = v(f)
			}
			f.deferred = append([][]reflect.Value{val}, f.deferred...)
			return tnext
		}
	} else {
		n.exec = func(f *Frame) Builtin {
			val := make([]reflect.Value, len(values))
			for i, v := range values {
				val[i] = v(f)
			}
			f.deferred = append([][]reflect.Value{val}, f.deferred...)
			return tnext
		}
	}
}

func call(n *Node) {
	goroutine := n.anc.kind == GoStmt
	var method bool
	value := genValue(n.child[0])
	var values []func(*Frame) reflect.Value
	if n.child[0].recv != nil {
		// Compute method receiver value
		values = append(values, genValueRecv(n.child[0]))
		method = true
	} else if n.child[0].action == Method {
		// add a place holder for interface method receiver
		values = append(values, nil)
		method = true
	}
	numRet := len(n.child[0].typ.ret)
	variadic := variadicPos(n)
	child := n.child[1:]
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)

	// compute input argument value functions
	for i, c := range child {
		if isRegularCall(c) {
			// Arguments are return values of a nested function call
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
				convertLiteralValue(c, argType)
			}
			if len(n.child[0].typ.arg) > i && n.child[0].typ.arg[i].cat == InterfaceT {
				values = append(values, genValueInterface(c))
			} else {
				values = append(values, genValue(c))
			}
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
		nf := Frame{anc: anc, data: make([]reflect.Value, len(def.types))}
		var vararg reflect.Value

		// Init local frame values
		for i, t := range def.types {
			nf.data[i] = reflect.New(t).Elem()
		}

		// Init variadic argument vector
		if variadic >= 0 {
			vararg = nf.data[numRet+variadic]
		}

		// Copy input parameters from caller
		dest := nf.data[numRet:]
		for i, v := range values {
			switch {
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
		if fnext != nil {
			if nf.data[0].Bool() {
				return tnext
			}
			return fnext
		}
		// Propagate return values to caller frame
		for i, r := range ret {
			f.data[r] = nf.data[i]
		}
		return tnext
	}
}

// Call a function from a bin import, accessible through reflect
func callBin(n *Node) {
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)
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
				convertLiteralValue(c, argType)
				if !reflect.ValueOf(c.val).IsValid() { //  Handle "nil"
					c.val = reflect.Zero(argType)
				}
			}
			// FIXME: nil types are forbidden and should be handled at compile time (CFG)
			if c.typ != nil {
				switch c.typ.cat {
				case FuncT:
					values = append(values, genNodeWrapper(c))
				case InterfaceT:
					values = append(values, genValueInterfaceValue(c))
				default:
					values = append(values, genValue(c))
				}
			} else {
				values = append(values, genValue(c))
			}
		}
	}
	l := len(values)

	switch {
	case n.anc.kind == GoStmt:
		// Execute function in a goroutine, discard results
		n.exec = func(f *Frame) Builtin {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			go value(f).Call(in)
			return tnext
		}
	case fnext != nil:
		// Handle branching according to boolean result
		n.exec = func(f *Frame) Builtin {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			res := value(f).Call(in)
			if res[0].Bool() {
				return tnext
			}
			return fnext
		}
	default:
		n.exec = func(f *Frame) Builtin {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			copy(f.data[n.findex:], value(f).Call(in))
			return tnext
		}
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

// getIndexArray returns array value from index
func getIndexArray(n *Node) {
	tnext := getExec(n.tnext)
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value0(f).Index(int(value1(f).Int())).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i] = value0(f).Index(int(value1(f).Int()))
			return tnext
		}
	}
}

// getIndexMap retrieves map value from index
func getIndexMap(n *Node) {
	value0 := genValue(n.child[0]) // map
	value1 := genValue(n.child[1]) // index
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value0(f).MapIndex(value1(f)).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i] = value0(f).MapIndex(value1(f))
			return tnext
		}
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
		f.data[i].Set(reflect.ValueOf(&node))
		return next
	}
}

func getMethod(n *Node) {
	i := n.findex
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		frame := *f
		node := *(n.val.(*Node))
		node.val = &node
		node.recv = n.recv
		node.frame = &frame
		f.data[i] = reflect.ValueOf(&node)
		return next
	}
}

func getMethodByName(n *Node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])
	name := n.child[1].ident
	i := n.findex

	n.exec = func(f *Frame) Builtin {
		val := value0(f).Interface().(valueInterface)
		m, li := val.node.typ.lookupMethod(name)
		frame := *f
		node := *m
		node.val = &node
		node.recv = &Receiver{nil, val.value, li}
		node.frame = &frame
		f.data[i] = reflect.ValueOf(&node)
		return next
	}
}

func getIndexSeq(n *Node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value(f).FieldByIndex(index).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i] = value(f).FieldByIndex(index)
			return tnext
		}
	}
}

func getPtrIndexSeq(n *Node) {
	index := n.val.([]int)
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value(f).Elem().FieldByIndex(index).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i] = value(f).Elem().FieldByIndex(index)
			return tnext
		}
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

func land(n *Node) {
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value0(f).Bool() && value1(f).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value0(f).Bool() && value1(f).Bool())
			return tnext
		}
	}
}

func lor(n *Node) {
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value0(f).Bool() || value1(f).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value0(f).Bool() || value1(f).Bool())
			return tnext
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
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}

	values := make([]func(*Frame) reflect.Value, len(child))
	index := make([]int, len(child))
	rtype := n.typ.val.TypeOf()
	var max, prev int

	for i, c := range child {
		if c.kind == KeyValueExpr {
			convertLiteralValue(c.child[1], rtype)
			values[i] = genValue(c.child[1])
			index[i] = c.child[0].val.(int)
		} else {
			convertLiteralValue(c, rtype)
			values[i] = genValue(c)
			index[i] = prev
		}
		prev = index[i] + 1
		if prev > max {
			max = prev
		}
	}

	var a reflect.Value
	if n.typ.size > 0 {
		a, _ = n.typ.zero()
	} else {
		a = reflect.MakeSlice(n.typ.TypeOf(), max, max)
	}

	n.exec = func(f *Frame) Builtin {
		for i, v := range values {
			a.Index(index[i]).Set(v(f))
		}
		value(f).Set(a)
		return next
	}
}

func mapLit(n *Node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}
	typ := n.typ.TypeOf()
	keys := make([]func(*Frame) reflect.Value, len(child))
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c.child[0], n.typ.key.TypeOf())
		convertLiteralValue(c.child[1], n.typ.val.TypeOf())
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

// compositeBin creates and populats a struct object from a binary type
func compositeBin(n *Node) {
	next := getExec(n.tnext)
	value := valueGenerator(n, n.findex)
	typ := n.typ.rtype
	child := n.child[1:]
	values := make([]func(*Frame) reflect.Value, len(child))
	fieldIndex := make([][]int, len(child))
	for i, c := range child {
		if c.kind == KeyValueExpr {
			if sf, ok := typ.FieldByName(c.child[0].ident); ok {
				fieldIndex[i] = sf.Index
				convertLiteralValue(c.child[1], sf.Type)
				if c.child[1].typ.cat == FuncT {
					values[i] = genNodeWrapper(c.child[1])
				} else {
					values[i] = genValue(c.child[1])
				}
			}
		} else {
			fieldIndex[i] = []int{i}
			convertLiteralValue(c.child[1], typ.Field(i).Type)
			if c.typ.cat == FuncT {
				values[i] = genNodeWrapper(c.child[1])
			} else {
				values[i] = genValue(c)
			}
		}
	}

	n.exec = func(f *Frame) Builtin {
		s := reflect.New(typ).Elem()
		for i, v := range values {
			s.FieldByIndex(fieldIndex[i]).Set(v(f))
		}
		value(f).Set(s)
		return next
	}
}

// compositeLit creates and populates a struct object
func compositeLit(n *Node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}

	a, _ := n.typ.zero()
	values := make([]func(*Frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c, n.typ.field[i].typ.TypeOf())
		if c.typ.cat == FuncT {
			values[i] = genNodeWrapper(c)
		} else {
			values[i] = genValue(c)
		}
	}

	n.exec = func(f *Frame) Builtin {
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
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}

	values := make(map[int]func(*Frame) reflect.Value)
	a, _ := n.typ.zero()
	for _, c := range child {
		field := n.typ.fieldIndex(c.child[0].ident)
		convertLiteralValue(c.child[1], n.typ.field[field].typ.TypeOf())
		values[field] = genValue(c.child[1])
	}

	n.exec = func(f *Frame) Builtin {
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

func rangeChan(n *Node) {
	i := n.child[0].findex        // element index location in frame
	value := genValue(n.child[1]) // chan
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		v, ok := value(f).Recv()
		if !ok {
			return fnext
		}
		f.data[i].Set(v)
		return tnext
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
	tnext := getExec(n.tnext)

	switch {
	case n.anc.anc.kind == TypeSwitch:
		fnext := getExec(n.fnext)
		sn := n.anc.anc // switch node
		types := make([]*Type, len(n.child)-1)
		for i := range types {
			types[i] = n.child[i].typ
		}
		srcValue := genValue(sn.child[1].lastChild().child[0])
		if len(sn.child[1].child) == 2 {
			// assign in switch guard
			destValue := genValue(n.lastChild().child[0])
			switch len(types) {
			case 0:
				// default clause: assign var to interface value
				n.exec = func(f *Frame) Builtin {
					destValue(f).Set(srcValue(f))
					return tnext
				}
			case 1:
				// match against 1 type: assign var to concrete value
				typ := types[0]
				n.exec = func(f *Frame) Builtin {
					v := srcValue(f)
					if !v.IsValid() {
						// match zero value against nil
						if typ.cat == NilT {
							return tnext
						}
						return fnext
					}
					vi := v.Interface().(valueInterface)
					if vi.node == nil {
						if typ.cat == NilT {
							return tnext
						}
						return fnext
					}
					if vi.node.typ.id() == typ.id() {
						destValue(f).Set(vi.value)
						return tnext
					}
					return fnext
				}
			default:
				// match against multiple types: assign var to interface value
				n.exec = func(f *Frame) Builtin {
					val := srcValue(f)
					vid := val.Interface().(valueInterface).node.typ.id()
					for _, typ := range types {
						if vid == typ.id() {
							destValue(f).Set(val)
							return tnext
						}
					}
					return fnext
				}
			}
		} else {
			// no assign in switch guard
			if len(n.child) <= 1 {
				n.exec = func(f *Frame) Builtin { return tnext }
			} else {
				n.exec = func(f *Frame) Builtin {
					vtyp := srcValue(f).Interface().(valueInterface).node.typ
					for _, typ := range types {
						if vtyp.id() == typ.id() {
							return tnext
						}
					}
					return fnext
				}
			}
		}

	case len(n.child) <= 1: // default clause
		n.exec = func(f *Frame) Builtin { return tnext }

	default:
		fnext := getExec(n.fnext)
		l := len(n.anc.anc.child)
		value := genValue(n.anc.anc.child[l-2])
		values := make([]func(*Frame) reflect.Value, len(n.child)-1)
		for i := range values {
			values[i] = genValue(n.child[i])
		}
		n.exec = func(f *Frame) Builtin {
			for _, v := range values {
				if value(f).Interface() == v(f).Interface() {
					return tnext
				}
			}
			return fnext
		}
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

func _close(n *Node) {
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *Frame) Builtin {
		value(f).Close()
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

	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
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

	case reflect.Chan:
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

	case reflect.Map:
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
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if v, _ := value(f).Recv(); v.Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i], _ = value(f).Recv()
			return tnext
		}
	}
}

func convertLiteralValue(n *Node, t reflect.Type) {
	if n.kind != BasicLit || t == nil || t.Kind() == reflect.Interface {
		return
	}
	n.val = reflect.ValueOf(n.val).Convert(t)
}

// Write to a channel
func send(n *Node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0]) // channel
	convertLiteralValue(n.child[1], n.child[0].typ.val.TypeOf())
	value1 := genValue(n.child[1]) // value to send

	n.exec = func(f *Frame) Builtin {
		value0(f).Send(value1(f))
		return next
	}
}

func clauseChanDir(n *Node) (*Node, *Node, *Node, reflect.SelectDir) {
	dir := reflect.SelectDefault
	var node, assigned, ok *Node
	var stop bool

	n.Walk(func(m *Node) bool {
		switch m.action {
		case Recv:
			dir = reflect.SelectRecv
			node = m.child[0]
			switch m.anc.action {
			case Assign:
				assigned = m.anc.child[0]
			case AssignX:
				ok = m.anc.child[1]
				// TODO
			}
			stop = true
		case Send:
			dir = reflect.SelectSend
			node = m.child[0]
			assigned = m.child[1]
			stop = true
		}
		return !stop
	}, nil)
	return node, assigned, ok, dir
}

func _select(n *Node) {
	nbClause := len(n.child)
	chans := make([]*Node, nbClause)
	assigned := make([]*Node, nbClause)
	ok := make([]*Node, nbClause)
	clause := make([]Builtin, nbClause)
	chanValues := make([]func(*Frame) reflect.Value, nbClause)
	assignedValues := make([]func(*Frame) reflect.Value, nbClause)
	okValues := make([]func(*Frame) reflect.Value, nbClause)
	cases := make([]reflect.SelectCase, nbClause)

	for i := 0; i < nbClause; i++ {
		if len(n.child[i].child) > 1 {
			clause[i] = getExec(n.child[i].child[1].start)
			chans[i], assigned[i], ok[i], cases[i].Dir = clauseChanDir(n.child[i])
			chanValues[i] = genValue(chans[i])
			if assigned[i] != nil {
				assignedValues[i] = genValue(assigned[i])
			}
			if ok[i] != nil {
				okValues[i] = genValue(ok[i])
			}
		} else {
			clause[i] = getExec(n.child[i].child[0].start)
			cases[i].Dir = reflect.SelectDefault
		}
	}

	n.exec = func(f *Frame) Builtin {
		for i := range cases {
			switch cases[i].Dir {
			case reflect.SelectRecv:
				cases[i].Chan = chanValues[i](f)
			case reflect.SelectSend:
				cases[i].Chan = chanValues[i](f)
				cases[i].Send = assignedValues[i](f)
			case reflect.SelectDefault:
				// Keep zero values for comm clause
			}
		}
		j, v, s := reflect.Select(cases)
		if cases[j].Dir == reflect.SelectRecv && assignedValues[j] != nil {
			assignedValues[j](f).Set(v)
			if ok[j] != nil {
				okValues[j](f).SetBool(s)
			}
		}
		return clause[j]
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
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value(f).IsNil() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(value(f).IsNil())
			return tnext
		}
	}
}

func isNotNil(n *Node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *Frame) Builtin {
			if value(f).IsNil() {
				return fnext
			}
			return tnext
		}
	} else {
		i := n.findex
		n.exec = func(f *Frame) Builtin {
			f.data[i].SetBool(!value(f).IsNil())
			return tnext
		}
	}
}
