package interp

//go:generate go run ../internal/genop/genop.go

import (
	"fmt"
	"go/constant"
	"log"
	"reflect"
	"sync"
	"unsafe"
)

// bltn type defines functions which run at CFG execution.
type bltn func(f *frame) bltn

// bltnGenerator type defines a builtin generator function.
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
	aBitNot:       bitNot,
	aCall:         call,
	aCallSlice:    call,
	aCase:         _case,
	aCompositeLit: arrayLit,
	aDec:          dec,
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
	aNeg:          neg,
	aNot:          not,
	aNotEqual:     notEqual,
	aOr:           or,
	aOrAssign:     orAssign,
	aPos:          pos,
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
	if n == nil {
		return
	}
	var f *frame
	if cf == nil {
		f = interp.frame
	} else {
		f = newFrame(cf, len(n.types), interp.runid())
	}
	interp.mutex.RLock()
	c := reflect.ValueOf(interp.done)
	interp.mutex.RUnlock()

	f.mutex.Lock()
	f.done = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: c}
	f.mutex.Unlock()

	for i, t := range n.types {
		f.data[i] = reflect.New(t).Elem()
	}
	runCfg(n.start, f)
}

// Functions set to run during execution of CFG.

// runCfg executes a node AST by walking its CFG and running node builtin at each step.
func runCfg(n *node, f *frame) {
	defer func() {
		f.mutex.Lock()
		f.recovered = recover()
		for _, val := range f.deferred {
			val[0].Call(val[1:])
		}
		if f.recovered != nil {
			fmt.Println(n.cfgErrorf("panic"))
			f.mutex.Unlock()
			panic(f.recovered)
		}
		f.mutex.Unlock()
	}()

	for exec := n.exec; exec != nil && f.runid() == n.interp.runid(); {
		exec = exec(f)
	}
}

func typeAssertStatus(n *node) {
	c0, c1 := n.child[0], n.child[1]   // cO contains the input value, c1 the type to assert
	value := genValue(c0)              // input value
	value1 := genValue(n.anc.child[1]) // returned status
	rtype := c1.typ.rtype              // type to assert
	next := getExec(n.tnext)

	switch {
	case isInterfaceSrc(c1.typ):
		typ := c1.typ
		n.exec = func(f *frame) bltn {
			v, ok := value(f).Interface().(valueInterface)
			value1(f).SetBool(ok && v.node.typ.implements(typ))
			return next
		}
	case isInterface(c1.typ):
		n.exec = func(f *frame) bltn {
			v := value(f)
			ok := v.IsValid() && canAssertTypes(v.Elem().Type(), rtype)
			value1(f).SetBool(ok)
			return next
		}
	case c0.typ.cat == valueT || c0.typ.cat == errorT:
		n.exec = func(f *frame) bltn {
			v := value(f)
			ok := v.IsValid() && canAssertTypes(v.Elem().Type(), rtype)
			value1(f).SetBool(ok)
			return next
		}
	default:
		n.exec = func(f *frame) bltn {
			v, ok := value(f).Interface().(valueInterface)
			ok = ok && v.value.IsValid() && canAssertTypes(v.value.Type(), rtype)
			value1(f).SetBool(ok)
			return next
		}
	}
}

func typeAssert(n *node) {
	c0, c1 := n.child[0], n.child[1]
	value := genValue(c0) // input value
	value0 := genValue(n) // returned result
	next := getExec(n.tnext)

	switch {
	case isInterfaceSrc(c1.typ):
		typ := n.child[1].typ
		typID := n.child[1].typ.id()
		n.exec = func(f *frame) bltn {
			v := value(f)
			vi, ok := v.Interface().(valueInterface)
			if !ok {
				panic(n.cfgErrorf("interface conversion: nil is not %v", typID))
			}
			if !vi.node.typ.implements(typ) {
				panic(n.cfgErrorf("interface conversion: %v is not %v", vi.node.typ.id(), typID))
			}
			value0(f).Set(v)
			return next
		}
	case isInterface(c1.typ):
		n.exec = func(f *frame) bltn {
			v := value(f).Elem()
			typ := value0(f).Type()
			if !v.IsValid() {
				panic(fmt.Sprintf("interface conversion: interface {} is nil, not %s", typ.String()))
			}
			if !canAssertTypes(v.Type(), typ) {
				method := firstMissingMethod(v.Type(), typ)
				panic(fmt.Sprintf("interface conversion: %s is not %s: missing method %s", v.Type().String(), typ.String(), method))
			}
			value0(f).Set(v)
			return next
		}
	case c0.typ.cat == valueT || c0.typ.cat == errorT:
		n.exec = func(f *frame) bltn {
			v := value(f).Elem()
			typ := value0(f).Type()
			if !v.IsValid() {
				panic(fmt.Sprintf("interface conversion: interface {} is nil, not %s", typ.String()))
			}
			if !canAssertTypes(v.Type(), typ) {
				method := firstMissingMethod(v.Type(), typ)
				panic(fmt.Sprintf("interface conversion: %s is not %s: missing method %s", v.Type().String(), typ.String(), method))
			}
			value0(f).Set(v)
			return next
		}
	default:
		n.exec = func(f *frame) bltn {
			v := value(f).Interface().(valueInterface)
			typ := value0(f).Type()
			if !v.value.IsValid() {
				panic(fmt.Sprintf("interface conversion: interface {} is nil, not %s", typ.String()))
			}
			if !canAssertTypes(v.value.Type(), typ) {
				panic(fmt.Sprintf("interface conversion: interface {} is %s, not %s", v.value.Type().String(), typ.String()))
			}
			value0(f).Set(v.value)
			return next
		}
	}
}

func typeAssert2(n *node) {
	c0, c1 := n.child[0], n.child[1]
	value := genValue(c0)                    // input value
	value0 := genValue(n.anc.child[0])       // returned result
	value1 := genValue(n.anc.child[1])       // returned status
	setStatus := n.anc.child[1].ident != "_" // do not assign status to "_"
	typ := c1.typ                            // type to assert or convert to
	typID := typ.id()
	rtype := typ.rtype // type to assert
	next := getExec(n.tnext)

	switch {
	case isInterfaceSrc(typ):
		n.exec = func(f *frame) bltn {
			v, ok := value(f).Interface().(valueInterface)
			if ok && v.node.typ.id() == typID {
				value0(f).Set(value(f))
			} else {
				ok = false
			}
			if setStatus {
				value1(f).SetBool(ok)
			}
			return next
		}
	case isInterface(typ):
		n.exec = func(f *frame) bltn {
			v := value(f).Elem()
			ok := v.IsValid() && canAssertTypes(v.Type(), rtype)
			if ok {
				value0(f).Set(v)
			}
			if setStatus {
				value1(f).SetBool(ok)
			}
			return next
		}
	case n.child[0].typ.cat == valueT || n.child[0].typ.cat == errorT:
		n.exec = func(f *frame) bltn {
			v := value(f).Elem()
			ok := v.IsValid() && canAssertTypes(v.Type(), rtype)
			if ok {
				value0(f).Set(v)
			}
			if setStatus {
				value1(f).SetBool(ok)
			}
			return next
		}
	default:
		n.exec = func(f *frame) bltn {
			v, ok := value(f).Interface().(valueInterface)
			ok = ok && v.value.IsValid() && canAssertTypes(v.value.Type(), rtype)
			if ok {
				value0(f).Set(v.value)
			}
			if setStatus {
				value1(f).SetBool(ok)
			}
			return next
		}
	}
}

func canAssertTypes(src, dest reflect.Type) bool {
	if dest == nil {
		return false
	}
	if src == dest {
		return true
	}
	if dest.Kind() == reflect.Interface && src.Implements(dest) {
		return true
	}
	if src.AssignableTo(dest) {
		return true
	}
	return false
}

func firstMissingMethod(src, dest reflect.Type) string {
	for i := 0; i < dest.NumMethod(); i++ {
		m := dest.Method(i).Name
		if _, ok := src.MethodByName(m); !ok {
			return m
		}
	}
	return ""
}

