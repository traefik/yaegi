package interp

//go:generate go run ../internal/genop/genop.go

import (
	"fmt"
	"log"
	"reflect"
)

// bltn type defines functions which run at CFG execution
type bltn func(f *frame) bltn

// bltnGenerator type defines a builtin generator function
type bltnGenerator func(n *node)

var builtin = [...]bltnGenerator{
	aNop:          nop,
	aAddr:         addr,
	aAssign:       assign,
	aAdd:          add,
	aAddAssign:    addAssign,
	aAnd:          and,
	aAndAssign:    andAssign,
	aAndNot:       andNot,
	aAndNotAssign: andNotAssign,
	aCall:         call,
	aCase:         _case,
	aCompositeLit: arrayLit,
	aDec:          dec,
	aDefer:        _defer,
	aEqual:        equal,
	aGetFunc:      getFunc,
	aGreater:      greater,
	aGreaterEqual: greaterEqual,
	aInc:          inc,
	aLand:         land,
	aLor:          lor,
	aLower:        lower,
	aLowerEqual:   lowerEqual,
	aMul:          mul,
	aMulAssign:    mulAssign,
	aNegate:       negate,
	aNot:          not,
	aNotEqual:     notEqual,
	aOr:           or,
	aOrAssign:     orAssign,
	aQuo:          quo,
	aQuoAssign:    quoAssign,
	aRange:        _range,
	aRecv:         recv,
	aRem:          rem,
	aRemAssign:    remAssign,
	aReturn:       _return,
	aSend:         send,
	aShl:          shl,
	aShlAssign:    shlAssign,
	aShr:          shr,
	aShrAssign:    shrAssign,
	aSlice:        slice,
	aSlice0:       slice0,
	aStar:         deref,
	aSub:          sub,
	aSubAssign:    subAssign,
	aTypeAssert:   typeAssert,
	aXor:          xor,
	aXorAssign:    xorAssign,
}

type valueInterface struct {
	node  *node
	value reflect.Value
}

var floatType, complexType reflect.Type

func init() {
	floatType = reflect.ValueOf(0.0).Type()
	complexType = reflect.ValueOf(complex(0, 0)).Type()
}

func (interp *Interpreter) run(n *node, cf *frame) {
	var f *frame
	if cf == nil {
		f = interp.frame
	} else {
		f = &frame{anc: cf, data: make([]reflect.Value, len(n.types))}
	}

	for i, t := range n.types {
		f.data[i] = reflect.New(t).Elem()
	}
	runCfg(n.start, f)
}

// Functions set to run during execution of CFG

// runCfg executes a node AST by walking its CFG and running node builtin at each step
func runCfg(n *node, f *frame) {
	defer func() {
		f.recovered = recover()
		for _, val := range f.deferred {
			val[0].Call(val[1:])
		}
		if f.recovered != nil {
			fmt.Println(n.cfgErrorf("panic"))
			panic(f.recovered)
		}
	}()

	for exec := n.exec; exec != nil; {
		exec = exec(f)
	}
}

func typeAssert(n *node) {
	value := genValue(n.child[0])
	i := n.findex
	next := getExec(n.tnext)

	switch {
	case n.child[0].typ.cat == valueT:
		n.exec = func(f *frame) bltn {
			f.data[i].Set(value(f).Elem())
			return next
		}
	case n.child[1].typ.cat == interfaceT:
		n.exec = func(f *frame) bltn {
			v := value(f).Interface().(valueInterface)
			f.data[i] = reflect.ValueOf(valueInterface{v.node, v.value})
			return next
		}
	default:
		n.exec = func(f *frame) bltn {
			v := value(f).Interface().(valueInterface)
			f.data[i].Set(v.value)
			return next
		}
	}
}

func typeAssert2(n *node) {
	value := genValue(n.child[0])      // input value
	value0 := genValue(n.anc.child[0]) // returned result
	value1 := genValue(n.anc.child[1]) // returned status
	next := getExec(n.tnext)

	switch {
	case n.child[0].typ.cat == valueT:
		n.exec = func(f *frame) bltn {
			if value(f).IsValid() && !value(f).IsNil() {
				value0(f).Set(value(f).Elem())
			}
			value1(f).SetBool(true)
			return next
		}
	case n.child[1].typ.cat == interfaceT:
		n.exec = func(f *frame) bltn {
			v, ok := value(f).Interface().(valueInterface)
			value0(f).Set(reflect.ValueOf(valueInterface{v.node, v.value}))
			value1(f).SetBool(ok)
			return next
		}
	default:
		n.exec = func(f *frame) bltn {
			v, ok := value(f).Interface().(valueInterface)
			value0(f).Set(v.value)
			value1(f).SetBool(ok)
			return next
		}
	}
}

func convert(n *node) {
	dest := genValue(n)
	c := n.child[1]
	typ := n.child[0].typ.TypeOf()
	next := getExec(n.tnext)

	if c.kind == basicLit && !c.rval.IsValid() { // convert nil to type
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.New(typ).Elem())
			return next
		}
		return
	}

	var value func(*frame) reflect.Value
	if c.typ.cat == funcT {
		value = genFunctionWrapper(c)
	} else {
		value = genValue(c)
	}

	n.exec = func(f *frame) bltn {
		dest(f).Set(value(f).Convert(typ))
		return next
	}
}

func isRecursiveStruct(t *itype) bool {
	if t.cat == structT && t.rtype.Kind() == reflect.Interface {
		return true
	}
	if t.cat == ptrT {
		return isRecursiveStruct(t.val)
	}
	return false
}

