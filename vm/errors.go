package vm

// errors.Is, errors.As, errors.AsType and errors.Unwrap with interpreted
// errors.
//
// Passed to native errors code, an interpreted error is boxed in a wrapper
// (errorW...) that only has an Error method, and each call makes a new box:
// native == compared the boxes (errors.Is(e, e) was false for a value
// type), and the Is, As and Unwrap methods of interpreted types were not
// seen. errors.As also saw the target *T with T copied to an unnamed native
// type, and panicked. These functions are replaced by a walk of the error
// chain on VM values: VM equality, and the methods called on the dynamic
// type, interpreted or native. Native errors that wrap interpreted ones
// (fmt.Errorf with %w, errors.Join) give back the original VM value when
// they unwrap (fromNative of a vmBoxed).

import (
	"errors"
	"go/token"
	"go/types"
	"reflect"
)

// errMethod is one of the optional methods errors looks for: the interface
// that has it, for interpreted types and for native types.
type errMethod struct {
	name string
	it   *types.Interface
	rt   reflect.Type
}

var (
	errIfaceT = types.Universe.Lookup("error").Type()
	anyIfaceT = types.Universe.Lookup("any").Type()

	errIs = newErrMethod("Is", errIfaceT, types.Typ[types.Bool],
		reflect.TypeFor[interface{ Is(error) bool }]())
	errAs = newErrMethod("As", anyIfaceT, types.Typ[types.Bool],
		reflect.TypeFor[interface{ As(any) bool }]())
	errUnwrap = newErrMethod("Unwrap", nil, errIfaceT,
		reflect.TypeFor[interface{ Unwrap() error }]())
	errUnwrapAll = newErrMethod("Unwrap", nil, types.NewSlice(errIfaceT),
		reflect.TypeFor[interface{ Unwrap() []error }]())
)

func newErrMethod(name string, param, result types.Type, rt reflect.Type) *errMethod {
	var params *types.Tuple
	if param != nil {
		params = types.NewTuple(types.NewParam(token.NoPos, nil, "", param))
	}
	sig := types.NewSignatureType(nil, nil, nil, params, types.NewTuple(types.NewParam(token.NoPos, nil, "", result)), false)
	it := types.NewInterfaceType([]*types.Func{types.NewFunc(token.NoPos, nil, name, sig)}, nil).Complete()
	return &errMethod{name: name, it: it, rt: rt}
}

func (e *errMethod) sig() *types.Signature { return e.it.Method(0).Type().(*types.Signature) }

func init() {
	vmHooks["errors.Is"] = func(m *Machine, a []Value) Value {
		return Value{N: b2i(m.errorsIs(a[0], a[1]))}
	}
	vmHooks["errors.As"] = func(m *Machine, a []Value) Value {
		return Value{N: b2i(m.errorsAs(a[0], a[1]))}
	}
	vmHooks["errors.Unwrap"] = func(m *Machine, a []Value) Value {
		iv, _ := a[0].R.(*Iface)
		if iv == nil || !m.hasErrMethod(iv, errUnwrap) {
			return Value{}
		}
		return m.callErrMethod(iv, errUnwrap)
	}
}

// hasErrMethod reports whether the dynamic type of iv has the method e with
// its exact signature (Unwrap() error and Unwrap() []error differ).
func (m *Machine) hasErrMethod(iv *Iface, e *errMethod) bool {
	if iv.T.Native != nil {
		return iv.T.Native.Implements(e.rt)
	}
	if iv.T.T == nil || m.vr != nil && iv.T == m.vr.typeRT {
		return false
	}
	return m.implements(iv.T, e.it)
}

// callErrMethod calls the method e on iv, as an interface method call of
// interpreted code (OpInvoke).
func (m *Machine) callErrMethod(iv *Iface, e *errMethod, args ...Value) Value {
	ids := make([]int32, len(args))
	for i := range ids {
		ids[i] = int32(i)
	}
	if iv.T.Native != nil {
		return m.callNativeMethod(iv.V.R.(reflect.Value), e.name, e.sig(), args, ids)
	}
	fn := iv.T.Methods[e.name]
	if !m.isInterp(fn.ssa) {
		return m.callBodyless(fn.ssa, iv.V, args, ids)
	}
	return m.Call(fn, append([]Value{iv.V}, args...), nil)
}

// errorsIs implements errors.Is(err, target).
func (m *Machine) errorsIs(err, target Value) bool {
	eiv, _ := err.R.(*Iface)
	tiv, _ := target.R.(*Iface)
	if eiv == nil || tiv == nil {
		return eiv == tiv
	}
	return m.errIs(eiv, target, ifaceComparable(tiv))
}

func (m *Machine) errIs(iv *Iface, target Value, comparable bool) bool {
	return m.errWalk(iv, func(iv *Iface) bool {
		if comparable && m.equal(Value{R: iv}, target, anyIfaceT) {
			return true
		}
		return m.hasErrMethod(iv, errIs) && m.callErrMethod(iv, errIs, target).N != 0
	})
}