func convert(n *node) {
	dest := genValue(n)
	c := n.child[1]
	typ := n.child[0].typ.TypeOf()
	next := getExec(n.tnext)

	if c.isNil() { // convert nil to type
		if n.child[0].typ.cat == interfaceT {
			typ = reflect.TypeOf((*valueInterface)(nil)).Elem()
		}
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

	for _, con := range n.interp.hooks.convert {
		if c.typ.rtype == nil {
			continue
		}

		fn := con(c.typ.rtype, typ)
		if fn == nil {
			continue
		}
		n.exec = func(f *frame) bltn {
			fn(value(f), dest(f))
			return next
		}
		return
	}

	n.exec = func(f *frame) bltn {
		dest(f).Set(value(f).Convert(typ))
		return next
	}
}

func isRecursiveType(t *itype, rtype reflect.Type) bool {
	if t.cat == structT && rtype.Kind() == reflect.Interface {
		return true
	}
	switch t.cat {
	case ptrT, arrayT, mapT:
		return isRecursiveType(t.val, t.val.rtype)
	default:
		return false
	}
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
		case (dest.typ.cat == valueT || dest.typ.cat == errorT) && dest.typ.rtype.Kind() == reflect.Interface:
			svalue[i] = genInterfaceWrapper(src, dest.typ.rtype)
		case src.typ.cat == funcT && dest.typ.cat == valueT:
			svalue[i] = genFunctionWrapper(src)
		case src.typ.cat == funcT && isField(dest):
			svalue[i] = genFunctionWrapper(src)
		case dest.typ.cat == funcT && src.typ.cat == valueT:
			svalue[i] = genValueNode(src)
		case src.kind == basicLit && src.val == nil:
			t := dest.typ.TypeOf()
			svalue[i] = func(*frame) reflect.Value { return reflect.New(t).Elem() }
		case isRecursiveType(dest.typ, dest.typ.rtype):
			svalue[i] = genValueRecursiveInterface(src, dest.typ.rtype)
		case isRecursiveType(src.typ, src.typ.rtype):
			svalue[i] = genValueRecursiveInterfacePtrValue(src)
		case src.typ.untyped && isComplex(dest.typ.TypeOf()):
			svalue[i] = genValueComplex(src)
		case src.typ.untyped && !dest.typ.untyped:
			svalue[i] = genValueAs(src, dest.typ.TypeOf())
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
		switch s, d, i := svalue[0], dvalue[0], ivalue[0]; {
		case n.child[0].ident == "_":
			n.exec = func(f *frame) bltn {
				return next
			}
		case i != nil:
			n.exec = func(f *frame) bltn {
				d(f).SetMapIndex(i(f), s(f))
				return next
			}
		default:
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
			types[i] = t
		}

		// To handle swap in multi-assign:
		// evaluate and copy all values in assign right hand side into temporary
		// then evaluate assign left hand side and copy temporary into it
		n.exec = func(f *frame) bltn {
			t := make([]reflect.Value, len(svalue))
			for i, s := range svalue {
				if n.child[i].ident == "_" {
					continue
				}
				t[i] = reflect.New(types[i]).Elem()
				t[i].Set(s(f))
			}
			for i, d := range dvalue {
				if n.child[i].ident == "_" {
					continue
				}
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
	dest := genValue(n)
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if !value(f).Bool() {
				dest(f).SetBool(true)
				return tnext
			}
			dest(f).SetBool(false)
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			dest(f).SetBool(!value(f).Bool())
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
	i := n.findex
	l := n.level

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			r := value(f).Elem()
			if r.Bool() {
				getFrame(f, l).data[i] = r
				return tnext
			}
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).Elem()
			return tnext
		}
	}
}

func _print(n *node) {
	child := n.child[1:]
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}
	out := n.interp.stdout

	genBuiltinDeferWrapper(n, values, nil, func(args []reflect.Value) []reflect.Value {
		for i, value := range args {
			if i > 0 {
				fmt.Fprintf(out, " ")
			}
			fmt.Fprintf(out, "%v", value)
		}
		return nil
	})
}

func _println(n *node) {
	child := n.child[1:]
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}
	out := n.interp.stdout

	genBuiltinDeferWrapper(n, values, nil, func(args []reflect.Value) []reflect.Value {
		for i, value := range args {
			if i > 0 {
				fmt.Fprintf(out, " ")
			}
			fmt.Fprintf(out, "%v", value)
		}
		fmt.Fprintln(out, "")
		return nil
	})
}

