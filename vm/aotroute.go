package vm

// Calls of native reflect-based functions with interpreted values, in
// default mode.
//
// Native code cannot see interpreted types: Go cannot create named types at
// run time, so a value crosses the boundary as a native copy of its
// underlying type, or boxed in a hand-written wrapper (stringerW...). fmt
// then prints "vm.stringerW" or "struct {}" for %T, %#v loses the type
// names, String methods of nested values are not called, and encoding/xml
// refuses an unnamed struct.
//
// When the AOT copy of the package is compiled into the host (aot/all), a
// call of one of the functions below whose `any` arguments hold an
// interpreted type goes to that copy instead: it works through the twin
// reflect, which sees the interpreted types, names, fields and methods
// (as with -aot). Calls whose arguments only hold basic and native values
// stay native.

import (
	"go/types"
	"reflect"

	"golang.org/x/tools/go/ssa"
)

// aotRoutes lists, per package, the functions routed to the AOT copy and
// the index of their `any` argument (-1: the variadic ...any argument, the
// last one). fmt.Errorf goes to the copy too when a value of the plugin
// needs it: the error of the copy wraps the native errors of %w as they
// are (errors.Is and errors.As walk them with Unwrap), and native Errorf
// did not call Format or GoString, and named the plugin's types with their
// underlying types.
var aotRoutes = map[string]map[string]int{
	"fmt": {
		"Errorf": -1,
		"Print":  -1, "Println": -1, "Printf": -1,
		"Sprint": -1, "Sprintln": -1, "Sprintf": -1,
		"Fprint": -1, "Fprintln": -1, "Fprintf": -1,
		"Append": -1, "Appendln": -1, "Appendf": -1,
	},
	"encoding/xml":  {"Marshal": 0, "MarshalIndent": 0, "Unmarshal": 1},
	"encoding/json": {"Marshal": 0, "MarshalIndent": 0, "Unmarshal": 1},
}

// setupAOTRoutes selects the packages of aotRoutes whose calls can go to
// their AOT copy: the copy is registered and the package is native by
// default (no explicit mode). It reports whether there is one.
func (m *Machine) setupAOTRoutes() bool {
	for path := range aotRoutes {
		if aotPkg(path) == nil || m.modeOf(path) != ModeNative {
			continue
		}
		if _, explicit := m.Modes[path]; explicit {
			continue
		}
		if m.routed == nil {
			m.routed = map[string]bool{}
		}
		m.routed[path] = true
	}
	return len(m.routed) > 0
}

// aotRouteCall returns the implementation of a call of the native function
// f with the argument registers args, when f is routed to its AOT copy
// (nil otherwise).
func (m *Machine) aotRouteCall(f *ssa.Function, args []int32) func([]Value) Value {
	if m.vr == nil || f.Pkg == nil || f.Signature.Recv() != nil || !m.routed[f.Pkg.Pkg.Path()] {
		return nil
	}
	path := f.Pkg.Pkg.Path()
	ai, ok := aotRoutes[path][f.Name()]
	if !ok {
		return nil
	}
	fn, ok := aotPkg(path).Funcs[f.Name()]
	if !ok {
		return nil
	}
	s := m.nativeSite(f, args)
	if ai < 0 {
		ai = len(args) - 1
	}
	ar := args[ai]
	walker := deepWalkers[f.String()]
	ls, lost := lostSiteOf(f)
	return func(regs []Value) Value {
		if m.routeNeeded(regs[ar], ai == len(s.args)-1 && s.vari, path) && !(s.prints && m.printTooDeep(s, regs)) {
			// (a value nested too deep to print stays native: deepprint.go)
			if walker && m.nestedDeep(regs[ar], deepSegs*maxBoxDepth, true) {
				return m.tooDeepResult(len(s.results)) // as in AOT mode
			}
			return m.callAOTRoute(path, s, fn, ai, regs)
		}
		if lost {
			if err := m.lostCheck(ls.name, regs[ar]); err != nil {
				return m.lostResult(err, len(s.results)) // lostmethods.go
			}
		}
		return m.callNativeSite(s, regs)
	}
}

// routeNeeded reports whether the `any` argument v (a slice of them when
// vari is set) of a function of the package path holds a value that native
// code would see wrongly, and that the AOT copy can take.
func (m *Machine) routeNeeded(v Value, vari bool, path string) bool {
	if !vari {
		iv, _ := v.R.(*Iface)
		if iv == nil {
			return false
		}
		need, stay := m.ifaceRoute(iv, path, new(int))
		return need && !stay
	}
	s, _ := v.R.([]Value)
	need := false
	walked := 0
	for _, v := range s {
		if iv, _ := v.R.(*Iface); iv != nil {
			n, stay := m.ifaceRoute(iv, path, &walked)
			if stay {
				return false
			}
			need = need || n
		}
	}
	return need
}

// routeWalkMax bounds the values that ifaceRoute walks in the native
// containers of one call.
const routeWalkMax = 1000

