package vm

// Printing a value nested very deep in interfaces ([]any{[]any{...}}).
//
// fmt recurses once per level on the host stack, and the stack of a
// goroutine is bounded (1 GB): a value nested about 300000 times killed the
// host with a fatal stack overflow, which nothing can recover. The AOT copy
// of fmt walks the value through the twin, with heavier frames. So:
//   - a printer of package fmt whose `any` arguments hold interfaces nested
//     maxBoxDepth times or more runs the native fmt of the host, also in
//     AOT mode (the hook of compile.go) and when the call would go to the
//     AOT copy (aotroute.go). Its arguments are plain native copies, also
//     with OpaqueAny (convGraph.plain): an opaque handle per level made one
//     Format call, and more host stack, per level;
//   - native code gets a handle at each maxBoxDepth bound (boxCopy), and the
//     native copy behind a handle counts the bounds: past deepSegs of them,
//     it panics (vmObj.native), and native fmt prints the panic. Up to
//     there, the output is the one of Go. Native json gets the error
//     errNestedTooDeep (vmObj.MarshalJSON);
//   - the AOT copy of encoding/json (deepWalkers) returns errNestedTooDeep
//     without the walk for a value nested past the same bound; it ended
//     with a fatal stack overflow near 1M levels.
//
// The other AOT copies that walk a value (yaml, toml, xml, asn1, go-cmp,
// templates, fmt called by another AOT copy...) have no bound of their own:
// the twin reflect stops them before the stack limit (stackguard.go).

import (
	"go/types"
	"reflect"

	"golang.org/x/tools/go/ssa"
)

// nestProbeMax bounds the work of nestedDeep: the number of values it
// looks at. A value shared many times in a DAG is looked at each time,
// as fmt prints it each time.
const nestProbeMax = 1 << 24

// cycleFrom is the depth from which nestedDeep looks for cycles (as json
// does): no cost for the values that are not deep.
const cycleFrom = 1000

// deepPrinter returns the site that calls the native fmt function of the
// host for the printer site s of package fmt, when s does not call it
// already (AOT mode); nil otherwise.
func (m *Machine) deepPrinter(f *ssa.Function, s *nativeSite) *nativeSite {
	d := m.printerSite(f, s.args)
	if d == nil || s.fn.IsValid() && s.fn.Pointer() == d.fn.Pointer() {
		return nil
	}
	return d
}

// deepHook returns the guard of the AOT hook of f against values nested too
// deep, or nil: for a printer of package fmt, a call of the native fmt of
// the host; for a walker of deepWalkers, its error without the walk. It
// reports false when the hook can run.
func (m *Machine) deepHook(f *ssa.Function) func(a []Value) (Value, bool) {
	args := make([]int32, f.Signature.Params().Len())
	for i := range args {
		args[i] = int32(i)
	}
	if site := m.printerSite(f, args); site != nil {
		return func(a []Value) (Value, bool) {
			if m.printTooDeep(site, a) {
				return m.callNativeSlow(site, a), true
			}
			return Value{}, false
		}
	}
	if !deepWalkers[f.String()] {
		return nil
	}
	// the `any` arguments, after the receiver of a method
	sig := f.Signature
	var ifs []int
	off := 0
	if sig.Recv() != nil {
		off = 1
	}
	for i := 0; i < sig.Params().Len(); i++ {
		if isIface(sig.Params().At(i).Type()) {
			ifs = append(ifs, i+off)
		}
	}
	nres := sig.Results().Len()
	return func(a []Value) (Value, bool) {
		for _, i := range ifs {
			if i < len(a) && m.nestedDeep(a[i], deepSegs*maxBoxDepth, true) {
				return m.tooDeepResult(nres), true
			}
		}
		return Value{}, false
	}
}

// tooDeepResult returns the results of a walker of deepWalkers that does
// not walk a value nested too deep: zero values and errNestedTooDeep last.
func (m *Machine) tooDeepResult(nres int) Value {
	e := m.ErrorVal(errNestedTooDeep)
	if nres == 1 {
		return e
	}
	vs := make([]Value, nres)
	vs[nres-1] = e
	return Value{R: vs}
}

// deepWalkers are the functions of AOT copies, by ssa name, that walk their
// `any` arguments to the end on the host stack, and return an error. The
// AOT copy of encoding/json walked a value nested about 1M times through
// the twin to a fatal stack overflow (as Go does). Past the bound of native
// code (deepSegs bounds of maxBoxDepth), they return errNestedTooDeep, as
// native json does then (vmObj.MarshalJSON).
var deepWalkers = map[string]bool{
	"encoding/json.Marshal":           true,
	"encoding/json.MarshalIndent":     true,
	"(*encoding/json.Encoder).Encode": true,
}