func _recover(n *node) {
	tnext := getExec(n.tnext)
	dest := genValue(n)

	n.exec = func(f *frame) bltn {
		if f.anc.recovered == nil {
			dest(f).Set(reflect.ValueOf(valueInterface{}))
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

func genBuiltinDeferWrapper(n *node, in, out []func(*frame) reflect.Value, fn func([]reflect.Value) []reflect.Value) {
	next := getExec(n.tnext)

	if n.anc.kind == deferStmt {
		n.exec = func(f *frame) bltn {
			val := make([]reflect.Value, len(in)+1)
			inTypes := make([]reflect.Type, len(in))
			for i, v := range in {
				val[i+1] = v(f)
				inTypes[i] = val[i+1].Type()
			}
			outTypes := make([]reflect.Type, len(out))
			for i, v := range out {
				outTypes[i] = v(f).Type()
			}

			funcType := reflect.FuncOf(inTypes, outTypes, false)
			val[0] = reflect.MakeFunc(funcType, fn)
			f.deferred = append([][]reflect.Value{val}, f.deferred...)
			return next
		}
		return
	}

	n.exec = func(f *frame) bltn {
		val := make([]reflect.Value, len(in))
		for i, v := range in {
			val[i] = v(f)
		}

		dests := fn(val)

		for i, dest := range dests {
			out[i](f).Set(dest)
		}
		return next
	}
}

func genFunctionWrapper(n *node) func(*frame) reflect.Value {
	var def *node
	var ok bool

	if n.kind == basicLit {
		return func(f *frame) reflect.Value { return n.rval }
	}
	if def, ok = n.val.(*node); !ok {
		return genValueAsFunctionWrapper(n)
	}
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
	funcType := n.typ.TypeOf()

	return func(f *frame) reflect.Value {
		if n.frame != nil { // Use closure context if defined
			f = n.frame
		}
		return reflect.MakeFunc(funcType, func(in []reflect.Value) []reflect.Value {
			// Allocate and init local frame. All values to be settable and addressable.
			fr := newFrame(f, len(def.types), f.runid())
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
				typ := def.typ.arg[i]
				switch {
				case typ.cat == interfaceT:
					d[i].Set(reflect.ValueOf(valueInterface{value: arg.Elem()}))
				case typ.cat == funcT && arg.Kind() == reflect.Func:
					d[i].Set(reflect.ValueOf(genFunctionNode(arg)))
				default:
					d[i].Set(arg)
				}
			}

			// Interpreter code execution
			runCfg(start, fr)

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

func genFunctionNode(v reflect.Value) *node {
	return &node{kind: funcType, action: aNop, rval: v, typ: &itype{cat: valueT, rtype: v.Type()}}
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
			_, indexes[i], _, _ = n.typ.lookupBinMethod(names[i])
		}
	}
	wrap := n.interp.getWrapper(typ)

	return func(f *frame) reflect.Value {
		v := value(f)
		vv := v
		switch v.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
			if v.IsNil() {
				return reflect.New(typ).Elem()
			}
			if v.Kind() == reflect.Ptr {
				vv = v.Elem()
			}
		}
		w := reflect.New(wrap).Elem()
		for i, m := range methods {
			if m == nil {
				if r := v.MethodByName(names[i]); r.IsValid() {
					w.Field(i).Set(r)
					continue
				}
				o := vv.FieldByIndex(indexes[i])
				if r := o.MethodByName(names[i]); r.IsValid() {
					w.Field(i).Set(r)
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

func call(n *node) {
	goroutine := n.anc.kind == goStmt
	var method bool
	value := genValue(n.child[0])
	var values []func(*frame) reflect.Value
	if n.child[0].recv != nil {
		// Compute method receiver value.
		if isRecursiveType(n.child[0].recv.node.typ, n.child[0].recv.node.typ.rtype) {
			values = append(values, genValueRecvInterfacePtr(n.child[0]))
		} else {
			values = append(values, genValueRecv(n.child[0]))
		}
		method = true
	} else if n.child[0].action == aMethod {
		// Add a place holder for interface method receiver.
		values = append(values, nil)
		method = true
	}
	numRet := len(n.child[0].typ.ret)
	variadic := variadicPos(n)
	child := n.child[1:]
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)

	// Compute input argument value functions.
	for i, c := range child {
		switch {
		case isBinCall(c):
			// Handle nested function calls: pass returned values as arguments.
			numOut := c.child[0].typ.rtype.NumOut()
			for j := 0; j < numOut; j++ {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
			}
		case isRegularCall(c):
			// Arguments are return values of a nested function call.
			for j := range c.child[0].typ.ret {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
			}
		default:
			var arg *itype
			if variadic >= 0 && i >= variadic {
				arg = n.child[0].typ.arg[variadic].val
			} else {
				arg = n.child[0].typ.arg[i]
			}
			if c.kind == basicLit || c.rval.IsValid() {
				argType := arg.TypeOf()
				convertLiteralValue(c, argType)
			}
			switch {
			case arg.cat == interfaceT:
				values = append(values, genValueInterface(c))
			case isRecursiveType(c.typ, c.typ.rtype):
				values = append(values, genValueRecursiveInterfacePtrValue(c))
			default:
				values = append(values, genValue(c))
			}
		}
	}

	// Compute output argument value functions.
	rtypes := n.child[0].typ.ret
	rvalues := make([]func(*frame) reflect.Value, len(rtypes))
	switch n.anc.kind {
	case defineXStmt, assignXStmt:
		for i := range rvalues {
			c := n.anc.child[i]
			switch {
			case c.ident == "_":
				// Skip assigning return value to blank var.
			case c.typ.cat == interfaceT && rtypes[i].cat != interfaceT:
				rvalues[i] = genValueInterfaceValue(c)
			default:
				rvalues[i] = genValue(c)
			}
		}
	case returnStmt:
		// Function call from a return statement: forward return values (always at frame start).
		for i := range rtypes {
			j := n.findex + i
			ret := n.child[0].typ.ret[i]
			callret := n.anc.val.(*node).typ.ret[i]
			if callret.cat == interfaceT && ret.cat != interfaceT {
				// Wrap the returned value in a valueInterface in caller frame.
				rvalues[i] = func(f *frame) reflect.Value {
					v := reflect.New(ret.rtype).Elem()
					f.data[j].Set(reflect.ValueOf(valueInterface{n, v}))
					return v
				}
			} else {
				// Set the return value location in return value of caller frame.
				rvalues[i] = func(f *frame) reflect.Value { return f.data[j] }
			}
		}
	default:
		// Multiple return values frame index are indexed from the node frame index.
		l := n.level
		for i := range rtypes {
			j := n.findex + i
			rvalues[i] = func(f *frame) reflect.Value { return getFrame(f, l).data[j] }
		}
	}

	if n.anc.kind == deferStmt {
		// Store function call in frame for deferred execution.
		value = genFunctionWrapper(n.child[0])
		if method {
			// The receiver is already passed in the function wrapper, skip it.
			values = values[1:]
		}
		n.exec = func(f *frame) bltn {
			val := make([]reflect.Value, len(values)+1)
			val[0] = value(f)
			for i, v := range values {
				val[i+1] = v(f)
			}
			f.deferred = append([][]reflect.Value{val}, f.deferred...)
			return tnext
		}
		return
	}

	n.exec = func(f *frame) bltn {
		var def *node
		var ok bool
		bf := value(f)
		if def, ok = bf.Interface().(*node); ok {
			bf = def.rval
		}

		// Call bin func if defined
		if bf.IsValid() {
			in := make([]reflect.Value, len(values))
			for i, v := range values {
				in[i] = v(f)
			}
			if goroutine {
				go bf.Call(in)
				return tnext
			}
			out := bf.Call(in)
			for i, v := range rvalues {
				if v != nil {
					v(f).Set(out[i])
				}
			}
			if fnext != nil && !out[0].Bool() {
				return fnext
			}
			return tnext
		}

		anc := f
		// Get closure frame context (if any)
		if def.frame != nil {
			anc = def.frame
		}
		nf := newFrame(anc, len(def.types), anc.runid())
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
		varIndex := variadic
		if variadic >= 0 {
			if method {
				vararg = nf.data[numRet+variadic+1]
				varIndex++
			} else {
				vararg = nf.data[numRet+variadic]
			}
		}

		// Copy input parameters from caller
		if dest := nf.data[numRet:]; len(dest) > 0 {
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
				case variadic >= 0 && i >= varIndex:
					if v(f).Type() == vararg.Type() {
						vararg.Set(v(f))
					} else {
						vararg.Set(reflect.Append(vararg, v(f)))
					}
				default:
					val := v(f)
					if !val.IsZero() {
						dest[i].Set(val)
					}
				}
			}
		}

		// Execute function body
		if goroutine {
			go runCfg(def.child[3].start, nf)
			return tnext
		}
		runCfg(def.child[3].start, nf)

		// Handle branching according to boolean result
		if fnext != nil && !nf.data[0].Bool() {
			return fnext
		}
		return tnext
	}
}

func getFrame(f *frame, l int) *frame {
	switch l {
	case 0:
		return f
	case 1:
		return f.anc
	case 2:
		return f.anc.anc
	}
	for ; l > 0; l-- {
		f = f.anc
	}
	return f
}

// Callbin calls a function from a bin import, accessible through reflect.
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
	// A method signature obtained from reflect.Type includes receiver as 1st arg, except for interface types.
	rcvrOffset := 0
	if recv := n.child[0].recv; recv != nil && !isInterface(recv.node.typ) {
		numIn, numChild := funcType.NumIn(), len(child)
		if variadic > 0 && numIn > variadic || numIn > numChild {
			rcvrOffset = 1
		}
	}

	// Determine if we should use `Call` or `CallSlice` on the function Value.
	callFn := func(v reflect.Value, in []reflect.Value) []reflect.Value { return v.Call(in) }
	if n.action == aCallSlice {
		callFn = func(v reflect.Value, in []reflect.Value) []reflect.Value { return v.CallSlice(in) }
	}

	for i, c := range child {
		var defType reflect.Type
		if variadic >= 0 && i >= variadic {
			defType = funcType.In(variadic)
		} else {
			defType = funcType.In(rcvrOffset + i)
		}

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
			if c.kind == basicLit || c.rval.IsValid() {
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
			case arrayT, variadicT:
				switch c.typ.val.cat {
				case interfaceT:
					values = append(values, genValueInterfaceArray(c))
				default:
					values = append(values, genInterfaceWrapper(c, defType))
				}
			default:
				values = append(values, genInterfaceWrapper(c, defType))
			}
		}
	}
	l := len(values)

	switch {
	case n.anc.kind == deferStmt:
		// Store function call in frame for deferred execution.
		n.exec = func(f *frame) bltn {
			val := make([]reflect.Value, l+1)
			val[0] = value(f)
			for i, v := range values {
				val[i+1] = v(f)
			}
			f.deferred = append([][]reflect.Value{val}, f.deferred...)
			return tnext
		}
	case n.anc.kind == goStmt:
		// Execute function in a goroutine, discard results.
		n.exec = func(f *frame) bltn {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			go callFn(value(f), in)
			return tnext
		}
	case fnext != nil:
		// Handle branching according to boolean result.
		index := n.findex
		level := n.level
		n.exec = func(f *frame) bltn {
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = v(f)
			}
			res := callFn(value(f), in)
			b := res[0].Bool()
			getFrame(f, level).data[index].SetBool(b)
			if b {
				return tnext
			}
			return fnext
		}
	default:
		switch n.anc.action {
		case aAssignX:
			// The function call is part of an assign expression, store results direcly
			// to assigned location, to avoid an additional frame copy.
			// The optimization of aAssign is handled in assign(), and should not
			// be handled here.
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
				out := callFn(value(f), in)
				for i, v := range rvalues {
					if v != nil {
						v(f).Set(out[i])
					}
				}
				return tnext
			}
		case aReturn:
			// The function call is part of a return statement, store output results
			// directly in the frame location of outputs of the current function.
			b := childPos(n)
			n.exec = func(f *frame) bltn {
				in := make([]reflect.Value, l)
				for i, v := range values {
					in[i] = v(f)
				}
				out := callFn(value(f), in)
				for i, v := range out {
					f.data[b+i].Set(v)
				}
				return tnext
			}
		default:
			n.exec = func(f *frame) bltn {
				in := make([]reflect.Value, l)
				for i, v := range values {
					in[i] = v(f)
				}
				out := callFn(value(f), in)
				for i := 0; i < len(out); i++ {
					getFrame(f, n.level).data[n.findex+i].Set(out[i])
				}
				return tnext
			}
		}
	}
}

func getIndexBinMethod(n *node) {
	// dest := genValue(n)
	i := n.findex
	l := n.level
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		// Can not use .Set() because dest type contains the receiver and source not
		// dest(f).Set(value(f).Method(m))
		getFrame(f, l).data[i] = value(f).Method(m)
		return next
	}
}

