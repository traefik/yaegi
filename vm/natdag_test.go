package vm

import (
	"go/types"
	"reflect"
	"testing"
	"time"
)

// TestFromNativeDAG converts back native values where the same slice or map
// is reached by several paths (a DAG: each level holds the level below
// twice). The conversions copied a shared part once per path: 2^depth
// copies, on the host CPU and not counted by the budget. They now convert
// each native slice and map once, and the VM value keeps the sharing, as
// the native value has it (a write through one path is seen through the
// other, as in Go).
func TestFromNativeDAG(t *testing.T) {
	m, spkg := buildSrc(t, "package p\n\ntype S []S\ntype M map[string]M\ntype E struct{ L, R []E }\n")
	scope := spkg.Pkg.Scope()
	sT, mT, eT := scope.Lookup("S").Type(), scope.Lookup("M").Type(), scope.Lookup("E").Type()
	const depth = 60

	s := reflect.MakeSlice(m.reflectType(sT), 0, 0)
	mp := reflect.MakeMap(m.reflectType(mT))
	esT := types.NewSlice(eT)
	// E is recursive: its fields are cut to []any (see reflectType), each
	// element holds an E.
	eN := m.reflectType(eT)
	fieldT := eN.Field(0).Type
	es := reflect.MakeSlice(fieldT, 0, 0)
	anyS := []any{}
	for range depth {
		n := reflect.MakeSlice(s.Type(), 2, 2)
		n.Index(0).Set(s)
		n.Index(1).Set(s)
		s = n
		nm := reflect.MakeMap(mp.Type())
		nm.SetMapIndex(reflect.ValueOf("a"), mp)
		nm.SetMapIndex(reflect.ValueOf("b"), mp)
		mp = nm
		e := reflect.New(eN).Elem()
		e.Field(0).Set(es)
		e.Field(1).Set(es)
		ne := reflect.MakeSlice(fieldT, 1, 1)
		ne.Index(0).Set(e)
		es = ne
		anyS = []any{anyS, anyS}
	}
	sameSlice := func(name string, a, b Value) {
		t.Helper()
		x, y := a.R.([]Value), b.R.([]Value)
		if len(x) == 0 || len(x) != len(y) || &x[0] != &y[0] {
			t.Errorf("%s: the two paths give two VM slices, want one", name)
		}
	}
	check := func(name string, conv func() Value, shared func(Value)) {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			v := conv()
			if d := time.Since(start); d > 10*time.Second {
				t.Errorf("conversion took %v", d)
			}
			shared(v)
		})
	}
	sliceShared := func(v Value) {
		out := v.R.([]Value)
		sameSlice("S", out[0], out[1])
	}
	mapShared := func(v Value) {
		out := v.R.(*MapV)
		a, b := out.M[storeKey("a")].R.(*MapV), out.M[storeKey("b")].R.(*MapV)
		if a != b {
			t.Errorf("M: the two paths give two VM maps, want one")
		}
	}
	structShared := func(v Value) {
		f := v.R.([]Value)[0].R.([]Value)
		sameSlice("E", f[0], f[1])
	}
	anyShared := func(v Value) {
		out := v.R.(*Iface).V.R.([]Value)
		sameSlice("[]any", out[0].R.(*Iface).V, out[1].R.(*Iface).V)
	}
	anyT := types.NewInterfaceType(nil, nil)
	check("fromNative/S", func() Value { return m.fromNative(s, sT) }, sliceShared)
	check("fromNativeDeep/S", func() Value { return m.fromNativeDeep(s, sT) }, sliceShared)
	check("fromNativeDeep/M", func() Value { return m.fromNativeDeep(mp, mT) }, mapShared)
	check("fromNativeDeep/E", func() Value { return m.fromNativeDeep(es, esT) }, structShared)
	check("natToVM/S", func() Value { return m.natToVM(s, sT) }, sliceShared)
	check("natToVM/M", func() Value { return m.natToVM(mp, mT) }, mapShared)
	check("natToVM/E", func() Value { return m.natToVM(es, esT) }, structShared)
	check("natToVM/any", func() Value { return m.natToVM(reflect.ValueOf(&anyS).Elem().Convert(reflect.TypeFor[any]()), anyT) }, anyShared)
	check("retVM/any", func() Value { return Value{R: &Iface{V: m.retVM(reflect.ValueOf(anyS))}} }, anyShared)
}

// TestToNativeDAG is TestFromNativeDAG the other way: an interpreted value
// that shares its parts (each level holds the level below twice) is
// converted to native memory once per part, and the native value shares
// them too.
func TestToNativeDAG(t *testing.T) {
	m, spkg := buildSrc(t, "package p\n\ntype S []S\ntype M map[string]M\n")
	scope := spkg.Pkg.Scope()
	sT, mT := scope.Lookup("S").Type(), scope.Lookup("M").Type()
	const depth = 60
	s := Value{R: []Value{}}
	mp := Value{R: &MapV{M: map[any]Value{}, K: map[any]Value{}}}
	for range depth {
		s = Value{R: []Value{s, s}}
		n := &MapV{M: map[any]Value{}, K: map[any]Value{}}
		for _, k := range []string{"a", "b"} {
			n.M[storeKey(k)], n.K[storeKey(k)] = mp, Value{R: k}
		}
		mp = Value{R: n}
	}
	start := time.Now()
	ns := m.toNative(s, sT)
	nm := m.toNative(mp, mT)
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("conversion took %v", d)
	}
	a, b := ns.Index(0).Elem(), ns.Index(1).Elem()
	if a.Pointer() != b.Pointer() {
		t.Errorf("S: the two paths give two native slices, want one")
	}
	ma, mb := nm.MapIndex(reflect.ValueOf("a")), nm.MapIndex(reflect.ValueOf("b"))
	if ma.Kind() == reflect.Interface {
		ma, mb = ma.Elem(), mb.Elem()
	}
	if ma.Pointer() != mb.Pointer() {
		t.Errorf("M: the two paths give two native maps, want one")
	}
}