// ifaceRoute reports whether the interface value iv holds a value that
// native code would see wrongly (need), and whether it holds a type of the
// package path that the AOT copy does not know (stay: the call stays
// native). A value of a native type that holds interfaces ([]any,
// map[string]any, [1]any) is walked: native fmt printed a named struct of
// the program inside a []any as an unnamed struct. walked counts the
// values walked, up to routeWalkMax.
func (m *Machine) ifaceRoute(iv *Iface, path string, walked *int) (need, stay bool) {
	if iv.T.Native == nil {
		if !m.nativeWrong(iv.T) {
			return false, false
		}
		return true, m.holdsPkgType(iv.T, path)
	}
	if iv.T.T == nil {
		return false, false
	}
	return m.valueRoute(iv.V, iv.T.T, path, walked)
}

// valueRoute is ifaceRoute for the value v of the native type t.
func (m *Machine) valueRoute(v Value, t types.Type, path string, walked *int) (need, stay bool) {
	if *walked >= routeWalkMax || !mentionsIface(t, map[types.Type]bool{}) {
		return false, false
	}
	*walked++
	walk := func(v Value, t types.Type) bool {
		n, s := m.valueRoute(v, t, path, walked)
		need = need || n
		stay = stay || s
		return stay
	}
	switch u := t.Underlying().(type) {
	case *types.Interface:
		if iv, _ := v.R.(*Iface); iv != nil {
			return m.ifaceRoute(iv, path, walked)
		}
	case *types.Slice:
		for _, e := range sliceVals(v) {
			if walk(e, u.Elem()) {
				break
			}
		}
	case *types.Array:
		for _, e := range sliceVals(v) {
			if walk(e, u.Elem()) {
				break
			}
		}
	case *types.Struct:
		fs := sliceVals(v)
		for i := 0; i < u.NumFields() && i < len(fs); i++ {
			if walk(fs[i], u.Field(i).Type()) {
				break
			}
		}
	case *types.Map:
		mp, _ := v.R.(*MapV)
		if mp == nil {
			break
		}
		for k, e := range mp.M {
			if walk(e, u.Elem()) || walk(mp.key(k), u.Key()) {
				break
			}
		}
	case *types.Pointer:
		if cell, ok := v.R.(*Value); ok && cell != nil {
			walk(*cell, u.Elem())
		}
	}
	return need, stay
}

// sliceVals returns the elements of an interpreted slice, array or struct,
// or nil for another representation (a native view).
func sliceVals(v Value) []Value {
	s, _ := v.R.([]Value)
	return s
}