func getIndexBinPtrMethod(n *node) {
	i := n.findex
	l := n.level
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		// Can not use .Set() because dest type contains the receiver and source not
		getFrame(f, l).data[i] = value(f).Addr().Method(m)
		return next
	}
}

// getIndexArray returns array value from index.
func getIndexArray(n *node) {
	tnext := getExec(n.tnext)
	value0 := genValueArray(n.child[0]) // array
	i := n.findex
	l := n.level

	if n.child[1].rval.IsValid() { // constant array index
		ai := int(vInt(n.child[1].rval))
		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				r := value0(f).Index(ai)
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value0(f).Index(ai)
				return tnext
			}
		}
	} else {
		value1 := genValueInt(n.child[1]) // array index

		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				_, vi := value1(f)
				r := value0(f).Index(int(vi))
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				_, vi := value1(f)
				getFrame(f, l).data[i] = value0(f).Index(int(vi))
				return tnext
			}
		}
	}
}

// valueInterfaceType is the reflection type of valueInterface.
var valueInterfaceType = reflect.TypeOf((*valueInterface)(nil)).Elem()

// getIndexMap retrieves map value from index.
func getIndexMap(n *node) {
	dest := genValue(n)
	value0 := genValue(n.child[0]) // map
	tnext := getExec(n.tnext)
	z := reflect.New(n.child[0].typ.frameType().Elem()).Elem()

	if n.child[1].rval.IsValid() { // constant map index
		mi := n.child[1].rval

		switch {
		case n.fnext != nil:
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(mi); v.IsValid() && v.Bool() {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).Set(z)
				return fnext
			}
		case n.typ.cat == interfaceT:
			z = reflect.New(n.child[0].typ.val.frameType()).Elem()
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(mi); v.IsValid() {
					if e := v.Elem(); e.Type().AssignableTo(valueInterfaceType) {
						dest(f).Set(e)
					} else {
						dest(f).Set(reflect.ValueOf(valueInterface{n, e}))
					}
				} else {
					dest(f).Set(z)
				}
				return tnext
			}
		default:
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

		switch {
		case n.fnext != nil:
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(value1(f)); v.IsValid() && v.Bool() {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).Set(z)
				return fnext
			}
		case n.typ.cat == interfaceT:
			z = reflect.New(n.child[0].typ.val.frameType()).Elem()
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(value1(f)); v.IsValid() {
					if e := v.Elem(); e.Type().AssignableTo(valueInterfaceType) {
						dest(f).Set(e)
					} else {
						dest(f).Set(reflect.ValueOf(valueInterface{n, e}))
					}
				} else {
					dest(f).Set(z)
				}
				return tnext
			}
		default:
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

// getIndexMap2 retrieves map value from index and set status.
func getIndexMap2(n *node) {
	dest := genValue(n.anc.child[0])   // result
	value0 := genValue(n.child[0])     // map
	value2 := genValue(n.anc.child[1]) // status
	next := getExec(n.tnext)
	typ := n.anc.child[0].typ
	doValue := n.anc.child[0].ident != "_"
	doStatus := n.anc.child[1].ident != "_"

	if !doValue && !doStatus {
		nop(n)
		return
	}
	if n.child[1].rval.IsValid() { // constant map index
		mi := n.child[1].rval
		switch {
		case !doValue:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(mi)
				value2(f).SetBool(v.IsValid())
				return next
			}
		case typ.cat == interfaceT:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(mi)
				if v.IsValid() {
					if e := v.Elem(); e.Type().AssignableTo(valueInterfaceType) {
						dest(f).Set(e)
					} else {
						dest(f).Set(reflect.ValueOf(valueInterface{n, e}))
					}
				}
				if doStatus {
					value2(f).SetBool(v.IsValid())
				}
				return next
			}
		default:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(mi)
				if v.IsValid() {
					dest(f).Set(v)
				}
				if doStatus {
					value2(f).SetBool(v.IsValid())
				}
				return next
			}
		}
	} else {
		value1 := genValue(n.child[1]) // map index
		switch {
		case !doValue:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(value1(f))
				value2(f).SetBool(v.IsValid())
				return next
			}
		case typ.cat == interfaceT:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(value1(f))
				if v.IsValid() {
					if e := v.Elem(); e.Type().AssignableTo(valueInterfaceType) {
						dest(f).Set(e)
					} else {
						dest(f).Set(reflect.ValueOf(valueInterface{n, e}))
					}
				}
				if doStatus {
					value2(f).SetBool(v.IsValid())
				}
				return next
			}
		default:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(value1(f))
				if v.IsValid() {
					dest(f).Set(v)
				}
				if doStatus {
					value2(f).SetBool(v.IsValid())
				}
				return next
			}
		}
	}
}

func getFunc(n *node) {
	dest := genValue(n)
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		fr := f.clone()
		nod := *n
		nod.val = &nod
		nod.frame = fr
		dest(f).Set(reflect.ValueOf(&nod))
		return next
	}
}

func getMethod(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		fr := f.clone()
		nod := *(n.val.(*node))
		nod.val = &nod
		nod.recv = n.recv
		nod.frame = fr
		getFrame(f, l).data[i] = reflect.ValueOf(&nod)
		return next
	}
}

func getMethodByName(n *node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])
	name := n.child[1].ident
	i := n.findex
	l := n.level

	n.exec = func(f *frame) bltn {
		val := value0(f).Interface().(valueInterface)
		m, li := val.node.typ.lookupMethod(name)
		fr := f.clone()
		nod := *m
		nod.val = &nod
		nod.recv = &receiver{nil, val.value, li}
		nod.frame = fr
		getFrame(f, l).data[i] = reflect.ValueOf(&nod)
		return next
	}
}

func getIndexSeq(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	tnext := getExec(n.tnext)
	i := n.findex
	l := n.level

	// Note:
	// Here we have to store the result using
	//    f.data[i] = value(...)
	// instead of normal
	//    dest(f).Set(value(...)
	// because the value returned by FieldByIndex() must be preserved
	// for possible future Set operations on the struct field (avoid a
	// dereference from Set, resulting in setting a copy of the
	// original field).

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			v := value(f)
			if v.Type().Kind() == reflect.Interface && n.child[0].typ.recursive {
				v = writableDeref(v)
			}
			r := v.FieldByIndex(index)
			getFrame(f, l).data[i] = r
			if r.Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			v := value(f)
			if v.Type().Kind() == reflect.Interface && n.child[0].typ.recursive {
				v = writableDeref(v)
			}
			getFrame(f, l).data[i] = v.FieldByIndex(index)
			return tnext
		}
	}
}