func assign(n *node) {
	next := getExec(n.tnext)
	dvalue := make([]func(*frame) reflect.Value, n.nleft)
	ivalue := make([]func(*frame) reflect.Value, n.nleft)
	svalue := make([]func(*frame) reflect.Value, n.nleft)
	var sbase int
	if n.nright > 0 {
		sbase = len(n.child) - n.nright
	}

	for i := 0; i < n.nleft; i++ {
		dest, src := n.child[i], n.child[sbase+i]
		switch {
		case dest.typ.cat == interfaceT:
			svalue[i] = genValueInterface(src)
		case dest.typ.cat == valueT && dest.typ.rtype.Kind() == reflect.Interface:
			svalue[i] = genInterfaceWrapper(src, dest.typ.rtype)
		case dest.typ.cat == valueT && src.typ.cat == funcT:
			svalue[i] = genFunctionWrapper(src)
		case src.kind == basicLit && src.val == nil:
			t := dest.typ.TypeOf()
			svalue[i] = func(*frame) reflect.Value { return reflect.New(t).Elem() }
		case isRecursiveStruct(dest.typ):
			svalue[i] = genValueInterfacePtr(src)
		default:
			svalue[i] = genValue(src)
		}
		if isMapEntry(dest) {
			if dest.child[1].typ.cat == interfaceT { // key
				ivalue[i] = genValueInterface(dest.child[1])
			} else {
				ivalue[i] = genValue(dest.child[1])
			}
			dvalue[i] = genValue(dest.child[0])
		} else {
			dvalue[i] = genValue(dest)
		}
	}

	if n.nleft == 1 {
		if s, d, i := svalue[0], dvalue[0], ivalue[0]; i != nil {
			n.exec = func(f *frame) bltn {
				d(f).SetMapIndex(i(f), s(f))
				return next
			}
		} else {
			n.exec = func(f *frame) bltn {
				d(f).Set(s(f))
				return next
			}
		}
	} else {
		types := make([]reflect.Type, n.nright)
		for i := range types {
			var t reflect.Type
			switch typ := n.child[sbase+i].typ; typ.cat {
			case funcT:
				t = reflect.TypeOf((*node)(nil))
			case interfaceT:
				t = reflect.TypeOf((*valueInterface)(nil)).Elem()
			default:
				t = typ.TypeOf()
			}
			//types[i] = n.child[sbase+i].typ.TypeOf()
			types[i] = t
		}

		// To handle swap in multi-assign:
		// evaluate and copy all values in assign right hand side into temporary
		// then evaluate assign left hand side and copy temporary into it
		n.exec = func(f *frame) bltn {
			t := make([]reflect.Value, len(svalue))
			for i, s := range svalue {
				t[i] = reflect.New(types[i]).Elem()
				t[i].Set(s(f))
			}
			for i, d := range dvalue {
				if j := ivalue[i]; j != nil {
					d(f).SetMapIndex(j(f), t[i]) // Assign a map entry
				} else {
					d(f).Set(t[i]) // Assign a var or array/slice entry
				}
			}
			return next
		}
	}
}

func not(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if !value(f).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *frame) bltn {
			f.data[i].SetBool(!value(f).Bool())
			return tnext
		}
	}
}

func addr(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).Set(value(f).Addr())
		return next
	}
}

func deref(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value(f).Elem().Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *frame) bltn {
			f.data[i] = value(f).Elem()
			return tnext
		}
	}
}

func _print(n *node) {
	child := n.child[1:]
	next := getExec(n.tnext)
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	n.exec = func(f *frame) bltn {
		for i, value := range values {
			if i > 0 {
				fmt.Printf(" ")
			}
			fmt.Printf("%v", value(f))
		}
		return next
	}
}

