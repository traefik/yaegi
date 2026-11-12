package vm

// Native functions re-implemented on VM values because the reflect boundary
// cannot give them correct semantics: they always replace the native call
// (unlike nativeHooks, which are AOT-only).

import (
	"go/types"
	"reflect"
	"sort"

	"golang.org/x/tools/go/ssa"
)

var vmHooks = map[string]NativeHook{}

func init() {
	vmHooks["sort.Slice"] = func(m *Machine, a []Value) Value {
		s, x := m.vmSorter(a)
		if s == nil {
			sort.Slice(x, s.lessFunc(m, a)) // panics as gc does
		}
		sort.Sort(s)
		return Value{}
	}
	vmHooks["sort.SliceStable"] = func(m *Machine, a []Value) Value {
		s, x := m.vmSorter(a)
		if s == nil {
			sort.SliceStable(x, s.lessFunc(m, a)) // panics as gc does
		}
		sort.Stable(s)
		return Value{}
	}
	vmHooks["sort.SliceIsSorted"] = func(m *Machine, a []Value) Value {
		s, x := m.vmSorter(a)
		if s == nil {
			// gc only needs Len: an array, a string, a map or a channel
			// does not panic
			return Value{N: b2i(sort.SliceIsSorted(x, s.lessFunc(m, a)))}
		}
		return Value{N: b2i(sort.IsSorted(s))}
	}
	// The AOT copy of package sort uses vreflect: a value that is not a
	// slice panicked with a *reflect.ValueError, not the
	// *reflectlite.ValueError of gc.
	for _, name := range []string{"sort.Slice", "sort.SliceStable", "sort.SliceIsSorted"} {
		vmHooksAOT[name] = true
	}
}

// vmSorter sorts an interpreted slice in place with an interpreted less
// function: native sort.Slice would swap a copy while less reads the original.
type vmSorter struct {
	m    *Machine
	vals []Value
	word bool // the elements of vals are of a wordKind type (wordcell.go)
	b    []byte
	nat  reflect.Value
	swap func(i, j int)
	less Value
}

// vmSorter returns the sorter of the arguments a of a hook of package sort.
// When a[0] is not a slice, it returns a nil sorter and a host value of the
// same kind and the same length: the native function runs on it, and
// panics with the *reflectlite.ValueError of gc, or only reads its length.
func (m *Machine) vmSorter(a []Value) (*vmSorter, any) {
	s := &vmSorter{m: m, less: a[1]}
	iv, _ := a[0].R.(*Iface)
	switch {
	case iv == nil:
		return nil, nil
	case iv.T.T == nil:
		x := iv.V.R.(reflect.Value)
		if x.Kind() != reflect.Slice {
			return nil, x.Interface()
		}
		s.nat, s.swap = x, reflect.Swapper(x.Interface()) // a native slice
		return s, nil
	}
	if _, ok := iv.T.T.Underlying().(*types.Slice); !ok {
		return nil, m.sortStandIn(iv)
	}
	switch x := iv.V.R.(type) {
	case []Value:
		s.vals = x
		s.word = wordKind(iv.T.T.Underlying().(*types.Slice).Elem())
	case []byte:
		s.b = x
	case reflect.Value:
		s.nat, s.swap = x, reflect.Swapper(x.Interface())
	}
	return s, nil // nil: a nil slice
}

// lessFunc is the less function of a hook, for the native function. s may
// be nil.
func (*vmSorter) lessFunc(m *Machine, a []Value) func(i, j int) bool {
	return (&vmSorter{m: m, less: a[1]}).Less
}

// sortStandIn returns a host value with the kind and the length of iv, a
// value of the program that is not a slice. In the AOT mode, the native form
// of a map, a channel or an array of the program is an opaque handle (a
// struct), so the native form cannot be used.
func (m *Machine) sortStandIn(iv *Iface) any {
	n := 0
	switch r := iv.V.R.(type) {
	case *MapV:
		n = len(r.M)
	case chan Value:
		n = len(r)
	case reflect.Value:
		if k := r.Kind(); k == reflect.Map || k == reflect.Chan {
			n = r.Len()
		}
	}
	type e struct{}
	switch u := iv.T.T.Underlying().(type) {
	case *types.Basic:
		return m.toNative(iv.V, u).Interface()
	case *types.Array:
		return reflect.New(reflect.ArrayOf(int(u.Len()), reflect.TypeFor[e]())).Elem().Interface()
	case *types.Map:
		x := make(map[int]e, n)
		for i := range n {
			x[i] = e{}
		}
		return x
	case *types.Chan:
		x := make(chan e, n)
		for range n {
			x <- e{}
		}
		return x
	case *types.Pointer:
		return (*e)(nil)
	case *types.Signature:
		return (func())(nil)
	}
	return e{}
}

func (s *vmSorter) Len() int {
	switch {
	case s.swap != nil:
		return s.nat.Len()
	case s.b != nil:
		return len(s.b)
	}
	return len(s.vals)
}

func (s *vmSorter) Swap(i, j int) {
	switch {
	case s.swap != nil:
		s.swap(i, j)
	case s.b != nil:
		s.m.hostBytes(s.b, 1) // the bytes of the host are not the plugin's (hostmem.go)
		s.b[i], s.b[j] = s.b[j], s.b[i]
	case s.word:
		swapWords(s.vals, i, j) // a racy reader must not see half of an element
	default:
		s.vals[i], s.vals[j] = s.vals[j], s.vals[i]
	}
}

func (s *vmSorter) Less(i, j int) bool {
	switch f := s.less.R.(type) {
	case *Closure:
		args := []Value{{N: int64(i)}, {N: int64(j)}}
		if f.Go != nil {
			return f.Go(args).N != 0
		}
		return s.m.Call(f.Fn, args, f.Env).N != 0
	case reflect.Value:
		return f.Call([]reflect.Value{reflect.ValueOf(i), reflect.ValueOf(j)})[0].Bool()
	}
	panic(errNilDeref)
}

// vmHookName returns "pkgpath.Name" for a package-level function, "" for a
// method or a function without a package (the key of vmHooks).
func vmHookName(f *ssa.Function) string {
	if f.Pkg == nil || f.Signature.Recv() != nil {
		return ""
	}
	return f.Pkg.Pkg.Path() + "." + f.Name()
}

// hookOf returns the vmHooks entry that replaces the native function f, or
// nil.
func hookOf(f *ssa.Function) NativeHook {
	return vmHooks[vmHookName(f)]
}