// mentionsIface reports whether t contains an interface type.
func mentionsIface(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := t.Underlying().(type) {
	case *types.Interface:
		return true
	case *types.Pointer:
		return mentionsIface(u.Elem(), seen)
	case *types.Slice:
		return mentionsIface(u.Elem(), seen)
	case *types.Array:
		return mentionsIface(u.Elem(), seen)
	case *types.Map:
		return mentionsIface(u.Key(), seen) || mentionsIface(u.Elem(), seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if mentionsIface(u.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

// ifaceInUnexported reports whether t contains an interface type in an
// unexported struct field (hidden: t is reached through one).
func ifaceInUnexported(t types.Type, hidden bool, seen map[types.Type]bool) bool {
	// (seen: visited, true when visited as hidden)
	if h, ok := seen[t]; ok && (h || !hidden) {
		return false
	}
	seen[t] = hidden
	switch u := t.Underlying().(type) {
	case *types.Interface:
		return hidden
	case *types.Pointer:
		return ifaceInUnexported(u.Elem(), hidden, seen)
	case *types.Slice:
		return ifaceInUnexported(u.Elem(), hidden, seen)
	case *types.Array:
		return ifaceInUnexported(u.Elem(), hidden, seen)
	case *types.Map:
		return ifaceInUnexported(u.Key(), hidden, seen) || ifaceInUnexported(u.Elem(), hidden, seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			if ifaceInUnexported(f.Type(), hidden || !f.Exported(), seen) {
				return true
			}
		}
	}
	return false
}

// holdsPkgType reports whether values of the dynamic type rt may hold a
// named type of the package path itself (xml.Name, xml.Attr in a struct).
// The AOT copy has its own copies of these types and does not know the
// native ones, so such a call stays native (cached per type and package).
// The types of routeNativeOK do not count: the copy handles the native
// ones as its own.
func (m *Machine) holdsPkgType(rt *RType, path string) bool {
	k := pkgTypeKey{rt, path}
	if r, ok := m.pkgTypes.Load(k); ok {
		return r.(bool)
	}
	r := mentionsPkg(rt.T, path, map[types.Type]bool{})
	m.pkgTypes.Store(k, r)
	return r
}

// routeNativeOK are the named types of a routed package that the AOT copy
// handles right when it gets them as native values, by package and name.
// json.RawMessage: the copy only reaches it through its methods, which the
// copy finds with its own interfaces (a RawMessage is a json.Marshaler and
// its pointer a json.Unmarshaler). json.Number and json.Delim: the copy
// takes the host's type as its own (sameValType, in EqualAny). The
// interfaces json.Marshaler and json.Unmarshaler: the copy calls their
// values through their methods. A value that holds one of them goes to the
// copy. fmt.Stringer, fmt.GoStringer and fmt.Formatter: the copy calls the
// value of such a field through its methods (a struct that embeds a nil
// fmt.Stringer prints "PANIC=String method", as in Go).
var routeNativeOK = map[string]map[string]bool{
	"encoding/json": {"RawMessage": true, "Number": true, "Delim": true, "Marshaler": true, "Unmarshaler": true},
	"fmt":           {"Stringer": true, "GoStringer": true, "Formatter": true},
}

type pkgTypeKey struct {
	rt   *RType
	path string
}

// mentionsPkg reports whether t mentions a named type of the package path,
// through pointers, composites, struct fields and the underlying types of
// other named types.
func mentionsPkg(t types.Type, path string, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.Named:
		if p := u.Obj().Pkg(); p != nil && p.Path() == path {
			return !routeNativeOK[path][u.Obj().Name()]
		}
		return mentionsPkg(u.Underlying(), path, seen)
	case *types.Pointer:
		return mentionsPkg(u.Elem(), path, seen)
	case *types.Slice:
		return mentionsPkg(u.Elem(), path, seen)
	case *types.Array:
		return mentionsPkg(u.Elem(), path, seen)
	case *types.Chan:
		return mentionsPkg(u.Elem(), path, seen)
	case *types.Map:
		return mentionsPkg(u.Key(), path, seen) || mentionsPkg(u.Elem(), path, seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if mentionsPkg(u.Field(i).Type(), path, seen) {
				return true
			}
		}
	}
	return false
}

// nativeWrong reports whether native reflect-based code sees values of the
// dynamic type rt wrongly (cached in rt).
func (m *Machine) nativeWrong(rt *RType) bool {
	switch rt.natWrong.Load() {
	case 1:
		return false
	case 2:
		return true
	}
	wrong := !m.nativeOK(rt.T, 0)
	if wrong {
		rt.natWrong.Store(2)
	} else {
		rt.natWrong.Store(1)
	}
	return wrong
}

// nativeOK reports whether native reflect sees values of type t like gc:
// basic types and native named types, and pointers, slices, arrays, maps,
// channels and functions of them. Interpreted named types, structs and
// interfaces (which may hold interpreted values) are not.
func (m *Machine) nativeOK(t types.Type, depth int) bool {
	if depth > 8 {
		return false
	}
	if _, ok := types.Unalias(t).(*types.Named); ok {
		return m.isNativeNamed(t) && !m.mentionsInterpNamed(t)
	}
	switch u := types.Unalias(t).(type) {
	case *types.Basic:
		return true
	case *types.Pointer:
		return m.nativeOK(u.Elem(), depth+1)
	case *types.Slice:
		return m.nativeOK(u.Elem(), depth+1)
	case *types.Array:
		return m.nativeOK(u.Elem(), depth+1)
	case *types.Chan:
		return m.nativeOK(u.Elem(), depth+1)
	case *types.Map:
		return m.nativeOK(u.Key(), depth+1) && m.nativeOK(u.Elem(), depth+1)
	case *types.Signature:
		// printed as an address, but %T prints the type, with the names
		// of the program's types in it (func(main.T) int)
		for _, tu := range []*types.Tuple{u.Params(), u.Results()} {
			for i := 0; i < tu.Len(); i++ {
				if !m.nativeOK(tu.At(i).Type(), depth+1) {
					return false
				}
			}
		}
		return true
	}
	return false
}

// callAOTRoute calls fn, the AOT copy of the native function of site s:
// the `any` argument ai (or the ...any arguments) is boxed for the twin
// reflect, as in AOT mode; the other arguments are converted as for the
// native call.
func (m *Machine) callAOTRoute(path string, s *nativeSite, fn reflect.Value, ai int, regs []Value) Value {
	var wb []func()
	c := newConv(&wb)
	c.g.aot = true
	in := make([]reflect.Value, len(s.args))
	vari := s.vari && ai == len(s.args)-1
	for i, a := range s.args {
		switch {
		case i != ai:
			in[i] = m.toNativeW(regs[a], s.params[i], c)
		case vari:
			vs, _ := regs[a].R.([]Value)
			xs := make([]any, len(vs))
			for j, v := range vs {
				xs[j] = m.routeArg(v, c)
			}
			in[i] = reflect.ValueOf(xs)
		default:
			in[i] = reflect.ValueOf(m.routeArg(regs[a], c))
			if !in[i].IsValid() {
				in[i] = reflect.Zero(anyType)
			}
		}
	}
	defer writeBack(wb)
	defer m.flushShadows()
	out := callRV(fn, in, s.vari)
	m.nativeRouteErrs(path, out) // aoterrors.go
	return m.pack(out, s.results)
}

// routeArg converts the interface value v for an AOT copy.
func (m *Machine) routeArg(v Value, c *natConv) any {
	iv, _ := v.R.(*Iface)
	switch {
	case iv == nil:
		return nil
	case iv.T.Native != nil:
		return copyVal(iv.V.R.(reflect.Value).Interface())
	}
	if b := m.boxAs(iv, c, true); b.IsValid() {
		return b.Interface()
	}
	return nil
}