func _println(n *node) {
	child := n.child[1:]
	next := getExec(n.tnext)
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}

	n.exec = func(f *frame) bltn {
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

func _recover(n *node) {
	tnext := getExec(n.tnext)
	dest := genValue(n)
	var err error
	nilErr := reflect.ValueOf(valueInterface{n, reflect.ValueOf(&err).Elem()})

	n.exec = func(f *frame) bltn {
		if f.anc.recovered == nil {
			dest(f).Set(nilErr)
		} else {
			dest(f).Set(reflect.ValueOf(valueInterface{n, reflect.ValueOf(f.anc.recovered)}))
			f.anc.recovered = nil
		}
		return tnext
	}
}

func _panic(n *node) {
	value := genValue(n.child[1])

	n.exec = func(f *frame) bltn {
		panic(value(f))
	}
}

func genFunctionWrapper(n *node) func(*frame) reflect.Value {
	var def *node
	var ok bool
	if def, ok = n.val.(*node); !ok {
		return genValueAsFunctionWrapper(n)
	}
	setExec(def.child[3].start)
	start := def.child[3].start
	numRet := len(def.typ.ret)
	var rcvr func(*frame) reflect.Value

	if n.recv != nil {
		if n.recv.node.typ.cat != defRecvType(def).cat {
			rcvr = genValueRecvIndirect(n)
		} else {
			rcvr = genValueRecv(n)
		}
	}

	return func(f *frame) reflect.Value {
		if n.frame != nil { // Use closure context if defined
			f = n.frame
		}
		return reflect.MakeFunc(n.typ.TypeOf(), func(in []reflect.Value) []reflect.Value {
			// Allocate and init local frame. All values to be settable and addressable.
			fr := frame{anc: f, data: make([]reflect.Value, len(def.types))}
			d := fr.data
			for i, t := range def.types {
				d[i] = reflect.New(t).Elem()
			}

			// Copy method receiver as first argument, if defined
			if rcvr != nil {
				src, dest := rcvr(f), d[numRet]
				if src.Type().Kind() != dest.Type().Kind() {
					dest.Set(src.Addr())
				} else {
					dest.Set(src)
				}
				d = d[numRet+1:]
			} else {
				d = d[numRet:]
			}

			// Copy function input arguments in local frame
			for i, arg := range in {
				if def.typ.arg[i].cat == interfaceT {
					d[i].Set(reflect.ValueOf(valueInterface{value: arg.Elem()}))
				} else {
					d[i].Set(arg)
				}
			}

			// Interpreter code execution
			runCfg(start, &fr)

			result := fr.data[:numRet]
			for i, r := range result {
				if v, ok := r.Interface().(*node); ok {
					result[i] = genFunctionWrapper(v)(f)
				}
				if def.typ.ret[i].cat == interfaceT {
					x := result[i].Interface().(valueInterface).value
					result[i] = reflect.New(reflect.TypeOf((*interface{})(nil)).Elem()).Elem()
					result[i].Set(x)
				}
			}
			return result
		})
	}
}

func genInterfaceWrapper(n *node, typ reflect.Type) func(*frame) reflect.Value {
	value := genValue(n)
	if typ == nil || typ.Kind() != reflect.Interface || typ.NumMethod() == 0 || n.typ.cat == valueT {
		return value
	}
	if nt := n.typ.TypeOf(); nt != nil && nt.Kind() == reflect.Interface {
		return value
	}
	mn := typ.NumMethod()
	names := make([]string, mn)
	methods := make([]*node, mn)
	indexes := make([][]int, mn)
	for i := 0; i < mn; i++ {
		names[i] = typ.Method(i).Name
		methods[i], indexes[i] = n.typ.lookupMethod(names[i])
		if methods[i] == nil && n.typ.cat != nilT {
			// interpreted method not found, look for binary method, possibly embedded
			_, indexes[i], _ = n.typ.lookupBinMethod(names[i])
		}
	}
	wrap := n.interp.getWrapper(typ)

	return func(f *frame) reflect.Value {
		v := value(f)
		switch v.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
			if v.IsNil() {
				return reflect.New(typ).Elem()
			}
		}
		w := reflect.New(wrap).Elem()
		for i, m := range methods {
			if m == nil {
				if r := v.FieldByIndex(indexes[i]).MethodByName(names[i]); r.IsValid() {
					w.Field(i).Set(v.FieldByIndex(indexes[i]).MethodByName(names[i]))
				} else {
					log.Println(n.cfgErrorf("genInterfaceWrapper error, no method %s", names[i]))
				}
				continue
			}
			nod := *m
			nod.recv = &receiver{n, v, indexes[i]}
			w.Field(i).Set(genFunctionWrapper(&nod)(f))
		}
		return w
	}
}

func _defer(n *node) {
	tnext := getExec(n.tnext)
	values := make([]func(*frame) reflect.Value, len(n.child[0].child))
	var method func(*frame) reflect.Value

	for i, c := range n.child[0].child {
		if c.typ.cat == funcT {
			values[i] = genFunctionWrapper(c)
		} else {
			if c.recv != nil {
				// defer a method on a binary obj
				mi := c.val.(int)
				m := genValue(c.child[0])
				method = func(f *frame) reflect.Value { return m(f).Method(mi) }
			}
			values[i] = genValue(c)
		}
	}

	if method != nil {
		n.exec = func(f *frame) bltn {
			val := make([]reflect.Value, len(values))
			val[0] = method(f)
			for i, v := range values[1:] {
				val[i+1] = v(f)
			}
			f.deferred = append([][]reflect.Value{val}, f.deferred...)
			return tnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			val := make([]reflect.Value, len(values))
			for i, v := range values {
				val[i] = v(f)
			}
			f.deferred = append([][]reflect.Value{val}, f.deferred...)
			return tnext
		}
	}
}

func call(n *node) {
	goroutine := n.anc.kind == goStmt
	var method bool
	value := genValue(n.child[0])
	var values []func(*frame) reflect.Value
	if n.child[0].recv != nil {
		// Compute method receiver value
		values = append(values, genValueRecv(n.child[0]))
		method = true
	} else if n.child[0].action == aMethod {
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
		switch {
		case isBinCall(c):
			// Handle nested function calls: pass returned values as arguments
			numOut := c.child[0].typ.rtype.NumOut()
			for j := 0; j < numOut; j++ {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
			}
		case isRegularCall(c):
			// Arguments are return values of a nested function call
			for j := range c.child[0].typ.ret {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
			}
		default:
			if c.kind == basicLit {
				var argType reflect.Type
				if variadic >= 0 && i >= variadic {
					argType = n.child[0].typ.arg[variadic].TypeOf()
				} else {
					argType = n.child[0].typ.arg[i].TypeOf()
				}
				convertLiteralValue(c, argType)
			}
			if len(n.child[0].typ.arg) > i && n.child[0].typ.arg[i].cat == interfaceT {
				values = append(values, genValueInterface(c))
			} else {
				values = append(values, genValue(c))
			}
		}
	}

	rtypes := n.child[0].typ.ret
	rvalues := make([]func(*frame) reflect.Value, len(rtypes))
	switch n.anc.kind {
	case defineXStmt, assignXStmt:
		for i := range rvalues {
			c := n.anc.child[i]
			if c.ident != "_" {
				rvalues[i] = genValue(c)
			}
		}
	default:
		for i := range rtypes {
			j := n.findex + i
			rvalues[i] = func(f *frame) reflect.Value { return f.data[j] }
		}
	}

	n.exec = func(f *frame) bltn {
		def := value(f).Interface().(*node)
		anc := f
		// Get closure frame context (if any)
		if def.frame != nil {
			anc = def.frame
		}
		nf := frame{anc: anc, data: make([]reflect.Value, len(def.types))}
		var vararg reflect.Value

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
			nf.data[numRet+i] = reflect.New(t).Elem()
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
		if fnext != nil && !nf.data[0].Bool() {
			return fnext
		}
		return tnext
	}
}

// pindex returns definition parameter index for function call
func pindex(i, variadic int) int {
	if variadic < 0 || i <= variadic {
		return i
	}
	return variadic
}