// printerSite returns a site that calls the native fmt function of the
// host for f, a printer of package fmt, with the argument registers args.
func (m *Machine) printerSite(f *ssa.Function, args []int32) *nativeSite {
	if !printFuncs[f.String()] || f.Pkg == nil || f.Pkg.Pkg.Path() != "fmt" || f.Signature.Recv() != nil {
		return nil
	}
	fn, ok := m.hostNativeFunc(f)
	if !ok {
		return nil
	}
	sig := f.Signature
	return &nativeSite{fn: fn, args: args, params: tupleTypes(sig.Params()), results: tupleTypes(sig.Results()), vari: sig.Variadic(), dst: decodeDstOf(f), prints: true, plain: true, stores: -1}
}

// hostNativeFunc returns the native function of the host for the package
// function f, as nativeFunc does without the AOT copies.
func (m *Machine) hostNativeFunc(f *ssa.Function) (reflect.Value, bool) {
	key := f.Pkg.Pkg.Path() + "." + f.Name()
	if rv, ok := m.overrides[key]; ok {
		return rv, true
	}
	if rv, ok := m.hostFunc(key); ok {
		return rv, true
	}
	if p := natPkg(f.Pkg.Pkg.Path()); p != nil {
		if rv, ok := p.Funcs[f.Name()]; ok {
			return rv, true
		}
	}
	if rv, ok := nativeFuncs[key]; ok {
		return rv, true
	}
	return m.stdSym(f.Pkg.Pkg, f.Name())
}

// printTooDeep reports whether an `any` argument of the printer site s
// holds interfaces nested maxBoxDepth times or more.
func (m *Machine) printTooDeep(s *nativeSite, regs []Value) bool {
	for i, a := range s.args {
		if s.vari && i == len(s.args)-1 {
			vs, _ := regs[a].R.([]Value)
			for _, v := range vs {
				if m.nestedDeep(v, maxBoxDepth, false) {
					return true
				}
			}
			continue
		}
		if isIface(s.params[i]) && m.nestedDeep(regs[a], maxBoxDepth, false) {
			return true
		}
	}
	return false
}

// nestedDeep reports whether the interface value v holds interfaces nested
// limit times or more, in the parts that fmt and json walk: interfaces,
// slices, arrays, structs and maps, and the pointer at the top. It uses
// its own stack, not the host stack. When it looks at nestProbeMax values
// without an answer, it says yes: a wide part looked at first (a DAG that
// shares its values) must not hide a deep part from the probe, and native
// fmt prints a value that is only wide like Go. With cycles, a slice or a
// map seen again stops the walk: json reports a cycle itself (it checks
// from 1000 levels), while fmt recurses on it to a stack overflow, so for
// fmt a cycle counts as deep.
func (m *Machine) nestedDeep(v Value, limit int, cycles bool) bool {
	iv, _ := v.R.(*Iface)
	if iv == nil || iv.T.Native != nil || !holdsIface(iv.T) {
		return false
	}
	type item struct {
		v Value
		d int
	}
	top := iv.V
	if c, ok := top.R.(*Value); ok && c != nil {
		top = *c // fmt prints the value of a pointer at the top
	}
	stack := []item{{top, 1}}
	var seen map[any]bool // containers past cycleFrom levels (cycles)
	for n := 0; len(stack) > 0; n++ {
		if n >= nestProbeMax {
			return true // no answer in the bound: take the safe path
		}
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch x := it.v.R.(type) {
		case *Iface:
			if x == nil || x.T.Native != nil || !holdsIface(x.T) {
				continue
			}
			if it.d+1 >= limit {
				return true
			}
			if cycles && it.d >= cycleFrom {
				var k any
				switch c := x.V.R.(type) {
				case []Value:
					if len(c) > 0 {
						k = &c[0]
					}
				case *MapV:
					k = c
				}
				if k != nil {
					if seen[k] {
						continue
					}
					if seen == nil {
						seen = map[any]bool{}
					}
					seen[k] = true
				}
			}
			stack = append(stack, item{x.V, it.d + 1})
		case []Value:
			for i := range x {
				stack = append(stack, item{x[i], it.d})
			}
		case *MapV:
			if x != nil {
				x.each(func(k, e Value) { stack = append(stack, item{k, it.d}, item{e, it.d}) })
			}
		}
	}
	return false
}

// holdsIface reports whether values of the dynamic type rt may hold an
// interface value in the parts that fmt walks (cached in rt).
func holdsIface(rt *RType) bool {
	switch rt.nests.Load() {
	case 1:
		return false
	case 2:
		return true
	}
	r := typeHoldsIface(rt.T, true, map[types.Type]bool{})
	if r {
		rt.nests.Store(2)
	} else {
		rt.nests.Store(1)
	}
	return r
}

func typeHoldsIface(t types.Type, top bool, seen map[types.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := t.Underlying().(type) {
	case *types.Interface:
		return true
	case *types.Pointer:
		return top && typeHoldsIface(u.Elem(), false, seen)
	case *types.Slice:
		return typeHoldsIface(u.Elem(), false, seen)
	case *types.Array:
		return typeHoldsIface(u.Elem(), false, seen)
	case *types.Map:
		return typeHoldsIface(u.Key(), false, seen) || typeHoldsIface(u.Elem(), false, seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if typeHoldsIface(u.Field(i).Type(), false, seen) {
				return true
			}
		}
	}
	return false
}