//go:nocheckptr
func writableDeref(v reflect.Value) reflect.Value {
	// Here we have an interface to a struct. Any attempt to dereference it will
	// make a copy of the struct. We need to get a Value to the actual struct.
	// TODO: using unsafe is a temporary measure. Rethink this.
	return reflect.NewAt(v.Elem().Type(), unsafe.Pointer(v.InterfaceData()[1])).Elem() //nolint:govet
}

func getPtrIndexSeq(n *node) {
	index := n.val.([]int)
	tnext := getExec(n.tnext)
	var value func(*frame) reflect.Value
	if isRecursiveType(n.child[0].typ, n.child[0].typ.rtype) {
		v := genValue(n.child[0])
		value = func(f *frame) reflect.Value { return v(f).Elem().Elem() }
	} else {
		value = genValue(n.child[0])
	}
	i := n.findex
	l := n.level

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			r := value(f).Elem().FieldByIndex(index)
			getFrame(f, l).data[i] = r
			if r.Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(index)
			return tnext
		}
	}
}

func getIndexSeqField(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	i := n.findex
	l := n.level
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
			n.exec = func(f *frame) bltn {
				r := value(f).Elem().FieldByIndex(index)
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				r := value(f).FieldByIndex(index)
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		}
	} else {
		if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(index)
				return tnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).FieldByIndex(index)
				return tnext
			}
		}
	}
}

func getIndexSeqPtrMethod(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	fi := index[1:]
	mi := index[0]
	i := n.findex
	l := n.level
	next := getExec(n.tnext)

	if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(fi).Addr().Method(mi)
			return next
		}
	} else {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).FieldByIndex(fi).Addr().Method(mi)
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
	l := n.level
	next := getExec(n.tnext)

	if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(fi).Method(mi)
			return next
		}
	} else {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).FieldByIndex(fi).Method(mi)
			return next
		}
	}
}

func neg(n *node) {
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

func pos(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).Set(value(f))
		return next
	}
}

func bitNot(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	next := getExec(n.tnext)
	typ := n.typ.TypeOf()

	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n.exec = func(f *frame) bltn {
			dest(f).SetInt(^value(f).Int())
			return next
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n.exec = func(f *frame) bltn {
			dest(f).SetUint(^value(f).Uint())
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
	def := n.val.(*node)
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		switch t := def.typ.ret[i]; t.cat {
		case errorT:
			values[i] = genInterfaceWrapper(c, t.TypeOf())
		case aliasT:
			if isInterfaceSrc(t) {
				values[i] = genValueInterface(c)
			} else {
				values[i] = genValue(c)
			}
		case funcT:
			values[i] = genValue(c)
		case interfaceT:
			values[i] = genValueInterface(c)
		case valueT:
			if t.rtype.Kind() == reflect.Interface {
				values[i] = genInterfaceWrapper(c, t.rtype)
				break
			}
			fallthrough
		default:
			if c.typ.untyped {
				values[i] = genValueAs(c, def.typ.ret[i].TypeOf())
			} else {
				values[i] = genValue(c)
			}
		}
	}

	switch len(child) {
	case 0:
		n.exec = nil
	case 1:
		if child[0].kind == binaryExpr || isCall(child[0]) {
			n.exec = nil
		} else {
			v := values[0]
			n.exec = func(f *frame) bltn {
				f.data[0].Set(v(f))
				return nil
			}
		}
	case 2:
		v0, v1 := values[0], values[1]
		n.exec = func(f *frame) bltn {
			f.data[0].Set(v0(f))
			f.data[1].Set(v1(f))
			return nil
		}
	default:
		n.exec = func(f *frame) bltn {
			for i, value := range values {
				f.data[i].Set(value(f))
			}
			return nil
		}
	}
}

func arrayLit(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if n.nleft == 1 {
		child = n.child[1:]
	}

	values := make([]func(*frame) reflect.Value, len(child))
	index := make([]int, len(child))
	rtype := n.typ.val.TypeOf()
	var max, prev int

	for i, c := range child {
		if c.kind == keyValueExpr {
			convertLiteralValue(c.child[1], rtype)
			if n.typ.val.cat == interfaceT {
				values[i] = genValueInterface(c.child[1])
			} else {
				values[i] = genValue(c.child[1])
			}
			index[i] = int(vInt(c.child[0].rval))
		} else {
			convertLiteralValue(c, rtype)
			if n.typ.val.cat == interfaceT {
				values[i] = genValueInterface(c)
			} else {
				values[i] = genValue(c)
			}
			index[i] = prev
		}
		prev = index[i] + 1
		if prev > max {
			max = prev
		}
	}

	typ := n.typ.frameType()
	n.exec = func(f *frame) bltn {
		var a reflect.Value
		if n.typ.sizedef {
			a, _ = n.typ.zero()
		} else {
			a = reflect.MakeSlice(typ, max, max)
		}
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
	if n.nleft == 1 {
		child = n.child[1:]
	}
	typ := n.typ.TypeOf()
	keys := make([]func(*frame) reflect.Value, len(child))
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c.child[0], n.typ.key.TypeOf())
		convertLiteralValue(c.child[1], n.typ.val.TypeOf())
		if n.typ.key.cat == interfaceT {
			keys[i] = genValueInterface(c.child[0])
		} else {
			keys[i] = genValue(c.child[0])
		}
		if n.typ.val.cat == interfaceT {
			values[i] = genValueInterface(c.child[1])
		} else {
			values[i] = genValue(c.child[1])
		}
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
	if n.nleft == 1 {
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

// compositeBinStruct creates and populates a struct object from a binary type.
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
			if c.typ.cat == funcT {
				convertLiteralValue(c.child[1], typ.Field(i).Type)
				values[i] = genFunctionWrapper(c.child[1])
			} else {
				convertLiteralValue(c, typ.Field(i).Type)
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

func destType(n *node) *itype {
	switch n.anc.kind {
	case assignStmt, defineStmt:
		return n.anc.child[0].typ
	default:
		return n.typ
	}
}

// doCompositeLit creates and populates a struct object.
func doCompositeLit(n *node, hasType bool) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	typ := n.typ
	if typ.cat == ptrT || typ.cat == aliasT {
		typ = typ.val
	}
	var mu sync.Mutex
	typ.mu = &mu
	child := n.child
	if hasType {
		child = n.child[1:]
	}
	destInterface := destType(n).cat == interfaceT

	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c, typ.field[i].typ.TypeOf())
		switch {
		case c.typ.cat == funcT:
			values[i] = genFunctionWrapper(c)
		case isArray(c.typ) && c.typ.val != nil && c.typ.val.cat == interfaceT:
			values[i] = genValueInterfaceArray(c)
		case isRecursiveType(typ.field[i].typ, typ.field[i].typ.rtype):
			values[i] = genValueRecursiveInterface(c, typ.field[i].typ.rtype)
		case isInterface(typ.field[i].typ):
			values[i] = genInterfaceWrapper(c, typ.field[i].typ.rtype)
		default:
			values[i] = genValue(c)
		}
	}

	i := n.findex
	l := n.level
	n.exec = func(f *frame) bltn {
		// TODO: it seems fishy that the typ might be modified post-compilation, and
		// hence that several goroutines might be using the same typ that they all modify.
		// We probably need to revisit that.
		typ.mu.Lock()
		a := reflect.New(typ.TypeOf()).Elem()
		typ.mu.Unlock()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		switch d := value(f); {
		case d.Type().Kind() == reflect.Ptr:
			d.Set(a.Addr())
		case destInterface:
			d.Set(reflect.ValueOf(valueInterface{n, a}))
		default:
			getFrame(f, l).data[i] = a
		}
		return next
	}
}

func compositeLit(n *node)       { doCompositeLit(n, true) }
func compositeLitNotype(n *node) { doCompositeLit(n, false) }

// doCompositeSparse creates a struct Object, filling fields from sparse key-values.
func doCompositeSparse(n *node, hasType bool) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	typ := n.typ
	if typ.cat == ptrT || typ.cat == aliasT {
		typ = typ.val
	}
	var mu sync.Mutex
	typ.mu = &mu
	child := n.child
	if hasType {
		child = n.child[1:]
	}
	destInterface := destType(n).cat == interfaceT

	values := make(map[int]func(*frame) reflect.Value)
	for _, c := range child {
		c1 := c.child[1]
		field := typ.fieldIndex(c.child[0].ident)
		convertLiteralValue(c1, typ.field[field].typ.TypeOf())
		switch {
		case c1.typ.cat == funcT:
			values[field] = genFunctionWrapper(c1)
		case isArray(c1.typ) && c1.typ.val != nil && c1.typ.val.cat == interfaceT:
			values[field] = genValueInterfaceArray(c1)
		case isRecursiveType(typ.field[field].typ, typ.field[field].typ.rtype):
			values[field] = genValueRecursiveInterface(c1, typ.field[field].typ.rtype)
		case isInterface(typ.field[field].typ):
			values[field] = genInterfaceWrapper(c1, typ.field[field].typ.rtype)
		default:
			values[field] = genValue(c1)
		}
	}

	n.exec = func(f *frame) bltn {
		typ.mu.Lock()
		a, _ := typ.zero()
		typ.mu.Unlock()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		d := value(f)
		switch {
		case d.Type().Kind() == reflect.Ptr:
			d.Set(a.Addr())
		case destInterface:
			d.Set(reflect.ValueOf(valueInterface{n, a}))
		default:
			d.Set(a)
		}
		return next
	}
}