// Call a function from a bin import, accessible through reflect
func callBin(n *node) {
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)
	child := n.child[1:]
	value := genValue(n.child[0])
	var values []func(*frame) reflect.Value
	funcType := n.child[0].typ.rtype
	variadic := -1
	if funcType.IsVariadic() {
		variadic = funcType.NumIn() - 1
	}
	// method signature obtained from reflect.Type include receiver as 1st arg, except for interface types
	rcvrOffset := 0
	if recv := n.child[0].recv; recv != nil && recv.node.typ.TypeOf().Kind() != reflect.Interface {
		rcvrOffset = 1
	}

	for i, c := range child {
		defType := funcType.In(pindex(i, variadic))
		switch {
		case isBinCall(c):
			// Handle nested function calls: pass returned values as arguments
			numOut := c.child[0].typ.rtype.NumOut()
			for j := 0; j < numOut; j++ {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
			}
		case isRegularCall(c):
			// Handle nested function calls: pass returned values as arguments
			for j := range c.child[0].typ.ret {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
			}
		default:
			if c.kind == basicLit {
				// Convert literal value (untyped) to function argument type (if not an interface{})
				var argType reflect.Type
				if variadic >= 0 && i >= variadic {
					argType = funcType.In(variadic).Elem()
				} else {
					argType = funcType.In(i + rcvrOffset)
				}
				convertLiteralValue(c, argType)
				if !reflect.ValueOf(c.val).IsValid() { //  Handle "nil"
					c.val = reflect.Zero(argType)
				}
			}
			switch c.typ.cat {
			case funcT:
				values = append(values, genFunctionWrapper(c))
			case interfaceT:
				values = append(values, genValueInterfaceValue(c))
			default:
				//values = append(values, genValue(c))
				values = append(values, genInterfaceWrapper(c, defType))
			}
		}
	}
	l := len(values)

	switch {
	case n.anc.kind == goStmt:
		// Execute function in a goroutine, discard results
		n.exec = func(f *frame) bltn {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			go value(f).Call(in)
			return tnext
		}
	case fnext != nil:
		// Handle branching according to boolean result
		n.exec = func(f *frame) bltn {
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
		switch n.anc.kind {
		case defineStmt, assignStmt, defineXStmt, assignXStmt:
			rvalues := make([]func(*frame) reflect.Value, funcType.NumOut())
			for i := range rvalues {
				c := n.anc.child[i]
				if c.ident != "_" {
					rvalues[i] = genValue(c)
				}
			}
			n.exec = func(f *frame) bltn {
				in := make([]reflect.Value, l)
				for i, v := range values {
					in[i] = v(f)
				}
				out := value(f).Call(in)
				for i, v := range rvalues {
					if v != nil {
						v(f).Set(out[i])
					}
				}
				return tnext
			}
		default:
			n.exec = func(f *frame) bltn {
				in := make([]reflect.Value, l)
				for i, v := range values {
					in[i] = v(f)
				}
				out := value(f).Call(in)
				copy(f.data[n.findex:], out)
				return tnext
			}
		}
	}
}

func getIndexBinMethod(n *node) {
	//dest := genValue(n)
	i := n.findex
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		// Can not use .Set() because dest type contains the receiver and source not
		//dest(f).Set(value(f).Method(m))
		f.data[i] = value(f).Method(m)
		return next
	}
}

func getIndexBinPtrMethod(n *node) {
	i := n.findex
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		// Can not use .Set() because dest type contains the receiver and source not
		f.data[i] = value(f).Addr().Method(m)
		return next
	}
}

// getIndexArray returns array value from index
func getIndexArray(n *node) {
	tnext := getExec(n.tnext)
	value0 := genValue(n.child[0]) // array

	if n.child[1].rval.IsValid() { // constant array index
		ai := int(vInt(n.child[1].rval))
		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if value0(f).Index(ai).Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			i := n.findex
			n.exec = func(f *frame) bltn {
				// Can not use .Set due to constraint of being able to assign an array element
				f.data[i] = value0(f).Index(ai)
				return tnext
			}
		}
	} else {
		value1 := genValueInt(n.child[1]) // array index

		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				_, vi := value1(f)
				if value0(f).Index(int(vi)).Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			i := n.findex
			n.exec = func(f *frame) bltn {
				_, vi := value1(f)
				// Can not use .Set due to constraint of being able to assign an array element
				f.data[i] = value0(f).Index(int(vi))
				return tnext
			}
		}
	}
}

// getIndexMap retrieves map value from index
func getIndexMap(n *node) {
	dest := genValue(n)
	value0 := genValue(n.child[0]) // map
	tnext := getExec(n.tnext)
	z := reflect.New(n.child[0].typ.TypeOf().Elem()).Elem()

	if n.child[1].rval.IsValid() { // constant map index
		mi := n.child[1].rval

		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(mi); v.IsValid() && v.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(mi); v.IsValid() {
					dest(f).Set(v)
				} else {
					dest(f).Set(z)
				}
				return tnext
			}
		}
	} else {
		value1 := genValue(n.child[1]) // map index

		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(value1(f)); v.IsValid() && v.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(value1(f)); v.IsValid() {
					dest(f).Set(v)
				} else {
					dest(f).Set(z)
				}
				return tnext
			}
		}
	}
}

// getIndexMap2 retrieves map value from index and set status
func getIndexMap2(n *node) {
	dest := genValue(n.anc.child[0])   // result
	value0 := genValue(n.child[0])     // map
	value2 := genValue(n.anc.child[1]) // status
	next := getExec(n.tnext)

	if n.child[1].rval.IsValid() { // constant map index
		mi := n.child[1].rval
		n.exec = func(f *frame) bltn {
			v := value0(f).MapIndex(mi)
			if v.IsValid() {
				dest(f).Set(v)
			}
			value2(f).SetBool(v.IsValid())
			return next
		}
	} else {
		value1 := genValue(n.child[1]) // map index
		n.exec = func(f *frame) bltn {
			v := value0(f).MapIndex(value1(f))
			if v.IsValid() {
				dest(f).Set(v)
			}
			value2(f).SetBool(v.IsValid())
			return next
		}
	}
}

