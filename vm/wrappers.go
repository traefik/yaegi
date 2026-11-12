package vm

// Generic interface wrappers: the extractor generates (package stdsyms),
// for every interface of the standard library, a struct type implementing it by forwarding each method
// to a func field (e.g. io/_Writer: struct{IValue any; WWrite func([]byte)
// (int, error)}). An interpreted value passed where native code expects a
// non-empty native interface is boxed in the largest such wrapper whose
// methods it has, with func fields built by reflect.MakeFunc calling its
// interpreted methods. This replaces the reflect wall's finite set of
// hand-written wrappers (stringerW, errorW, handlerW...), which are still
// preferred when they fit (no MakeFunc).

import (
	"go/types"
	"reflect"
	"strings"
	"sync"

	"github.com/traefik/yaegi/v2/stdsyms"
)

type yaegiWrapper struct {
	t       reflect.Type // the wrapper struct type
	methods []string     // method names, field i+1 is "W"+methods[i]
	realStd bool         // from stdsyms.Unrestricted: only for Machine.realStd
}

var (
	wrappersOnce sync.Once
	wrappers     []*yaegiWrapper
	wrapperTypes map[reflect.Type]bool
	rwGeneric    reflect.Type // the generic wrapper of http.ResponseWriter (rwwriter.go)
)

// loadWrappers finds the wrappers of both tables: stdsyms.Symbols, and
// stdsyms.Unrestricted (syscall.Conn, syscall.RawConn... for the machines
// with the standard library as in Go).
func loadWrappers() {
	wrapperTypes = map[reflect.Type]bool{}
	scanWrappers(stdsyms.Symbols, false)
	scanWrappers(stdsyms.Unrestricted, true)
	scanWrappers(nativeWrappers(), false)
	rwGeneric = stdsyms.Symbols["net/http/http"]["_ResponseWriter"].Type().Elem()
}

func scanWrappers(table map[string]map[string]reflect.Value, realStd bool) {
	for _, syms := range table {
		for name, v := range syms {
			if !strings.HasPrefix(name, "_") || v.Kind() != reflect.Pointer || v.Type().Elem().Kind() != reflect.Struct {
				continue
			}
			st := v.Type().Elem()
			if st.NumField() < 2 || st.Field(0).Name != "IValue" {
				continue
			}
			if wrapperTypes[st] {
				continue
			}
			w := &yaegiWrapper{t: st, realStd: realStd}
			ok := true
			for i := 1; i < st.NumField(); i++ {
				f := st.Field(i)
				if !strings.HasPrefix(f.Name, "W") || f.Type.Kind() != reflect.Func {
					ok = false
					break
				}
				w.methods = append(w.methods, f.Name[1:])
			}
			if ok {
				wrappers = append(wrappers, w)
				wrapperTypes[st] = true
			}
		}
	}
}

// nativeWrappers returns the wrappers of the native tables
// (Package.Wrappers) in the format of the stdsyms tables.
func nativeWrappers() map[string]map[string]reflect.Value {
	regMu.RLock()
	defer regMu.RUnlock()
	table := map[string]map[string]reflect.Value{}
	for path, p := range natPkgs {
		syms := map[string]reflect.Value{}
		for name, t := range p.Wrappers {
			syms["_"+name] = reflect.Zero(reflect.PointerTo(t))
		}
		table[path] = syms
	}
	return table
}

func isWrapperType(t reflect.Type) bool {
	wrappersOnce.Do(loadWrappers)
	return wrapperTypes[t]
}

type wrapKey struct {
	rt     *RType
	target reflect.Type
}

// wrapperFor returns the best yaegi wrapper for an interpreted dynamic type
// passed as the native interface target, or nil.
func (m *Machine) wrapperFor(rt *RType, target reflect.Type) *yaegiWrapper {
	k := wrapKey{rt, target}
	if w, ok := m.wrapCache.Load(k); ok {
		return w.(*yaegiWrapper)
	}
	wrappersOnce.Do(loadWrappers)
	var best *yaegiWrapper
	for _, w := range wrappers {
		if w.realStd && !m.realStd || !w.t.Implements(target) || best != nil && len(w.methods) <= len(best.methods) {
			continue
		}
		if m.hasMethods(rt, w) {
			best = w
		}
	}
	m.wrapCache.Store(k, best)
	return best
}

// hasMethods reports whether rt has every method of the wrapper w, with the
// same (native) signature.
func (m *Machine) hasMethods(rt *RType, w *yaegiWrapper) bool {
	for i, name := range w.methods {
		if nt := m.methodType(rt, name); nt == nil || nt != w.t.Field(i+1).Type {
			return false
		}
	}
	return true
}

// methodType returns the native func type of the method name of rt, without
// the receiver, or nil when rt has no such method or its signature has no
// native type.
func (m *Machine) methodType(rt *RType, name string) reflect.Type {
	fn := rt.Methods[name]
	if fn == nil {
		return nil
	}
	sig := fn.ssa.Signature
	return m.tryReflectType(types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic()))
}

// wrap boxes an interpreted value in the yaegi wrapper w.
func (m *Machine) wrap(iv *Iface, w *yaegiWrapper) reflect.Value {
	o := vmObj{m: m, iv: iv, origin: m.originAcct()}
	out := reflect.New(w.t).Elem()
	out.Field(0).Set(reflect.ValueOf(o))
	for i, name := range w.methods {
		f := out.Field(i + 1)
		name := name
		f.Set(reflect.MakeFunc(f.Type(), func(in []reflect.Value) []reflect.Value {
			res := o.call(name, in...)
			// unpack converts to the go/types results; the native ones may be
			// distinct but assignable types (interpreted named types).
			for j := range res {
				if t := f.Type().Out(j); res[j].Type() != t {
					if res[j].Type().ConvertibleTo(t) {
						res[j] = res[j].Convert(t)
					}
				}
			}
			return res
		}))
	}
	return out
}

// unwrapWrapper returns the interpreted value of m boxed in a yaegi wrapper.
func (m *Machine) unwrapWrapper(rv reflect.Value) (*Iface, bool) {
	if rv.Kind() != reflect.Struct || !isWrapperType(rv.Type()) {
		return nil, false
	}
	b, ok := rv.Field(0).Interface().(vmBoxed)
	if !ok || !m.owns(b) {
		return nil, false
	}
	return b.iface(), true
}