func compositeSparse(n *node)       { doCompositeSparse(n, true) }
func compositeSparseNotype(n *node) { doCompositeSparse(n, false) }

func empty(n *node) {}

var rat = reflect.ValueOf((*[]rune)(nil)).Type().Elem() // runes array type

func _range(n *node) {
	index0 := n.child[0].findex // array index location in frame
	index2 := index0 - 1        // shallow array for range, always just behind index0
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	var value func(*frame) reflect.Value
	if len(n.child) == 4 {
		an := n.child[2]
		index1 := n.child[1].findex // array value location in frame
		if isString(an.typ.TypeOf()) {
			value = genValueAs(an, rat) // range on string iterates over runes
		} else {
			value = genValueRangeArray(an)
		}
		n.exec = func(f *frame) bltn {
			a := f.data[index2]
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
		an := n.child[1]
		if isString(an.typ.TypeOf()) {
			value = genValueAs(an, rat) // range on string iterates over runes
		} else {
			value = genValueRangeArray(an)
		}
		n.exec = func(f *frame) bltn {
			v0 := f.data[index0]
			v0.SetInt(v0.Int() + 1)
			if int(v0.Int()) >= f.data[index2].Len() {
				return fnext
			}
			return tnext
		}
	}

	// Init sequence
	next := n.exec
	n.child[0].exec = func(f *frame) bltn {
		f.data[index2] = value(f) // set array shallow copy for range
		f.data[index0].SetInt(-1) // assing index value
		return next
	}
}

func rangeChan(n *node) {
	i := n.child[0].findex        // element index location in frame
	value := genValue(n.child[1]) // chan
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		f.mutex.RLock()
		done := f.done
		f.mutex.RUnlock()

		chosen, v, ok := reflect.Select([]reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: value(f)}})
		if chosen == 0 {
			return nil
		}
		if !ok {
			return fnext
		}
		f.data[i].Set(v)
		return tnext
	}
}

func rangeMap(n *node) {
	index0 := n.child[0].findex // map index location in frame
	index2 := index0 - 1        // iterator for range, always just behind index0
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	var value func(*frame) reflect.Value
	if len(n.child) == 4 {
		index1 := n.child[1].findex  // map value location in frame
		value = genValue(n.child[2]) // map
		if n.child[1].typ.cat == interfaceT {
			n.exec = func(f *frame) bltn {
				iter := f.data[index2].Interface().(*reflect.MapIter)
				if !iter.Next() {
					return fnext
				}
				f.data[index0].Set(iter.Key())
				if e := iter.Value().Elem(); e.Type().AssignableTo(valueInterfaceType) {
					f.data[index1].Set(e)
				} else {
					f.data[index1].Set(reflect.ValueOf(valueInterface{n, e}))
				}
				return tnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				iter := f.data[index2].Interface().(*reflect.MapIter)
				if !iter.Next() {
					return fnext
				}
				f.data[index0].Set(iter.Key())
				f.data[index1].Set(iter.Value())
				return tnext
			}
		}
	} else {
		value = genValue(n.child[1]) // map
		n.exec = func(f *frame) bltn {
			iter := f.data[index2].Interface().(*reflect.MapIter)
			if !iter.Next() {
				return fnext
			}
			f.data[index0].Set(iter.Key())
			return tnext
		}
	}

	// Init sequence
	next := n.exec
	n.child[0].exec = func(f *frame) bltn {
		f.data[index2].Set(reflect.ValueOf(value(f).MapRange()))
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
					if v := srcValue(f).Interface().(valueInterface).node; v != nil {
						for _, typ := range types {
							if v.typ.id() == typ.id() {
								destValue(f).Set(val)
								return tnext
							}
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
					if v := srcValue(f).Interface().(valueInterface).node; v != nil {
						for _, typ := range types {
							if v.typ.id() == typ.id() {
								return tnext
							}
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
			v0 := value(f)
			for _, v := range values {
				v1 := v(f)
				if !v0.Type().AssignableTo(v1.Type()) {
					v0 = v0.Convert(v1.Type())
				}
				if v0.Interface() == v1.Interface() {
					return tnext
				}
			}
			return fnext
		}
	}
}

func appendSlice(n *node) {
	dest := genValueOutput(n, n.typ.rtype)
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
	if c1, c2 := n.child[1], n.child[2]; len(n.child) == 3 && c2.typ.cat == arrayT && c2.typ.val.id() == n.typ.val.id() ||
		isByteArray(c1.typ.TypeOf()) && isString(c2.typ.TypeOf()) {
		appendSlice(n)
		return
	}
	dest := genValueOutput(n, n.typ.rtype)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	if len(n.child) > 3 {
		args := n.child[2:]
		l := len(args)
		values := make([]func(*frame) reflect.Value, l)
		for i, arg := range args {
			switch {
			case n.typ.val.cat == interfaceT:
				values[i] = genValueInterface(arg)
			case isRecursiveType(n.typ.val, n.typ.val.rtype):
				values[i] = genValueRecursiveInterface(arg, n.typ.val.rtype)
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
		case n.typ.val.cat == interfaceT:
			value0 = genValueInterface(n.child[2])
		case isRecursiveType(n.typ.val, n.typ.val.rtype):
			value0 = genValueRecursiveInterface(n.child[2], n.typ.val.rtype)
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
	dest := genValueOutput(n, reflect.TypeOf(int(0)))
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).SetInt(int64(value(f).Cap()))
		return next
	}
}

func _copy(n *node) {
	in := []func(*frame) reflect.Value{genValueArray(n.child[1]), genValue(n.child[2])}
	out := []func(*frame) reflect.Value{genValueOutput(n, reflect.TypeOf(0))}

	genBuiltinDeferWrapper(n, in, out, func(args []reflect.Value) []reflect.Value {
		cnt := reflect.Copy(args[0], args[1])
		return []reflect.Value{reflect.ValueOf(cnt)}
	})
}

func _close(n *node) {
	in := []func(*frame) reflect.Value{genValue(n.child[1])}

	genBuiltinDeferWrapper(n, in, nil, func(args []reflect.Value) []reflect.Value {
		args[0].Close()
		return nil
	})
}

func _complex(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(complex(0, 0)))
	c1, c2 := n.child[1], n.child[2]
	convertLiteralValue(c1, floatType)
	convertLiteralValue(c2, floatType)
	value0 := genValue(c1)
	value1 := genValue(c2)
	next := getExec(n.tnext)

	if typ := n.typ.TypeOf(); isComplex(typ) {
		n.exec = func(f *frame) bltn {
			dest(f).SetComplex(complex(value0(f).Float(), value1(f).Float()))
			return next
		}
	} else {
		// Not a complex type: ignore imaginary part
		n.exec = func(f *frame) bltn {
			dest(f).Set(value0(f).Convert(typ))
			return next
		}
	}
}

func _imag(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(float64(0)))
	convertLiteralValue(n.child[1], complexType)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).SetFloat(imag(value(f).Complex()))
		return next
	}
}