func getFunc(n *node) {
	dest := genValue(n)
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		fr := *f
		nod := *n
		nod.val = &nod
		nod.frame = &fr
		dest(f).Set(reflect.ValueOf(&nod))
		return next
	}
}

func getMethod(n *node) {
	i := n.findex
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		fr := *f
		nod := *(n.val.(*node))
		nod.val = &nod
		nod.recv = n.recv
		nod.frame = &fr
		f.data[i] = reflect.ValueOf(&nod)
		return next
	}
}

func getMethodByName(n *node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])
	name := n.child[1].ident
	i := n.findex

	n.exec = func(f *frame) bltn {
		val := value0(f).Interface().(valueInterface)
		m, li := val.node.typ.lookupMethod(name)
		fr := *f
		nod := *m
		nod.val = &nod
		nod.recv = &receiver{nil, val.value, li}
		nod.frame = &fr
		f.data[i] = reflect.ValueOf(&nod)
		return next
	}
}

func getIndexSeq(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value(f).FieldByIndex(index).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *frame) bltn {
			f.data[i] = value(f).FieldByIndex(index)
			return tnext
		}
	}
}

func getPtrIndexSeq(n *node) {
	index := n.val.([]int)
	tnext := getExec(n.tnext)
	var value func(*frame) reflect.Value
	if isRecursiveStruct(n.child[0].typ) {
		v := genValue(n.child[0])
		value = func(f *frame) reflect.Value { return v(f).Elem().Elem() }
	} else {
		value = genValue(n.child[0])
	}

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value(f).Elem().FieldByIndex(index).Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *frame) bltn {
			f.data[i] = value(f).Elem().FieldByIndex(index)
			return tnext
		}
	}
}

func getIndexSeqField(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	i := n.findex
	next := getExec(n.tnext)

	if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
		n.exec = func(f *frame) bltn {
			f.data[i] = value(f).Elem().FieldByIndex(index)
			return next
		}
	} else {
		n.exec = func(f *frame) bltn {
			f.data[i] = value(f).FieldByIndex(index)
			return next
		}
	}
}

func getIndexSeqMethod(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	fi := index[1:]
	mi := index[0]
	i := n.findex
	next := getExec(n.tnext)

	if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
		n.exec = func(f *frame) bltn {
			f.data[i] = value(f).Elem().FieldByIndex(fi).Method(mi)
			return next
		}
	} else {
		n.exec = func(f *frame) bltn {
			f.data[i] = value(f).FieldByIndex(fi).Method(mi)
			return next
		}
	}
}

func negate(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n.exec = func(f *frame) bltn {
			dest(f).SetInt(-value(f).Int())
			return next
		}
	case reflect.Float32, reflect.Float64:
		n.exec = func(f *frame) bltn {
			dest(f).SetFloat(-value(f).Float())
			return next
		}
	case reflect.Complex64, reflect.Complex128:
		n.exec = func(f *frame) bltn {
			dest(f).SetComplex(-value(f).Complex())
			return next
		}
	}
}

func land(n *node) {
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)
	dest := genValue(n)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value0(f).Bool() && value1(f).Bool() {
				dest(f).SetBool(true)
				return tnext
			}
			dest(f).SetBool(false)
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			dest(f).SetBool(value0(f).Bool() && value1(f).Bool())
			return tnext
		}
	}
}

func lor(n *node) {
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)
	dest := genValue(n)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value0(f).Bool() || value1(f).Bool() {
				dest(f).SetBool(true)
				return tnext
			}
			dest(f).SetBool(false)
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			dest(f).SetBool(value0(f).Bool() || value1(f).Bool())
			return tnext
		}
	}
}

func nop(n *node) {
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		return next
	}
}

func branch(n *node) {
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)
	value := genValue(n)

	n.exec = func(f *frame) bltn {
		if value(f).Bool() {
			return tnext
		}
		return fnext
	}
}

func _return(n *node) {
	child := n.child
	next := getExec(n.tnext)
	def := n.val.(*node)
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		switch t := def.typ.ret[i]; t.cat {
		case errorT:
			values[i] = genInterfaceWrapper(c, t.TypeOf())
		case interfaceT:
			values[i] = genValueInterface(c)
		default:
			values[i] = genValue(c)
		}
	}

	switch len(child) {
	case 0:
		n.exec = func(f *frame) bltn { return next }
	case 1:
		if child[0].kind == binaryExpr {
			n.exec = func(f *frame) bltn { return next }
		} else {
			v := values[0]
			n.exec = func(f *frame) bltn {
				f.data[0].Set(v(f))
				return next
			}
		}
	case 2:
		v0, v1 := values[0], values[1]
		n.exec = func(f *frame) bltn {
			f.data[0].Set(v0(f))
			f.data[1].Set(v1(f))
			return next
		}
	default:
		n.exec = func(f *frame) bltn {
			for i, value := range values {
				f.data[i].Set(value(f))
			}
			return next
		}
	}
}

