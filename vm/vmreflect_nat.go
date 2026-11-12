package vm

// Native values in the VM reflect.
//
// A reflect.Type or reflect.Value of the program can describe native data:
// a host object (*http.Request, the error of fmt.Errorf), or a native value
// stored in an interpreted variable. The VM intrinsics only know the VM
// representation, so these values stay on the twin path:
//
//   - reflect.ValueOf of a native value keeps the native data and a native
//     twin descriptor (vtype.isNat): reads and writes go to the host object
//     through native reflect, which also keeps its checks (Set of an
//     unexported field of a host object panics);
//   - the calls that receive a native descriptor, or a reflect.Value whose
//     cell holds native data (a field of type *http.Request of an
//     interpreted struct), are done by the twin;
//   - a reflect.Type of a native type whose go/types type is known is the
//     VM descriptor of that type, so type identity holds
//     (reflect.ValueOf(r).Elem().Type() == reflect.TypeOf(http.Request{})).

import (
	"fmt"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/ssa"
)

// intrinsic returns the implementation of a call to a reflect function: the
// VM one, or the twin one when an argument holds native data.
func (vr *vmReflect) intrinsic(f *ssa.Function, args []int32) func([]Value) Value {
	if o := f.Origin(); o != nil && o.Name() == "TypeAssert" && f.Signature.Recv() == nil {
		return vr.typeAssert(f, args)
	}
	impl := vr.intrinsic1(f, args)
	switch f.Name() {
	case "Pointer", "UnsafePointer", "MapRange":
		if f.Signature.Recv() != nil {
			return impl // the VM implementation handles native data
		}
	}
	type natCheck struct {
		reg  int32
		kind int // 1 reflect.Type, 2 reflect.Value
	}
	var checks []natCheck
	n := f.Signature.Params().Len()
	if f.Signature.Recv() != nil {
		n++
	}
	if n != len(args) {
		return impl
	}
	param := func(i int) types.Type {
		if r := f.Signature.Recv(); r != nil {
			if i == 0 {
				return r.Type()
			}
			i--
		}
		return f.Signature.Params().At(i).Type()
	}
	for i := range args {
		if k := reflectKindOf(param(i)); k != 0 {
			checks = append(checks, natCheck{args[i], k})
		}
	}
	if len(checks) == 0 {
		return impl
	}
	twin := vr.tryTwinIntrinsic(f, args)
	if twin == nil {
		return impl
	}
	// A value derived from a value obtained through an unexported field is
	// read-only too (Elem, Field, Index...): native reflect does not know
	// the read-only flag of the VM, so it is added to the result here.
	roRecv := f.Signature.Recv() != nil && checks[0].kind == 2 && checks[0].reg == args[0] &&
		f.Signature.Results().Len() == 1 && reflectKindOf(f.Signature.Results().At(0).Type()) == 2
	return func(regs []Value) Value {
		for _, c := range checks {
			if hasNatData(regs[c.reg], c.kind) {
				r := twin(regs)
				if roRecv && decV(regs[args[0]]).flag&flagRO != 0 {
					if s, ok := r.R.([]Value); ok && len(s) == 3 && s[0].R != nil {
						s[2].N |= flagRO
					}
				}
				return r
			}
		}
		return impl(regs)
	}
}

// reflectKindOf returns 1 for reflect.Type, 2 for reflect.Value, 0 for
// other types.
func reflectKindOf(t types.Type) int {
	if n, ok := types.Unalias(t).(*types.Named); ok && isReflectPkg(n.Obj().Pkg()) {
		switch n.Obj().Name() {
		case "Type":
			return 1
		case "Value":
			return 2
		}
	}
	return 0
}

// hasNatData reports whether v, a reflect.Type (k == 1) or a reflect.Value
// (k == 2) of the program, uses a native descriptor or holds native data.
func hasNatData(v Value, k int) bool {
	if k == 1 {
		vt := argType(v)
		return vt != nil && vt.isNat()
	}
	s, _ := v.R.([]Value)
	if len(s) < 3 {
		return false
	}
	t, _ := s[0].R.(*vtype)
	if t == nil {
		return false
	}
	if t.isNat() {
		return true
	}
	if t.kind == reflect.Interface {
		return false
	}
	p, _ := s[1].R.(*Value)
	if p == nil {
		return false
	}
	if ownShadow(p) != nil {
		return true
	}
	switch p.R.(type) {
	case reflect.Value, *byte, roByte, *shadowRef: // as RV.native and RV.Elem
		return true
	}
	return false
}

// tryTwinIntrinsic is twinIntrinsic, or nil when the twin has no such
// function.
func (vr *vmReflect) tryTwinIntrinsic(f *ssa.Function, args []int32) (fn func([]Value) Value) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*CompileError); !ok {
				panic(r) // a real bug, not a missing twin function
			}
			fn = nil
		}
	}()
	name := f.Name()
	if o := f.Origin(); o != nil {
		name = o.Name()
	}
	if recv := f.Signature.Recv(); recv != nil {
		rt := recv.Type()
		if p, ok := rt.(*types.Pointer); ok {
			rt = p.Elem()
		}
		n := rt.(*types.Named).Obj().Name()
		if n != "Value" {
			return nil
		}
		name = n + "." + name
	}
	return vr.twinIntrinsic(f, name, args)
}

