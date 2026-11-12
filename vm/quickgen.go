package vm

// testing/quick with the Generate method of a type of the program.
//
// quick makes the arguments of the function it checks with the Generate
// method of their type (quick.Generator), found with reflect on the native
// type of the parameter: the native type of a program type (a native int
// for type small int) has no such method, so quick made its own random
// values. quick.Check and CheckEqual now give quick a Config.Values that
// calls the Generate method of the program for these parameters, and
// quick.Value for the others, as quick does. A Config with its own Values
// is left as it is. Only the parameters themselves are made with Generate:
// quick also calls it for a field, an element or the target of a pointer
// of a type with Generate, which still get random values.
//
// The functions of quick come from the registry: the package vm does not
// import testing/quick, whose init adds the flag -quickchecks to the
// flag.CommandLine of the host (TestHostImport).

import (
	"go/types"
	"math/rand"
	"reflect"
)

func init() {
	for name, n := range map[string]int{"testing/quick.Check": 1, "testing/quick.CheckEqual": 2} {
		n := n
		vmHooks[name] = func(m *Machine, a []Value) Value { return m.quickCheck(a, n) }
		vmHooksAOT[name] = true
	}
}

// quickCheck implements quick.Check (nf 1) and quick.CheckEqual (nf 2):
// a holds the nf functions, then the *quick.Config.
func (m *Machine) quickCheck(a []Value, nf int) Value {
	name := "Check"
	if nf == 2 {
		name = "CheckEqual"
	}
	check := m.quickFunc(name)
	in := make([]reflect.Value, nf+1)
	for i := range nf {
		in[i] = reflect.Zero(check.Type().In(i))
		if iv, _ := a[i].R.(*Iface); iv != nil {
			in[i] = m.boxAs(iv, newConv(nil), false) // a native func, as plainFuncs
		}
	}
	cfgT := check.Type().In(nf) // *quick.Config
	cfg := reflect.Zero(cfgT)
	if a[nf].R != nil {
		if c := m.toNative(a[nf], types.NewPointer(m.quickConfigType())); c.Type() == cfgT {
			cfg = c
		}
	}
	if gens := m.quickGenerators(a[0]); gens != nil && (cfg.IsNil() || cfg.Elem().FieldByName("Values").IsNil()) {
		c := reflect.New(cfgT.Elem())
		if !cfg.IsNil() {
			c.Elem().Set(cfg.Elem())
		}
		value := m.quickFunc("Value")
		ft := in[0].Type()
		if ft.Kind() == reflect.Interface {
			ft = in[0].Elem().Type()
		}
		values := func(args []reflect.Value, r *rand.Rand) {
			for j := range args {
				t := ft.In(j)
				if gens[j] == nil {
					// quick.Value(t, r) returns a reflect.Value
					args[j], _ = value.Call([]reflect.Value{reflect.ValueOf(t), reflect.ValueOf(r)})[0].Interface().(reflect.Value)
					continue
				}
				v := gens[j](r, quickComplexSize)
				if v.IsValid() && v.Type() != t && v.Type().ConvertibleTo(t) {
					v = v.Convert(t)
				}
				args[j] = v
			}
		}
		c.Elem().FieldByName("Values").Set(reflect.ValueOf(values))
		cfg = c
	}
	in[nf] = cfg
	return m.fromNative(check.Call(in)[0], errorT)
}

// quickFunc returns the native function name of testing/quick.
func (m *Machine) quickFunc(name string) reflect.Value {
	return m.nativeFunc(m.prog.ImportedPackage("testing/quick").Func(name))
}

// quickComplexSize is the size quick gives to Generate (complexSize).
const quickComplexSize = 50

// quickConfigType is quick.Config.
func (m *Machine) quickConfigType() types.Type {
	return m.prog.ImportedPackage("testing/quick").Pkg.Scope().Lookup("Config").Type()
}

// quickGenerators returns, for each parameter of the function f of the
// program, the Generate method of its type (nil when it has none), or nil
// when no parameter has one.
func (m *Machine) quickGenerators(f Value) []func(*rand.Rand, int) reflect.Value {
	iv, _ := f.R.(*Iface)
	if iv == nil || iv.T.T == nil {
		return nil
	}
	sig, ok := iv.T.T.Underlying().(*types.Signature)
	if !ok {
		return nil
	}
	gens := make([]func(*rand.Rand, int) reflect.Value, sig.Params().Len())
	found := false
	for i := range gens {
		t := sig.Params().At(i).Type()
		if types.IsInterface(t) {
			// quick asks the zero value, a nil interface: not a Generator
			continue
		}
		sel := m.prog.MethodSets.MethodSet(t).Lookup(nil, "Generate")
		if sel == nil || !isQuickGenerate(sel.Type().(*types.Signature)) {
			continue
		}
		fn := m.funcOf(m.prog.MethodValue(sel))
		rt := sel.Type().(*types.Signature).Results().At(0).Type()
		gens[i] = func(r *rand.Rand, size int) reflect.Value {
			res := m.Call(fn, []Value{m.zero(t), {R: reflect.ValueOf(r)}, {N: int64(size)}}, nil)
			switch v := m.toNative(res, rt).Interface().(type) {
			case reflect.Value:
				return v
			case RV: // the twin reflect of the program
				return m.realValue(v)
			}
			return reflect.Value{}
		}
		found = true
	}
	if !found {
		return nil
	}
	return gens
}

// isQuickGenerate reports whether sig is the signature of the Generate
// method of quick.Generator: func(*rand.Rand, int) reflect.Value.
func isQuickGenerate(sig *types.Signature) bool {
	if sig.Params().Len() != 2 || sig.Results().Len() != 1 || !types.Identical(sig.Params().At(1).Type(), types.Typ[types.Int]) {
		return false
	}
	named := func(t types.Type, pkg, name string) bool {
		n, ok := types.Unalias(t).(*types.Named)
		return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkg && n.Obj().Name() == name
	}
	p, ok := sig.Params().At(0).Type().(*types.Pointer)
	return ok && named(p.Elem(), "math/rand", "Rand") && named(sig.Results().At(0).Type(), "reflect", "Value")
}