func arrayLit(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}

	values := make([]func(*frame) reflect.Value, len(child))
	index := make([]int, len(child))
	rtype := n.typ.val.TypeOf()
	var max, prev int

	for i, c := range child {
		if c.kind == keyValueExpr {
			convertLiteralValue(c.child[1], rtype)
			values[i] = genValue(c.child[1])
			index[i] = int(c.child[0].rval.Int())
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

	n.exec = func(f *frame) bltn {
		for i, v := range values {
			a.Index(index[i]).Set(v(f))
		}
		value(f).Set(a)
		return next
	}
}

func mapLit(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}
	typ := n.typ.TypeOf()
	keys := make([]func(*frame) reflect.Value, len(child))
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c.child[0], n.typ.key.TypeOf())
		convertLiteralValue(c.child[1], n.typ.val.TypeOf())
		keys[i] = genValue(c.child[0])
		values[i] = genValue(c.child[1])
	}

	n.exec = func(f *frame) bltn {
		m := reflect.MakeMap(typ)
		for i, k := range keys {
			m.SetMapIndex(k(f), values[i](f))
		}
		value(f).Set(m)
		return next
	}
}

func compositeBinMap(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}
	typ := n.typ.TypeOf()
	keys := make([]func(*frame) reflect.Value, len(child))
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c.child[0], typ.Key())
		convertLiteralValue(c.child[1], typ.Elem())
		keys[i] = genValue(c.child[0])
		values[i] = genValue(c.child[1])
	}

	n.exec = func(f *frame) bltn {
		m := reflect.MakeMap(typ)
		for i, k := range keys {
			m.SetMapIndex(k(f), values[i](f))
		}
		value(f).Set(m)
		return next
	}
}

// compositeBinStruct creates and populates a struct object from a binary type
func compositeBinStruct(n *node) {
	next := getExec(n.tnext)
	value := valueGenerator(n, n.findex)
	typ := n.typ.rtype
	child := n.child[1:]
	values := make([]func(*frame) reflect.Value, len(child))
	fieldIndex := make([][]int, len(child))
	for i, c := range child {
		if c.kind == keyValueExpr {
			if sf, ok := typ.FieldByName(c.child[0].ident); ok {
				fieldIndex[i] = sf.Index
				convertLiteralValue(c.child[1], sf.Type)
				if c.child[1].typ.cat == funcT {
					values[i] = genFunctionWrapper(c.child[1])
				} else {
					values[i] = genValue(c.child[1])
				}
			}
		} else {
			fieldIndex[i] = []int{i}
			convertLiteralValue(c.child[1], typ.Field(i).Type)
			if c.typ.cat == funcT {
				values[i] = genFunctionWrapper(c.child[1])
			} else {
				values[i] = genValue(c)
			}
		}
	}

	n.exec = func(f *frame) bltn {
		s := reflect.New(typ).Elem()
		for i, v := range values {
			s.FieldByIndex(fieldIndex[i]).Set(v(f))
		}
		value(f).Set(s)
		return next
	}
}

// compositeLit creates and populates a struct object
func compositeLit(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}

	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c, n.typ.field[i].typ.TypeOf())
		if c.typ.cat == funcT {
			values[i] = genFunctionWrapper(c)
		} else {
			values[i] = genValue(c)
		}
	}

	n.exec = func(f *frame) bltn {
		a := reflect.New(n.typ.TypeOf()).Elem()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		if d := value(f); d.Type().Kind() == reflect.Ptr {
			d.Set(a.Addr())
		} else {
			d.Set(a)
		}
		return next
	}
}

// compositeSparse creates a struct Object, filling fields from sparse key-values
func compositeSparse(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if !n.typ.untyped {
		child = n.child[1:]
	}

	values := make(map[int]func(*frame) reflect.Value)
	a, _ := n.typ.zero()
	for _, c := range child {
		c1 := c.child[1]
		field := n.typ.fieldIndex(c.child[0].ident)
		convertLiteralValue(c1, n.typ.field[field].typ.TypeOf())
		if c1.typ.cat == funcT {
			values[field] = genFunctionWrapper(c1)
		} else {
			values[field] = genValue(c1)
		}
	}

	n.exec = func(f *frame) bltn {
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		if d := value(f); d.Type().Kind() == reflect.Ptr {
			d.Set(a.Addr())
		} else {
			d.Set(a)
		}
		return next
	}
}

func empty(n *node) {}

func _range(n *node) {
	index0 := n.child[0].findex // array index location in frame
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	if len(n.child) == 4 {
		index1 := n.child[1].findex   // array value location in frame
		value := genValue(n.child[2]) // array
		n.exec = func(f *frame) bltn {
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
	} else {
		value := genValue(n.child[1]) // array
		n.exec = func(f *frame) bltn {
			a := value(f)
			v0 := f.data[index0]
			v0.SetInt(v0.Int() + 1)
			if int(v0.Int()) >= a.Len() {
				return fnext
			}
			return tnext
		}
	}

	// Init sequence
	next := n.exec
	n.child[0].exec = func(f *frame) bltn {
		f.data[index0].SetInt(-1)
		return next
	}
}

func rangeChan(n *node) {
	i := n.child[0].findex        // element index location in frame
	value := genValue(n.child[1]) // chan
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		v, ok := value(f).Recv()
		if !ok {
			return fnext
		}
		f.data[i].Set(v)
		return tnext
	}
}

func rangeMap(n *node) {
	index0 := n.child[0].findex   // array index location in frame
	index1 := n.child[1].findex   // array value location in frame
	value := genValue(n.child[2]) // array
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)
	// TODO: move i and keys to frame
	var i int
	var keys []reflect.Value

	n.exec = func(f *frame) bltn {
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
	n.child[0].exec = func(f *frame) bltn {
		keys = value(f).MapKeys()
		i = -1
		return next
	}
}