// errWalk calls visit on each error of the chain of iv, in the order of
// errors.Is and errors.As (depth first), until visit returns true.
//
// An error chain can be a cycle (an Unwrap method that returns its
// receiver). A compiled program then loops or dies with a stack overflow,
// but a plugin must stop with ErrBudget or ErrStackOverflow, as for an
// endless loop or recursion of interpreted code. So each visited error
// counts one step of the budget, and the walk uses its own stack (not the
// Go stack): each Unwrap() []error counts one frame, against MaxDepth.
func (m *Machine) errWalk(iv *Iface, visit func(*Iface) bool) bool {
	var cx *ctx
	maxDepth := m.MaxDepth
	if gs := lookupG(getg()); gs != nil && gs.cur != nil {
		cx = gs.cur
		maxDepth = cx.maxFrames - len(cx.frames)
	}
	type node struct {
		iv    *Iface
		depth int
	}
	stack := []node{{iv, 0}}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for iv := n.iv; iv != nil; {
			if cx != nil {
				cx.steps++
				if cx.steps >= cx.limit {
					cx.refill()
				}
			}
			if visit(iv) {
				return true
			}
			switch {
			case m.hasErrMethod(iv, errUnwrap):
				iv, _ = m.callErrMethod(iv, errUnwrap).R.(*Iface)
			case m.hasErrMethod(iv, errUnwrapAll):
				errs, _ := m.callErrMethod(iv, errUnwrapAll).R.([]Value)
				if n.depth+1 >= maxDepth {
					if cx != nil {
						cx.overflow()
					}
					panic(ErrStackOverflow)
				}
				// Pushed in reverse order: the first error is walked first.
				for i := len(errs) - 1; i >= 0; i-- {
					if eiv, _ := errs[i].R.(*Iface); eiv != nil {
						stack = append(stack, node{eiv, n.depth + 1})
					}
				}
				iv = nil
			default:
				iv = nil
			}
		}
	}
	return false
}

// ifaceComparable reports whether the dynamic type of iv is comparable.
func ifaceComparable(iv *Iface) bool {
	if iv.T.T != nil {
		return types.Comparable(iv.T.T)
	}
	return iv.T.Native.Comparable()
}

// errorsAs implements errors.As(err, target).
func (m *Machine) errorsAs(err, target Value) bool {
	eiv, _ := err.R.(*Iface)
	if eiv == nil {
		return false
	}
	tiv, _ := target.R.(*Iface)
	if tiv == nil {
		panic("errors: target cannot be nil")
	}
	if tiv.T.T == nil {
		// A native pointer that native code gave to interpreted code.
		nerr, _ := m.toNative(err, errIfaceT).Interface().(error)
		return errors.As(nerr, tiv.V.R.(reflect.Value).Interface())
	}
	pt, ok := tiv.T.T.Underlying().(*types.Pointer)
	if !ok || ptrIsNil(tiv.V) {
		panic("errors: target must be a non-nil pointer")
	}
	et := pt.Elem()
	if !types.IsInterface(et) && !types.Implements(et, errIfaceT.Underlying().(*types.Interface)) {
		panic("errors: *target must be interface or implement error")
	}
	return m.errAs(eiv, target, tiv.V, et)
}

func (m *Machine) errAs(iv *Iface, target, ptr Value, et types.Type) bool {
	return m.errWalk(iv, func(iv *Iface) bool {
		if v, ok := m.assertTo(iv, et); ok {
			m.store(ptr, v, newMemSite(et))
			return true
		}
		return m.hasErrMethod(iv, errAs) && m.callErrMethod(iv, errAs, target).N != 0
	})
}

// errorsAsType implements errors.AsType[E](err), with et the type E (a
// generic function of a native package: genericIntrinsic). The As method
// of an error gets a *E that points to a VM cell.
func (m *Machine) errorsAsType(err Value, et types.Type) Value {
	cell := new(Value)
	*cell = m.zero(et)
	if eiv, _ := err.R.(*Iface); eiv != nil {
		ptr := Value{R: cell}
		target := Value{R: &Iface{T: m.rtypeOf(types.NewPointer(et)), V: ptr}}
		if m.errAs(eiv, target, ptr, et) {
			return Tuple(*cell, Value{N: 1})
		}
	}
	return Tuple(m.zero(et), Value{})
}

// assertTo is the type assertion x.(t) of the interface value iv, as
// OpTypeAssert does it: the value of type t, and whether it holds.
func (m *Machine) assertTo(iv *Iface, t types.Type) (Value, bool) {
	if it, ok := t.Underlying().(*types.Interface); ok {
		return Value{R: iv}, m.implements(iv.T, it)
	}
	if iv.T == m.rtypeOf(t) {
		return iv.V, true
	}
	if iv.T.Native != nil {
		if nat := m.counterpart(t); nat != nil && iv.T.Native == nat {
			return m.fromNative(iv.V.R.(reflect.Value), t), true
		}
	}
	return Value{}, false
}
