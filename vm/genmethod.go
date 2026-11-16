package vm

// Generic methods (Go 1.27): a method can have type parameters of its own,
//
//	func (b Box[T]) Map[U any](f func(T) U) Box[U]
//
// go/types puts such a method in the method set of its type, but it is not
// a method at run time: no code exists for it until a call instantiates it
// (b.Map[string](f)), it does not satisfy the methods of an interface, and
// reflect does not list it (NumMethod, Method, MethodByName). Every place
// of the VM that walks a method set skips it with runtimeMethod, so they
// all agree.

import "go/types"

// isGenericMethod reports whether the method fn has type parameters of
// its own (a Go 1.27 generic method). The type parameters of a generic
// receiver do not count.
func isGenericMethod(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() != nil && sig.TypeParams().Len() > 0
}

// runtimeMethod reports whether the method of sel exists at run time: it
// is not a generic method (isGenericMethod), and its receiver is not a
// generic type that is not instantiated (no code exists for either).
func runtimeMethod(sel *types.Selection) bool {
	if fn, ok := sel.Obj().(*types.Func); ok && isGenericMethod(fn) {
		return false
	}
	recv := sel.Recv()
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	n, ok := types.Unalias(recv).(*types.Named)
	return !ok || n.TypeParams().Len() == 0 || n.TypeArgs().Len() > 0
}

// allTypeParams returns the type parameters of the function or method fn:
// those of its generic receiver first, then its own (a generic method of
// a generic type has both).
func allTypeParams(fn *types.Func) []*types.TypeParam {
	sig := fn.Type().(*types.Signature)
	var out []*types.TypeParam
	for _, l := range []*types.TypeParamList{sig.RecvTypeParams(), sig.TypeParams()} {
		for i := 0; i < l.Len(); i++ {
			out = append(out, l.At(i))
		}
	}
	return out
}
