package vm

// reflect called by the interpreted program, beyond the VM reflect.
//
// With the VM reflect enabled (AOT mode, -opaque), the plugin's own reflect
// calls are VM intrinsics (vmreflect.go) that cover what encoding/json, fmt,
// errors and sort needed when they were interpreted. Everything else falls
// back to the Go-level twin (twin_*.go), the complete reflect over VM values
// that the AOT copies use: arguments are converted to the twin's types
// (twinFromVM for reflect.Type and reflect.Value, the native boundary for
// the rest), the twin function or method runs, and its results come back
// in the VM encoding.

import (
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// twinFuncs are the package functions of the twin, by name (filled at
// init: they reach the compiler, which reads the map).
var twinFuncs = map[string]reflect.Value{}

func init() {
	for name, f := range map[string]reflect.Value{
		"Append": reflect.ValueOf(Append), "AppendSlice": reflect.ValueOf(AppendSlice),
		"ArrayOf": reflect.ValueOf(ArrayOf), "ChanOf": reflect.ValueOf(ChanOf), "Copy": reflect.ValueOf(Copy),
		"DeepEqual": reflect.ValueOf(DeepEqual), "FuncOf": reflect.ValueOf(FuncOf), "Indirect": reflect.ValueOf(Indirect),
		"MakeChan": reflect.ValueOf(MakeChan), "MakeFunc": reflect.ValueOf(MakeFunc), "MakeMap": reflect.ValueOf(MakeMap), "MakeMapWithSize": reflect.ValueOf(MakeMapWithSize),
		"MakeSlice": reflect.ValueOf(MakeSlice), "MapOf": reflect.ValueOf(MapOf), "New": reflect.ValueOf(New),
		"PointerTo": reflect.ValueOf(PointerTo), "PtrTo": reflect.ValueOf(PtrTo), "SliceOf": reflect.ValueOf(SliceOf),
		"StructOf": reflect.ValueOf(StructOf), "Swapper": reflect.ValueOf(Swapper), "TypeOf": reflect.ValueOf(TypeOf),
		"ValueOf": reflect.ValueOf(ValueOf), "VisibleFields": reflect.ValueOf(VisibleFields), "Zero": reflect.ValueOf(Zero),
	} {
		twinFuncs[name] = f
	}
}

var (
	structFieldNative = reflect.TypeFor[StructField]()
	methodNative      = reflect.TypeFor[RMethod]()
)

// twinIntrinsic returns the implementation of a reflect package function or
// reflect.Value method by the twin, or reports it as unsupported.
func (vr *vmReflect) twinIntrinsic(f *ssa.Function, name string, args []int32) func([]Value) Value {
	m := vr.m
	sig := f.Signature
	if method, ok := strings.CutPrefix(name, "Value."); ok {
		if _, ok := rvNative.MethodByName(method); !ok {
			unsupported("vm reflect: %s.%s", f.Pkg.Pkg.Path(), name)
		}
		return func(regs []Value) Value {
			recv, _ := m.twinFromVM(regs[args[0]], rvNative)
			return vr.twinCall(recv.MethodByName(method), sig, regs, args[1:])
		}
	}
	switch name {
	case "MakeFunc":
		return vr.makeFunc(sig, args)
	case "Select":
		return vr.selectFn(sig, args)
	}
	fn, ok := twinFuncs[name]
	if !ok {
		unsupported("vm reflect: %s.%s", f.Pkg.Pkg.Path(), name)
	}
	return func(regs []Value) Value { return vr.twinCall(fn, sig, regs, args) }
}

// makeFunc implements reflect.MakeFunc called by the program with an
// interpreted function: a deferred call of the made function passes its
// panic to that function, so that recover works in it as in gc.
func (vr *vmReflect) makeFunc(sig *types.Signature, args []int32) func([]Value) Value {
	m := vr.m
	implT := sig.Params().At(1).Type().Underlying().(*types.Signature)
	inT, outT := implT.Params().At(0).Type(), implT.Results().At(0).Type()
	rvsT := reflect.TypeFor[[]RV]()
	return func(regs []Value) Value {
		cl, _ := regs[args[1]].R.(*Closure)
		if cl == nil || cl.Fn == nil {
			return vr.twinCall(twinFuncs["MakeFunc"], sig, regs, args)
		}
		tv, _ := m.twinFromVM(regs[args[0]], rtNative)
		t := tv.Interface().(RT)
		if vt := vr.vmVT(t); vt != nil {
			t = vt // a VM func value: a deferred call can pass its panic
		}
		rv := makeFuncP(t, func(p *panicState, in []RV) []RV {
			defer m.resumeShadows(m.suspendShadows())
			res := m.callP(cl.Fn, []Value{vr.twinResult(reflect.ValueOf(in), inT)}, cl.Env, p)
			out, _ := vr.twinArg(res, outT, rvsT).Interface().([]RV)
			return out
		})
		return vr.twinResult(reflect.ValueOf(rv), sig.Results().At(0).Type())
	}
}

// selectFn implements reflect.Select called by the program, with the twin.
// As for the select statement (execSelect), a blocking wait in a call from
// the host ends when the account of the call stops.
func (vr *vmReflect) selectFn(sig *types.Signature, args []int32) func([]Value) Value {
	m := vr.m
	recvT := sig.Results().At(1).Type()
	return func(regs []Value) Value {
		vs, _ := regs[args[0]].R.([]Value)
		cases := make([]SelectCase, len(vs))
		blocking := true
		for i, v := range vs {
			fs, _ := v.R.([]Value) // reflect.SelectCase: Dir, Chan, Send
			if len(fs) != 3 {
				unsupported("vm reflect: reflect.Select with a case of %T", v.R)
			}
			cases[i].Dir = reflect.SelectDir(fs[0].N)
			blocking = blocking && cases[i].Dir != reflect.SelectDefault
			if x, ok := m.twinFromVM(fs[1], rvNative); ok {
				cases[i].Chan = x.Interface().(RV)
			}
			if x, ok := m.twinFromVM(fs[2], rvNative); ok {
				cases[i].Send = x.Interface().(RV)
			}
		}
		var cx *ctx
		if gs := lookupG(getg()); gs != nil {
			cx = gs.cur
		}
		var as, ms chan struct{}
		if blocking && cx != nil {
			as, ms = cx.stopChans()
		}
		chosen, r, ok := selectStop(cases, as, ms)
		if blocking && cx != nil {
			if chosen < 0 {
				cx.halted()
			}
			cx.blocked()
		}
		return Value{R: []Value{{N: int64(chosen)}, vr.twinResult(reflect.ValueOf(r), recvT), {N: b2i(ok)}}}
	}
}

// twinInvoke implements a reflect.Type method missing from the VM reflect
// (Method, MethodByName, Implements...) with the twin.
func (vr *vmReflect) twinInvoke(vt *vtype, name string, regs []Value, args []int32) Value {
	recv, _ := vr.m.twinFromVM(Value{R: vt}, rtNative)
	if vt != nil && vt.t != nil && vr.m.twinNamed(vt.t) != nil {
		// reflect.Type or reflect.Value: their native counterpart is the
		// twin's own type (RV, *vtype), whose fields are not those of
		// reflect. Keep the go/types descriptor: FieldByName("flag") of
		// reflect.Value is reflect's uintptr field (go-spew's bypass.go).
		recv = reflect.ValueOf(vt)
	}
	mv := recv.MethodByName(name)
	it, _ := vr.m.reflectNamed("Type").Underlying().(*types.Interface)
	if !mv.IsValid() || it == nil {
		unsupported("vm reflect: Type.%s", name)
	}
	for i := 0; i < it.NumMethods(); i++ {
		if it.Method(i).Name() == name {
			return vr.twinCall(mv, it.Method(i).Type().(*types.Signature), regs, args)
		}
	}
	unsupported("vm reflect: Type.%s", name)
	return Value{}
}

// twinCall calls the twin function fn with the VM arguments regs[args] of
// the reflect signature sig, and converts its results.
func (vr *vmReflect) twinCall(fn reflect.Value, sig *types.Signature, regs []Value, args []int32) Value {
	ft := fn.Type()
	in := make([]reflect.Value, len(args))
	for i := range args {
		in[i] = vr.twinArg(regs[args[i]], sig.Params().At(i).Type(), ft.In(i))
	}
	var out []reflect.Value
	if ft.IsVariadic() {
		out = fn.CallSlice(in)
	} else {
		out = fn.Call(in)
	}
	res := sig.Results()
	switch len(out) {
	case 0:
		return Value{}
	case 1:
		return vr.twinResult(out[0], res.At(0).Type())
	}
	vs := make([]Value, len(out))
	for i := range out {
		vs[i] = vr.twinResult(out[i], res.At(i).Type())
	}
	return Value{R: vs}
}

// twinArg converts a VM argument of the reflect type t to the twin type rt.
func (vr *vmReflect) twinArg(v Value, t types.Type, rt reflect.Type) reflect.Value {
	m := vr.m
	if x, ok := m.twinFromVM(v, rt); ok {
		return x
	}
	switch {
	case rt.Kind() == reflect.Slice && (rt.Elem() == rvNative || rt.Elem() == rtNative || rt.Elem() == structFieldNative):
		vs, _ := v.R.([]Value)
		et := t.Underlying().(*types.Slice).Elem()
		out := reflect.MakeSlice(rt, len(vs), len(vs))
		for i := range vs {
			out.Index(i).Set(vr.twinArg(vs[i], et, rt.Elem()))
		}
		return out
	case rt == anyType:
		// An `any` argument (DeepEqual, Swapper): the twin takes the
		// interpreted value boxed, as in AOT mode, and sees its type and
		// its cells. In default mode the native boundary gave a native
		// copy: DeepEqual(A{1}, B{1}) was true (two copies of struct {
		// X int }), and a slice compared with itself was a new copy.
		x := reflect.New(rt).Elem()
		if a := m.routeArg(v, newConv(nil)); a != nil {
			x.Set(reflect.ValueOf(a))
		}
		return x
	case rt == structFieldNative:
		fs, _ := v.R.([]Value) // reflect.StructField: Name, PkgPath, Type, Tag, Offset, Index, Anonymous
		sf := StructField{Name: valStr(fs[0]), PkgPath: valStr(fs[1]), Tag: reflect.StructTag(valStr(fs[3])), Offset: uintptr(fs[4].N), Anonymous: fs[6].N != 0}
		if t, ok := m.twinFromVM(fs[2], rtNative); ok {
			sf.Type = t.Interface().(RT)
		}
		idx, _ := fs[5].R.([]Value)
		for _, x := range idx {
			sf.Index = append(sf.Index, int(x.N))
		}
		return reflect.ValueOf(sf)
	}
	return m.toNativeAs(v, t, rt, newConv(nil))
}

// twinResult converts a twin result to the VM value of the reflect type t.
func (vr *vmReflect) twinResult(r reflect.Value, t types.Type) Value {
	m := vr.m
	if v, ok := m.twinToVM(r); ok {
		return v
	}
	switch {
	case r.Kind() == reflect.Slice && (r.Type().Elem() == rvNative || r.Type().Elem() == rtNative || r.Type().Elem() == structFieldNative):
		if r.IsNil() {
			return Value{}
		}
		et := t.Underlying().(*types.Slice).Elem()
		vs := make([]Value, r.Len())
		for i := range vs {
			vs[i] = vr.twinResult(r.Index(i), et)
		}
		return Value{R: vs}
	case r.Type() == structFieldNative:
		sf := r.Interface().(StructField)
		idx := make([]Value, len(sf.Index))
		for i, x := range sf.Index {
			idx[i] = Value{N: int64(x)}
		}
		typ, _ := m.twinToVM(reflect.ValueOf(sf.Type))
		return Value{R: []Value{{R: sf.Name}, {R: sf.PkgPath}, typ, {R: string(sf.Tag)}, {N: int64(sf.Offset)}, {R: idx}, {N: b2i(sf.Anonymous)}}}
	case r.Type() == methodNative:
		mt := r.Interface().(RMethod)
		typ, _ := m.twinToVM(reflect.ValueOf(mt.Type))
		fn, _ := m.twinToVM(reflect.ValueOf(mt.Func))
		return Value{R: []Value{{R: mt.Name}, {R: mt.PkgPath}, typ, fn, {N: int64(mt.Index)}}}
	}
	return m.fromNative(r, t)
}