// natTypeVal returns the program's reflect.Type of the native type rt: the
// VM descriptor of its go/types type when there is one, the native
// descriptor otherwise. The Type of an error of a routed call names an
// interpreted type (namedType, aoterrors.go): the program gets the
// descriptor of that type.
func (vr *vmReflect) natTypeVal(rt reflect.Type) Value {
	if nt, ok := rt.(namedType); ok && nt.gt != nil {
		return vr.typeVal(nt.gt)
	}
	if gt := vr.m.goTypeOf(rt); gt != nil {
		return vr.typeVal(gt)
	}
	return Value{R: &Iface{T: vr.typeRT, V: Value{R: natT(rt)}}}
}

// natValueOf returns the program's reflect.Value of the native value n: a
// native descriptor and the native data itself, not a copy (see the file
// comment).
func (vr *vmReflect) natValueOf(n reflect.Value) Value {
	return encV(natT(n.Type()), &Value{R: n}, 0)
}

// natValueOfIface is natValueOf for the native value held by an interface
// (reflect.ValueOf(x), Elem of an interface). Like in gc it cannot be set:
// the VM keeps a native value of an interface in an addressable
// reflect.Value, so a Field or Index of it would be settable, and a Set
// would change the value inside the interface without a panic. The value is
// copied into an unaddressable reflect.Value.
func (vr *vmReflect) natValueOfIface(n reflect.Value) Value {
	return vr.natValueOf(unaddrNat(n))
}

// unaddrNat returns n, or a copy of n that cannot be set when n is
// addressable.
func unaddrNat(n reflect.Value) reflect.Value {
	if n.CanAddr() && n.CanInterface() {
		return reflect.ValueOf(n.Interface())
	}
	return n
}

// ifaceCell returns a new cell that holds v, the value of an interface, for
// a reflect.Value that cannot be set (reflect.ValueOf(x), Elem of an
// interface). When v holds native data (an interpreted struct type whose
// value is a native struct), that data is copied into an unaddressable
// reflect.Value, see natValueOfIface.
func ifaceCell(v Value) *Value {
	c := new(Value)
	*c = v
	if r, ok := v.R.(reflect.Value); ok {
		c.R = unaddrNat(r)
	}
	return c
}

// rvTop is a reflect.Value of the program given as a direct argument of a
// native printer (printFuncs): fmt.Println(v). fmt has a special case for
// an argument of type reflect.Value: it prints the value that v holds. RV
// does not match that case, so rvTop does the same in its Format method.
//
// Only a direct argument gets it (topPrintArgs): fmt prints a
// reflect.Value inside a struct, a slice or behind a pointer with its
// String method ("<int Value>"), and RV has a String method but no Format
// method, so it prints like Go there.
type rvTop RV

func (v rvTop) Format(f fmt.State, verb rune) {
	format := fmt.FormatString(f, verb)
	x := RV(v)
	if x.t == nil {
		fmt.Fprintf(f, format, reflect.Value{})
		return
	}
	if n, ok := x.native(); ok {
		fmt.Fprintf(f, format, n)
		return
	}
	x.flag &^= flagRO // fmt prints unexported fields too
	fmt.Fprintf(f, format, x.Interface())
}

// topPrintArgs gives an rvTop for each direct argument of a native printer
// that is a reflect.Value of the program (in: the converted arguments, the
// last one the slice of a variadic function).
func (m *Machine) topPrintArgs(in []reflect.Value, vari bool) {
	if !m.progVR {
		return
	}
	for i, x := range in {
		if vari && i == len(in)-1 {
			if x.Kind() != reflect.Slice {
				continue
			}
			var cp reflect.Value // a copy: the slice may be shared
			for j := range x.Len() {
				e := x.Index(j)
				if e.Kind() != reflect.Interface || e.IsNil() || e.Elem().Type() != rvNative {
					continue
				}
				if !cp.IsValid() {
					cp = reflect.MakeSlice(x.Type(), x.Len(), x.Len())
					reflect.Copy(cp, x)
				}
				cp.Index(j).Set(reflect.ValueOf(rvTop(e.Elem().Interface().(RV))))
			}
			if cp.IsValid() {
				in[i] = cp
			}
			continue
		}
		if x.IsValid() && x.Type() == rvNative {
			in[i] = reflect.ValueOf(rvTop(x.Interface().(RV)))
		}
	}
}

// natTypeArg reports whether the argument of a reflect.Type method that
// takes a reflect.Type (Implements, AssignableTo, ConvertibleTo) has a
// native descriptor.
func natTypeArg(name string, regs []Value, args []int32) bool {
	switch name {
	case "Implements", "AssignableTo", "ConvertibleTo":
		return len(args) == 1 && hasNatData(regs[args[0]], 1)
	}
	return false
}

// typeAssert implements reflect.TypeAssert[T](v), which is
// v.Interface().(T): Value.Interface of the program (VM data or native
// data), then the type assertion of the VM.
func (vr *vmReflect) typeAssert(f *ssa.Function, args []int32) func([]Value) Value {
	m := vr.m
	pkg := f.Origin().Pkg
	iface := vr.intrinsic(f.Prog.LookupMethod(pkg.Pkg.Scope().Lookup("Value").Type(), pkg.Pkg, "Interface"), args)
	t := f.TypeArgs()[0]
	return func(regs []Value) Value {
		x := decV(regs[args[0]])
		if x.t == nil {
			panic(&reflect.ValueError{Method: "reflect.TypeAssert", Kind: reflect.Invalid})
		}
		if x.flag&flagRO != 0 {
			panic("reflect.TypeAssert: cannot return value obtained from unexported field or method")
		}
		if iv, _ := iface(regs).R.(*Iface); iv != nil {
			if v, ok := m.assertTo(iv, t); ok {
				return Tuple(v, Value{N: 1})
			}
		}
		return Tuple(m.zero(t), Value{})
	}
}