func _real(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(float64(0)))
	convertLiteralValue(n.child[1], complexType)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).SetFloat(real(value(f).Complex()))
		return next
	}
}

func _delete(n *node) {
	value0 := genValue(n.child[1]) // map
	value1 := genValue(n.child[2]) // key
	in := []func(*frame) reflect.Value{value0, value1}
	var z reflect.Value

	genBuiltinDeferWrapper(n, in, nil, func(args []reflect.Value) []reflect.Value {
		args[0].SetMapIndex(args[1], z)
		return nil
	})
}

func _len(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(int(0)))
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).SetInt(int64(value(f).Len()))
		return next
	}
}

func _new(n *node) {
	next := getExec(n.tnext)
	typ := n.child[1].typ.TypeOf()
	dest := genValueOutput(n, reflect.PtrTo(typ))

	n.exec = func(f *frame) bltn {
		dest(f).Set(reflect.New(typ))
		return next
	}
}

// _make allocates and initializes a slice, a map or a chan.
func _make(n *node) {
	next := getExec(n.tnext)
	typ := n.child[1].typ.frameType()
	dest := genValueOutput(n, typ)

	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		value := genValue(n.child[2])

		switch len(n.child) {
		case 3:
			n.exec = func(f *frame) bltn {
				len := int(vInt(value(f)))
				dest(f).Set(reflect.MakeSlice(typ, len, len))
				return next
			}
		case 4:
			value1 := genValue(n.child[3])
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.MakeSlice(typ, int(vInt(value(f))), int(vInt(value1(f)))))
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
				dest(f).Set(reflect.MakeChan(typ, int(vInt(value(f)))))
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
				dest(f).Set(reflect.MakeMapWithSize(typ, int(vInt(value(f)))))
				return next
			}
		}
	}
}

func reset(n *node) {
	next := getExec(n.tnext)

	switch l := len(n.child) - 1; l {
	case 1:
		typ := n.child[0].typ.frameType()
		i := n.child[0].findex
		n.exec = func(f *frame) bltn {
			f.data[i] = reflect.New(typ).Elem()
			return next
		}
	case 2:
		c0, c1 := n.child[0], n.child[1]
		i0, i1 := c0.findex, c1.findex
		t0, t1 := c0.typ.frameType(), c1.typ.frameType()
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
			types[i] = c.typ.frameType()
		}
		n.exec = func(f *frame) bltn {
			for i, ind := range index {
				f.data[ind] = reflect.New(types[i]).Elem()
			}
			return next
		}
	}
}

// recv reads from a channel.
func recv(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)
	i := n.findex
	l := n.level

	if n.interp.cancelChan {
		// Cancellable channel read
		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				// Fast: channel read doesn't block
				ch := value(f)
				if r, ok := ch.TryRecv(); ok {
					getFrame(f, l).data[i] = r
					if r.Bool() {
						return tnext
					}
					return fnext
				}
				// Slow: channel read blocks, allow cancel
				f.mutex.RLock()
				done := f.done
				f.mutex.RUnlock()

				chosen, v, _ := reflect.Select([]reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: ch}})
				if chosen == 0 {
					return nil
				}
				if v.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				// Fast: channel read doesn't block
				ch := value(f)
				if r, ok := ch.TryRecv(); ok {
					getFrame(f, l).data[i] = r
					return tnext
				}
				// Slow: channel is blocked, allow cancel
				f.mutex.RLock()
				done := f.done
				f.mutex.RUnlock()

				var chosen int
				chosen, getFrame(f, l).data[i], _ = reflect.Select([]reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: ch}})
				if chosen == 0 {
					return nil
				}
				return tnext
			}
		}
	} else {
		// Blocking channel read (less overhead)
		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if r, _ := value(f).Recv(); r.Bool() {
					getFrame(f, l).data[i] = r
					return tnext
				}
				return fnext
			}
		} else {
			i := n.findex
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i], _ = value(f).Recv()
				return tnext
			}
		}
	}
}

func recv2(n *node) {
	vchan := genValue(n.child[0])    // chan
	vres := genValue(n.anc.child[0]) // result
	vok := genValue(n.anc.child[1])  // status
	tnext := getExec(n.tnext)

	if n.interp.cancelChan {
		// Cancellable channel read
		n.exec = func(f *frame) bltn {
			ch, result, status := vchan(f), vres(f), vok(f)
			//  Fast: channel read doesn't block
			if v, ok := ch.TryRecv(); ok {
				result.Set(v)
				status.SetBool(true)
				return tnext
			}
			// Slow: channel is blocked, allow cancel
			f.mutex.RLock()
			done := f.done
			f.mutex.RUnlock()

			chosen, v, ok := reflect.Select([]reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: ch}})
			if chosen == 0 {
				return nil
			}
			result.Set(v)
			status.SetBool(ok)
			return tnext
		}
	} else {
		// Blocking channel read (less overhead)
		n.exec = func(f *frame) bltn {
			v, ok := vchan(f).Recv()
			vres(f).Set(v)
			vok(f).SetBool(ok)
			return tnext
		}
	}
}

func convertLiteralValue(n *node, t reflect.Type) {
	switch {
	case n.typ.cat == nilT:
		// Create a zero value of target type.
		n.rval = reflect.New(t).Elem()
	case !(n.kind == basicLit || n.rval.IsValid()) || t == nil || t.Kind() == reflect.Interface || t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Interface:
		// Skip non-constant values, undefined target type or interface target type.
	case n.rval.IsValid():
		// Convert constant value to target type.
		convertConstantValue(n)
		n.rval = n.rval.Convert(t)
	default:
		// Create a zero value of target type.
		n.rval = reflect.New(t).Elem()
	}
}

func convertConstantValue(n *node) {
	if !n.rval.IsValid() {
		return
	}
	c, ok := n.rval.Interface().(constant.Value)
	if !ok {
		return
	}

	v := n.rval
	typ := n.typ.TypeOf()
	kind := typ.Kind()
	switch kind {
	case reflect.Bool:
		v = reflect.ValueOf(constant.BoolVal(c)).Convert(typ)
	case reflect.String:
		v = reflect.ValueOf(constant.StringVal(c)).Convert(typ)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, _ := constant.Int64Val(c)
		l := constant.BitLen(c)
		if l > bitlen[kind] {
			panic(fmt.Sprintf("constant %s overflows int%d", c.ExactString(), bitlen[kind]))
		}
		v = reflect.ValueOf(i).Convert(typ)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		i, _ := constant.Uint64Val(c)
		l := constant.BitLen(c)
		if l > bitlen[kind] {
			panic(fmt.Sprintf("constant %s overflows uint%d", c.ExactString(), bitlen[kind]))
		}
		v = reflect.ValueOf(i).Convert(typ)
	case reflect.Float32:
		f, _ := constant.Float32Val(c)
		v = reflect.ValueOf(f).Convert(typ)
	case reflect.Float64:
		f, _ := constant.Float64Val(c)
		v = reflect.ValueOf(f).Convert(typ)
	case reflect.Complex64:
		r, _ := constant.Float32Val(constant.Real(c))
		i, _ := constant.Float32Val(constant.Imag(c))
		v = reflect.ValueOf(complex(r, i)).Convert(typ)
	case reflect.Complex128:
		r, _ := constant.Float64Val(constant.Real(c))
		i, _ := constant.Float64Val(constant.Imag(c))
		v = reflect.ValueOf(complex(r, i)).Convert(typ)
	default:
		// Type kind is from internal constant representation. Only use default types here.
		switch c.Kind() {
		case constant.Bool:
			v = reflect.ValueOf(constant.BoolVal(c))
		case constant.String:
			v = reflect.ValueOf(constant.StringVal(c))
		case constant.Int:
			i, x := constant.Int64Val(c)
			if !x {
				panic(fmt.Sprintf("constant %s overflows int64", c.ExactString()))
			}
			v = reflect.ValueOf(int(i))
		case constant.Float:
			f, _ := constant.Float64Val(c)
			v = reflect.ValueOf(f)
		case constant.Complex:
			r, _ := constant.Float64Val(constant.Real(c))
			i, _ := constant.Float64Val(constant.Imag(c))
			v = reflect.ValueOf(complex(r, i))
		}
	}
	n.rval = v
}