func _case(n *node) {
	tnext := getExec(n.tnext)

	switch {
	case n.anc.anc.kind == typeSwitch:
		fnext := getExec(n.fnext)
		sn := n.anc.anc // switch node
		types := make([]*itype, len(n.child)-1)
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
				n.exec = func(f *frame) bltn {
					destValue(f).Set(srcValue(f))
					return tnext
				}
			case 1:
				// match against 1 type: assign var to concrete value
				typ := types[0]
				n.exec = func(f *frame) bltn {
					v := srcValue(f)
					if !v.IsValid() {
						// match zero value against nil
						if typ.cat == nilT {
							return tnext
						}
						return fnext
					}
					if t := v.Type(); t.Kind() == reflect.Interface {
						if typ.cat == nilT && v.IsNil() {
							return tnext
						}
						if typ.TypeOf().String() == t.String() {
							destValue(f).Set(v.Elem())
							return tnext
						}
						return fnext
					}
					vi := v.Interface().(valueInterface)
					if vi.node == nil {
						if typ.cat == nilT {
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
				n.exec = func(f *frame) bltn {
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
				n.exec = func(f *frame) bltn { return tnext }
			} else {
				n.exec = func(f *frame) bltn {
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
		n.exec = func(f *frame) bltn { return tnext }

	default:
		fnext := getExec(n.fnext)
		l := len(n.anc.anc.child)
		value := genValue(n.anc.anc.child[l-2])
		values := make([]func(*frame) reflect.Value, len(n.child)-1)
		for i := range values {
			values[i] = genValue(n.child[i])
		}
		n.exec = func(f *frame) bltn {
			for _, v := range values {
				if value(f).Interface() == v(f).Interface() {
					return tnext
				}
			}
			return fnext
		}
	}
}

func appendSlice(n *node) {
	dest := genValue(n)
	next := getExec(n.tnext)
	value := genValue(n.child[1])
	value0 := genValue(n.child[2])

	if isString(n.child[2].typ.TypeOf()) {
		typ := reflect.TypeOf([]byte{})
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.AppendSlice(value(f), value0(f).Convert(typ)))
			return next
		}
	} else {
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.AppendSlice(value(f), value0(f)))
			return next
		}
	}

}

func _append(n *node) {
	dest := genValue(n)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	if len(n.child) > 3 {
		args := n.child[2:]
		l := len(args)
		values := make([]func(*frame) reflect.Value, l)
		for i, arg := range args {
			switch {
			case isRecursiveStruct(n.typ.val):
				values[i] = genValueInterfacePtr(arg)
			case arg.typ.untyped:
				values[i] = genValueAs(arg, n.child[1].typ.TypeOf().Elem())
			default:
				values[i] = genValue(arg)
			}
		}

		n.exec = func(f *frame) bltn {
			sl := make([]reflect.Value, l)
			for i, v := range values {
				sl[i] = v(f)
			}
			dest(f).Set(reflect.Append(value(f), sl...))
			return next
		}
	} else {
		var value0 func(*frame) reflect.Value
		switch {
		case isRecursiveStruct(n.typ.val):
			value0 = genValueInterfacePtr(n.child[2])
		case n.child[2].typ.untyped:
			value0 = genValueAs(n.child[2], n.child[1].typ.TypeOf().Elem())
		default:
			value0 = genValue(n.child[2])
		}

		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.Append(value(f), value0(f)))
			return next
		}
	}
}

func _cap(n *node) {
	dest := genValue(n)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).SetInt(int64(value(f).Cap()))
		return next
	}
}

func _copy(n *node) {
	dest := genValue(n)
	value0 := genValue(n.child[1])
	value1 := genValue(n.child[2])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).SetInt(int64(reflect.Copy(value0(f), value1(f))))
		return next
	}
}

func _close(n *node) {
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		value(f).Close()
		return next
	}
}

func _complex(n *node) {
	i := n.findex
	convertLiteralValue(n.child[1], floatType)
	convertLiteralValue(n.child[2], floatType)
	value0 := genValue(n.child[1])
	value1 := genValue(n.child[2])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		f.data[i].SetComplex(complex(value0(f).Float(), value1(f).Float()))
		return next
	}
}

func _imag(n *node) {
	i := n.findex
	convertLiteralValue(n.child[1], complexType)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		f.data[i].SetFloat(imag(value(f).Complex()))
		return next
	}
}

func _real(n *node) {
	i := n.findex
	convertLiteralValue(n.child[1], complexType)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		f.data[i].SetFloat(real(value(f).Complex()))
		return next
	}
}

func _delete(n *node) {
	value0 := genValue(n.child[1]) // map
	value1 := genValue(n.child[2]) // key
	next := getExec(n.tnext)
	var z reflect.Value

	n.exec = func(f *frame) bltn {
		value0(f).SetMapIndex(value1(f), z)
		return next
	}
}

func _len(n *node) {
	i := n.findex
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		f.data[i].SetInt(int64(value(f).Len()))
		return next
	}
}

func _new(n *node) {
	dest := genValue(n)
	next := getExec(n.tnext)
	typ := n.child[1].typ.TypeOf()

	n.exec = func(f *frame) bltn {
		dest(f).Set(reflect.New(typ))
		return next
	}
}

// _make allocates and initializes a slice, a map or a chan.
func _make(n *node) {
	dest := genValue(n)
	next := getExec(n.tnext)
	typ := n.child[1].typ.TypeOf()

	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		value := genValue(n.child[2])

		switch len(n.child) {
		case 3:
			n.exec = func(f *frame) bltn {
				len := int(value(f).Int())
				dest(f).Set(reflect.MakeSlice(typ, len, len))
				return next
			}
		case 4:
			value1 := genValue(n.child[3])
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.MakeSlice(typ, int(value(f).Int()), int(value1(f).Int())))
				return next
			}
		}

	case reflect.Chan:
		switch len(n.child) {
		case 2:
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.MakeChan(typ, 0))
				return next
			}
		case 3:
			value := genValue(n.child[2])
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.MakeChan(typ, int(value(f).Int())))
				return next
			}
		}

	case reflect.Map:
		switch len(n.child) {
		case 2:
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.MakeMap(typ))
				return next
			}
		case 3:
			value := genValue(n.child[2])
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.MakeMapWithSize(typ, int(value(f).Int())))
				return next
			}
		}
	}
}

