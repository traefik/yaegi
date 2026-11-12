package vm

// Callback trampolines: an interpreted func given to native code (or to an
// AOT copy) as a func of a common signature (the less func of sort.Slice,
// the func of strings.FieldsFunc or strings.Map) becomes a Go closure that
// reads its arguments and writes its result without reflect. Other funcs
// become a reflect.MakeFunc (toNativeW), whose calls convert a
// []reflect.Value of arguments and of results at each call: sort.Slice of 64
// elements calls its less func about 500 times. VM_OPT=-callbacks turns it
// off.
//
// The closures do what the MakeFunc of toNativeW does, in the same order:
// nativeCallback, the call of the closure, the conversion of the result
// (toNative of an integer or a bool keeps the low bits, as here). They are
// recorded by noteClosureFunc like the MakeFunc ones, and closureOfFunc
// knows their code (callbackPCs), so a func that comes back to the VM is
// the interpreted closure again.

import (
	"go/types"
	"reflect"
)

func cbIntIntBool(m *Machine, cl *Closure, u *types.Signature, origin *account) func(int, int) bool {
	return func(i, j int) bool {
		defer m.nativeCallback(origin)()
		return m.callClosure(cl, []Value{{N: int64(i)}, {N: int64(j)}}, u).N != 0
	}
}

func cbIntBool(m *Machine, cl *Closure, u *types.Signature, origin *account) func(int) bool {
	return func(i int) bool {
		defer m.nativeCallback(origin)()
		return m.callClosure(cl, []Value{{N: int64(i)}}, u).N != 0
	}
}

func cbIntInt(m *Machine, cl *Closure, u *types.Signature, origin *account) func(int) int {
	return func(i int) int {
		defer m.nativeCallback(origin)()
		return int(m.callClosure(cl, []Value{{N: int64(i)}}, u).N)
	}
}

func cbRuneBool(m *Machine, cl *Closure, u *types.Signature, origin *account) func(rune) bool {
	return func(r rune) bool {
		defer m.nativeCallback(origin)()
		return m.callClosure(cl, []Value{{N: int64(r)}}, u).N != 0
	}
}

func cbRuneRune(m *Machine, cl *Closure, u *types.Signature, origin *account) func(rune) rune {
	return func(r rune) rune {
		defer m.nativeCallback(origin)()
		return rune(m.callClosure(cl, []Value{{N: int64(r)}}, u).N)
	}
}

// callbackSigs are the signatures that have a trampoline, by their native
// type: the go/types form that they need, and the maker of the closure.
// They are set in init: as package variables, they would make an
// initialization cycle (callClosure, ..., closureOfFunc, callbackPCs).
var (
	callbackSigs map[reflect.Type]callbackSig
	callbackPCs  map[uintptr]bool // the code pointers of their closures
)

type callbackSig struct {
	sig  *types.Signature
	make func(m *Machine, cl *Closure, u *types.Signature, origin *account) any
}

func init() {
	callbackSigs = map[reflect.Type]callbackSig{
		reflect.TypeFor[func(int, int) bool](): {basicSig([]types.BasicKind{types.Int, types.Int}, types.Bool),
			func(m *Machine, cl *Closure, u *types.Signature, o *account) any { return cbIntIntBool(m, cl, u, o) }},
		reflect.TypeFor[func(int) bool](): {basicSig([]types.BasicKind{types.Int}, types.Bool),
			func(m *Machine, cl *Closure, u *types.Signature, o *account) any { return cbIntBool(m, cl, u, o) }},
		reflect.TypeFor[func(int) int](): {basicSig([]types.BasicKind{types.Int}, types.Int),
			func(m *Machine, cl *Closure, u *types.Signature, o *account) any { return cbIntInt(m, cl, u, o) }},
		reflect.TypeFor[func(rune) bool](): {basicSig([]types.BasicKind{types.Int32}, types.Bool),
			func(m *Machine, cl *Closure, u *types.Signature, o *account) any { return cbRuneBool(m, cl, u, o) }},
		reflect.TypeFor[func(rune) rune](): {basicSig([]types.BasicKind{types.Int32}, types.Int32),
			func(m *Machine, cl *Closure, u *types.Signature, o *account) any { return cbRuneRune(m, cl, u, o) }},
	}
	callbackPCs = map[uintptr]bool{}
	for _, c := range callbackSigs {
		callbackPCs[reflect.ValueOf(c.make(nil, nil, nil, nil)).Pointer()] = true
	}
}

func basicSig(ps []types.BasicKind, r types.BasicKind) *types.Signature {
	vs := make([]*types.Var, len(ps))
	for i, k := range ps {
		vs[i] = types.NewParam(0, nil, "", types.Typ[k])
	}
	return types.NewSignatureType(nil, nil, nil, types.NewTuple(vs...), types.NewTuple(types.NewParam(0, nil, "", types.Typ[r])), false)
}

// valueCallback returns the trampoline of the interpreted func value v for
// the native func type rt, as argNat makes it for an AOT copy (the
// sort.Search of aot/sort): the same as its MakeFunc (retVM of each
// argument, callValue, argNat of the result), without reflect.
func (m *Machine) valueCallback(v Value, rt reflect.Type) (reflect.Value, bool) {
	if !optCallbacks {
		return reflect.Value{}, false
	}
	var f any
	switch rt {
	case reflect.TypeFor[func(int, int) bool]():
		f = func(i, j int) bool { return m.callValue(v, []Value{{N: int64(i)}, {N: int64(j)}}).N != 0 }
	case reflect.TypeFor[func(int) bool]():
		f = func(i int) bool { return m.callValue(v, []Value{{N: int64(i)}}).N != 0 }
	case reflect.TypeFor[func(int) int]():
		f = func(i int) int { return int(m.callValue(v, []Value{{N: int64(i)}}).N) }
	case reflect.TypeFor[func(rune) bool]():
		f = func(r rune) bool { return m.callValue(v, []Value{{N: int64(r)}}).N != 0 }
	case reflect.TypeFor[func(rune) rune]():
		f = func(r rune) rune { return rune(m.callValue(v, []Value{{N: int64(r)}}).N) }
	default:
		return reflect.Value{}, false
	}
	return reflect.ValueOf(f), true
}

// fastCallback returns the trampoline of the interpreted closure cl of
// signature u, for the native func type rt, when there is one.
func (m *Machine) fastCallback(cl *Closure, u *types.Signature, rt reflect.Type, origin *account) (reflect.Value, bool) {
	if !optCallbacks {
		return reflect.Value{}, false
	}
	c, ok := callbackSigs[rt]
	if !ok || !types.Identical(u, c.sig) {
		return reflect.Value{}, false // a named type in the signature: the general path
	}
	return reflect.ValueOf(c.make(m, cl, u, origin)), true
}
