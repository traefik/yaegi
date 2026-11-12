package vm

// Generic functions of AOT copies.
//
// An AOT copy is compiled Go: it has the instances of its generic functions
// that its own code needs, not the ones for the types of the program
// (json.MarshalFunc[celsius]). The glue of a copy can register a
// GenericHook for a generic function (cmd/aotgen, generics/): a version of
// the function that takes the type arguments as twin types (RT) and the
// values as AOT code sees them (any, as RV.Interface gives them).
// genericIntrinsic uses it when the package of the function runs in
// ModeAOT.

import (
	"go/types"
	"reflect"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// GenericHook implements the instances of a generic function of an AOT
// copy.
type GenericHook func(c *GenericCall) Value

// GenericCall is one call of a GenericHook.
type GenericCall struct {
	M     *Machine
	Types []RT    // the twin types of the type arguments
	Args  []Value // the VM values of the arguments
	targs []types.Type
	sig   *types.Signature // of the instance
}

var aotGenerics = map[string]GenericHook{}

// RegisterGeneric installs the hook of the generic function "pkgpath.Func"
// of an AOT copy (called by generated code).
func RegisterGeneric(name string, h GenericHook) {
	regMu.Lock()
	defer regMu.Unlock()
	aotGenerics[name] = h
}

// aotGeneric returns the hook of the instance f of a generic function, when
// its package runs in ModeAOT and its copy registered one.
func (m *Machine) aotGeneric(f *ssa.Function) GenericHook {
	o := f.Origin()
	if o == nil || o.Pkg == nil || m.vr == nil || m.modeOf(o.Pkg.Pkg.Path()) != ModeAOT {
		return nil
	}
	regMu.RLock()
	defer regMu.RUnlock()
	return aotGenerics[genericName(o)]
}

// aotGenericCall returns the implementation of the instance f with h.
func (m *Machine) aotGenericCall(h GenericHook, f *ssa.Function, args []int32) func([]Value) Value {
	targs := f.TypeArgs()
	rts := make([]RT, len(targs))
	for i, t := range targs {
		rts[i] = m.vr.vtypeOf(t).cn()
	}
	return func(regs []Value) Value {
		a := make([]Value, len(args))
		for i, r := range args {
			a[i] = regs[r]
		}
		return h(&GenericCall{M: m, Types: rts, Args: a, targs: targs, sig: f.Signature})
	}
}

// AOTValue converts the VM value v of type argument i to the value AOT code
// sees.
func (c *GenericCall) AOTValue(i int, v Value) any { return c.M.aotOfVM(v, c.targs[i]) }

// Zero returns the zero value of type argument i, as AOT code sees it.
func (c *GenericCall) Zero(i int) any { return c.AOTValue(i, c.M.zero(c.targs[i])) }

// VMValue converts x, a value of AOT code, to the VM value of type
// argument i.
func (c *GenericCall) VMValue(i int, x any) Value { return c.M.vmOfAOT(x, c.targs[i]) }

// VMOfRV converts rv, a twin value of type argument i, to its VM value:
// the same pointer for a pointer, a copy for a value.
func (c *GenericCall) VMOfRV(i int, rv RV) Value { return rv.vmVal(c.M, c.targs[i]) }

// ParamValue converts x, a value of AOT code, to the VM value of the type
// of parameter j of the function value of argument i.
func (c *GenericCall) ParamValue(i, j int, x any) Value {
	return c.M.vmOfAOT(x, c.funcSig(i).Params().At(j).Type())
}

func (c *GenericCall) funcSig(i int) *types.Signature {
	return c.sig.Params().At(i).Type().Underlying().(*types.Signature)
}

// Call calls the function value of argument i of the call with the VM
// values args, and returns its results.
func (c *GenericCall) Call(i int, args ...Value) []Value {
	m := c.M
	fn := c.Args[i]
	sig := c.funcSig(i)
	if fn.R == nil {
		panic(errNilDeref)
	}
	var r Value
	if rv, ok := fn.R.(reflect.Value); ok { // a function of a native or AOT package
		// Its own parameter types: a function of an AOT copy takes the
		// types of the copy (json.WithMarshalers takes *Marshalers, which
		// has a type of an internal package as a type argument).
		nin := make([]reflect.Value, len(args))
		for j := range args {
			nin[j] = m.argNat(args[j], rv.Type().In(j))
		}
		r = m.pack(rv.Call(nin), tupleTypes(sig.Results()))
	} else {
		r = m.callWithShadows(fn, args, sig)
	}
	if sig.Results().Len() == 1 {
		return []Value{r}
	}
	rs, _ := r.R.([]Value)
	return rs
}

// callWithShadows calls the function value fn of the program with args,
// of the types of sig, when AOT code may have given it native pointers into
// the memory of its shadows (shadowRef). callValue gives the shadowed cells
// their VM value during the call and copies them over the shadows after it
// (suspendShadows, resumeShadows): a write of the program through such a
// pointer was lost.
//
//   - A pointer that a shadowed pointer holds (AOT code allocated the value
//     for a nil pointer of the cell: json.Unmarshal into &p, p a nil *int):
//     the shadows are given back first, and the program gets the pointer
//     that the cell holds then.
//   - A pointer into the memory of a shadow (an element of an array whose
//     address AOT code took): the program gets a new VM cell with the value,
//     and the cell is written back through the pointer after the shadows
//     are attached again; the end of the AOT call gives the shadow to the
//     cell.
func (m *Machine) callWithShadows(fn Value, args []Value, sig *types.Signature) Value {
	type held struct {
		j int
		s *shadowRef
	}
	var hs []held
	var back []func()
	args = slices.Clone(args)
	for j, a := range args {
		p, ok := a.R.(reflect.Value)
		pt, isPtr := sig.Params().At(j).Type().Underlying().(*types.Pointer)
		if !ok || !isPtr || p.Kind() != reflect.Pointer || p.IsNil() {
			continue
		}
		s, inside := m.shadowReaching(p)
		switch {
		case s != nil && !inside:
			hs = append(hs, held{j, s})
		case s != nil:
			et := pt.Elem()
			cell := &Value{}
			*cell = m.natToVM(p.Elem(), et)
			args[j] = Value{R: cell}
			back = append(back, func() { p.Elem().Set(m.toNative(*cell, et)) })
		}
	}
	if len(hs) == 0 && len(back) == 0 {
		return m.callValue(fn, args)
	}
	r := func() Value {
		ss := m.suspendShadows()
		defer m.resumeShadows(ss)
		for _, h := range hs {
			args[h.j] = *h.s.cell
		}
		return m.callValue(fn, args)
	}()
	for _, wb := range back {
		wb()
	}
	return r
}

// aotOfVM converts the VM value v of type t to the value AOT code sees.
func (m *Machine) aotOfVM(v Value, t types.Type) any {
	if types.IsInterface(t) {
		iv, _ := v.R.(*Iface)
		if iv == nil {
			return nil
		}
		return m.ifaceAny(iv)
	}
	return m.ifaceAny(&Iface{T: m.rtypeOf(t), V: v})
}

// vmOfAOT converts x, a value of AOT code, to the VM value of type t.
func (m *Machine) vmOfAOT(x any, t types.Type) Value { return ValueOf(x).vmVal(m, t) }