// Write to a channel.
func send(n *node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0]) // channel
	convertLiteralValue(n.child[1], n.child[0].typ.val.TypeOf())
	value1 := genValue(n.child[1]) // value to send

	if n.interp.cancelChan {
		// Cancellable send
		n.exec = func(f *frame) bltn {
			ch, data := value0(f), value1(f)
			// Fast: send on channel doesn't block
			if ok := ch.TrySend(data); ok {
				return next
			}
			// Slow: send on channel blocks, allow cancel
			f.mutex.RLock()
			done := f.done
			f.mutex.RUnlock()

			chosen, _, _ := reflect.Select([]reflect.SelectCase{done, {Dir: reflect.SelectSend, Chan: ch, Send: data}})
			if chosen == 0 {
				return nil
			}
			return next
		}
	} else {
		// Blocking send (less overhead)
		n.exec = func(f *frame) bltn {
			value0(f).Send(value1(f))
			return next
		}
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
	cases := make([]reflect.SelectCase, nbClause+1)
	next := getExec(n.tnext)

	for i := 0; i < nbClause; i++ {
		if len(n.child[i].child) == 0 {
			// The comm clause is an empty default, exit select.
			cases[i].Dir = reflect.SelectDefault
			clause[i] = func(*frame) bltn { return next }
		} else {
			switch c0 := n.child[i].child[0]; {
			case len(n.child[i].child) > 1:
				// The comm clause contains a channel operation and a clause body.
				clause[i] = getExec(n.child[i].child[1].start)
				chans[i], assigned[i], ok[i], cases[i].Dir = clauseChanDir(n.child[i])
				chanValues[i] = genValue(chans[i])
				if assigned[i] != nil {
					assignedValues[i] = genValue(assigned[i])
				}
				if ok[i] != nil {
					okValues[i] = genValue(ok[i])
				}
			case c0.kind == exprStmt && len(c0.child) == 1 && c0.child[0].action == aRecv:
				// The comm clause has an empty body clause after channel receive.
				chanValues[i] = genValue(c0.child[0].child[0])
				cases[i].Dir = reflect.SelectRecv
			case c0.kind == sendStmt:
				// The comm clause as an empty body clause after channel send.
				chanValues[i] = genValue(c0.child[0])
				cases[i].Dir = reflect.SelectSend
				assignedValues[i] = genValue(c0.child[1])
			default:
				// The comm clause has a default clause.
				clause[i] = getExec(c0.start)
				cases[i].Dir = reflect.SelectDefault
			}
		}
	}

	n.exec = func(f *frame) bltn {
		f.mutex.RLock()
		cases[nbClause] = f.done
		f.mutex.RUnlock()

		for i := range cases[:nbClause] {
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
		if j == nbClause {
			return nil
		}
		if cases[j].Dir == reflect.SelectRecv && assignedValues[j] != nil {
			assignedValues[j](f).Set(v)
			if ok[j] != nil {
				okValues[j](f).SetBool(s)
			}
		}
		return clause[j]
	}
}

// slice expression: array[low:high:max].
func slice(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)
	value0 := genValueArray(n.child[0]) // array
	value1 := genValue(n.child[1])      // low (if 2 or 3 args) or high (if 1 arg)

	switch len(n.child) {
	case 2:
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(int(vInt(value1(f))), a.Len())
			return next
		}
	case 3:
		value2 := genValue(n.child[2]) // max

		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(int(vInt(value1(f))), int(vInt(value2(f))))
			return next
		}
	case 4:
		value2 := genValue(n.child[2])
		value3 := genValue(n.child[3])

		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice3(int(vInt(value1(f))), int(vInt(value2(f))), int(vInt(value3(f))))
			return next
		}
	}
}

// slice expression, no low value: array[:high:max].
func slice0(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)
	value0 := genValueArray(n.child[0])

	switch len(n.child) {
	case 1:
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(0, a.Len())
			return next
		}
	case 2:
		value1 := genValue(n.child[1])
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(0, int(vInt(value1(f))))
			return next
		}
	case 3:
		value1 := genValue(n.child[1])
		value2 := genValue(n.child[2])
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice3(0, int(vInt(value1(f))), int(vInt(value2(f))))
			return next
		}
	}
}

func isNil(n *node) {
	var value func(*frame) reflect.Value
	c0 := n.child[0]
	if c0.typ.cat == funcT {
		value = genValueAsFunctionWrapper(c0)
	} else {
		value = genValue(c0)
	}
	tnext := getExec(n.tnext)
	dest := genValue(n)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		if c0.typ.cat == interfaceT {
			n.exec = func(f *frame) bltn {
				vi := value(f).Interface().(valueInterface)
				if (vi == valueInterface{} ||
					vi.node.kind == basicLit && vi.node.typ.cat == nilT) {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).SetBool(false)
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				if value(f).IsNil() {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).SetBool(false)
				return fnext
			}
		}
	} else {
		if c0.typ.cat == interfaceT {
			n.exec = func(f *frame) bltn {
				dest(f).SetBool(value(f).Interface().(valueInterface) == valueInterface{})
				return tnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				dest(f).SetBool(value(f).IsNil())
				return tnext
			}
		}
	}
}

func isNotNil(n *node) {
	var value func(*frame) reflect.Value
	c0 := n.child[0]
	if c0.typ.cat == funcT {
		value = genValueAsFunctionWrapper(c0)
	} else {
		value = genValue(c0)
	}
	tnext := getExec(n.tnext)
	dest := genValue(n)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		if c0.typ.cat == interfaceT {
			n.exec = func(f *frame) bltn {
				vi := value(f).Interface().(valueInterface)
				if (vi == valueInterface{} ||
					vi.node.kind == basicLit && vi.node.typ.cat == nilT) {
					dest(f).SetBool(false)
					return fnext
				}
				dest(f).SetBool(true)
				return tnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				if value(f).IsNil() {
					dest(f).SetBool(false)
					return fnext
				}
				dest(f).SetBool(true)
				return tnext
			}
		}
	} else {
		if c0.typ.cat == interfaceT {
			n.exec = func(f *frame) bltn {
				dest(f).SetBool(!(value(f).Interface().(valueInterface) == valueInterface{}))
				return tnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				dest(f).SetBool(!value(f).IsNil())
				return tnext
			}
		}
	}
}

func complexConst(n *node) {
	if v0, v1 := n.child[1].rval, n.child[2].rval; v0.IsValid() && v1.IsValid() {
		n.rval = reflect.ValueOf(complex(vFloat(v0), vFloat(v1)))
		n.gen = nop
	}
}

func imagConst(n *node) {
	if v := n.child[1].rval; v.IsValid() {
		n.rval = reflect.ValueOf(imag(v.Complex()))
		n.gen = nop
	}
}

func realConst(n *node) {
	if v := n.child[1].rval; v.IsValid() {
		n.rval = reflect.ValueOf(real(v.Complex()))
		n.gen = nop
	}
}