func reset(n *node) {
	next := getExec(n.tnext)

	switch l := len(n.child) - 1; l {
	case 1:
		typ := n.child[0].typ.TypeOf()
		i := n.child[0].findex
		n.exec = func(f *frame) bltn {
			f.data[i] = reflect.New(typ).Elem()
			return next
		}
	case 2:
		c0, c1 := n.child[0], n.child[1]
		i0, i1 := c0.findex, c1.findex
		t0, t1 := c0.typ.TypeOf(), c1.typ.TypeOf()
		n.exec = func(f *frame) bltn {
			f.data[i0] = reflect.New(t0).Elem()
			f.data[i1] = reflect.New(t1).Elem()
			return next
		}
	default:
		types := make([]reflect.Type, l)
		index := make([]int, l)
		for i, c := range n.child[:l] {
			index[i] = c.findex
			types[i] = c.typ.TypeOf()
		}
		n.exec = func(f *frame) bltn {
			for i, ind := range index {
				f.data[ind] = reflect.New(types[i]).Elem()
			}
			return next
		}
	}
}

// recv reads from a channel
func recv(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if v, _ := value(f).Recv(); v.Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *frame) bltn {
			f.data[i], _ = value(f).Recv()
			return tnext
		}
	}
}

func recv2(n *node) {
	vchan := genValue(n.child[0])    // chan
	vres := genValue(n.anc.child[0]) // result
	vok := genValue(n.anc.child[1])  // status
	tnext := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		v, ok := vchan(f).Recv()
		vres(f).Set(v)
		vok(f).SetBool(ok)
		return tnext
	}
}

func convertLiteralValue(n *node, t reflect.Type) {
	if n.kind != basicLit || t == nil || t.Kind() == reflect.Interface {
		return
	}
	if n.rval.IsValid() {
		n.rval = n.rval.Convert(t)
	} else {
		n.rval = reflect.New(t).Elem() // convert to type nil value
	}
}

// Write to a channel
func send(n *node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0]) // channel
	convertLiteralValue(n.child[1], n.child[0].typ.val.TypeOf())
	value1 := genValue(n.child[1]) // value to send

	n.exec = func(f *frame) bltn {
		value0(f).Send(value1(f))
		return next
	}
}

func clauseChanDir(n *node) (*node, *node, *node, reflect.SelectDir) {
	dir := reflect.SelectDefault
	var nod, assigned, ok *node
	var stop bool

	n.Walk(func(m *node) bool {
		switch m.action {
		case aRecv:
			dir = reflect.SelectRecv
			nod = m.child[0]
			switch m.anc.action {
			case aAssign:
				assigned = m.anc.child[0]
			case aAssignX:
				assigned = m.anc.child[0]
				ok = m.anc.child[1]
			}
			stop = true
		case aSend:
			dir = reflect.SelectSend
			nod = m.child[0]
			assigned = m.child[1]
			stop = true
		}
		return !stop
	}, nil)
	return nod, assigned, ok, dir
}

func _select(n *node) {
	nbClause := len(n.child)
	chans := make([]*node, nbClause)
	assigned := make([]*node, nbClause)
	ok := make([]*node, nbClause)
	clause := make([]bltn, nbClause)
	chanValues := make([]func(*frame) reflect.Value, nbClause)
	assignedValues := make([]func(*frame) reflect.Value, nbClause)
	okValues := make([]func(*frame) reflect.Value, nbClause)
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

	n.exec = func(f *frame) bltn {
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
func slice(n *node) {
	i := n.findex
	next := getExec(n.tnext)
	value0 := genValue(n.child[0]) // array
	value1 := genValue(n.child[1]) // low (if 2 or 3 args) or high (if 1 arg)

	switch len(n.child) {
	case 2:
		n.exec = func(f *frame) bltn {
			a := value0(f)
			f.data[i] = a.Slice(int(value1(f).Int()), a.Len())
			return next
		}
	case 3:
		value2 := genValue(n.child[2]) // max

		n.exec = func(f *frame) bltn {
			a := value0(f)
			f.data[i] = a.Slice(int(value1(f).Int()), int(value2(f).Int()))
			return next
		}
	case 4:
		value2 := genValue(n.child[2])
		value3 := genValue(n.child[3])

		n.exec = func(f *frame) bltn {
			a := value0(f)
			f.data[i] = a.Slice3(int(value1(f).Int()), int(value2(f).Int()), int(value3(f).Int()))
			return next
		}
	}
}

// slice expression, no low value: array[:high:max]
func slice0(n *node) {
	i := n.findex
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])

	switch len(n.child) {
	case 1:
		n.exec = func(f *frame) bltn {
			a := value0(f)
			f.data[i] = a.Slice(0, a.Len())
			return next
		}
	case 2:
		value1 := genValue(n.child[1])
		n.exec = func(f *frame) bltn {
			a := value0(f)
			f.data[i] = a.Slice(0, int(value1(f).Int()))
			return next
		}
	case 3:
		value1 := genValue(n.child[1])
		value2 := genValue(n.child[2])
		n.exec = func(f *frame) bltn {
			a := value0(f)
			f.data[i] = a.Slice3(0, int(value1(f).Int()), int(value2(f).Int()))
			return next
		}
	}
}

func isNil(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value(f).IsNil() {
				return tnext
			}
			return fnext
		}
	} else {
		i := n.findex
		n.exec = func(f *frame) bltn {
			f.data[i].SetBool(value(f).IsNil())
			return tnext
		}
	}
}

func isNotNil(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value(f).IsNil() {
				return fnext
			}
			return tnext
		}
	} else {
		i := n.findex
		n.exec = func(f *frame) bltn {
			f.data[i].SetBool(!value(f).IsNil())
			return tnext
		}
	}
}
