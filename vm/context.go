package vm

// context.WithValue and Context.Value with interpreted keys and values.
//
// Passed to native context code, an interpreted key became a native copy
// (default mode: every empty struct key type turned into the same struct{},
// so distinct keys collided) or a fresh opaque handle (AOT mode: never equal
// to the handle of the next call, so Value found nothing), and an
// interpreted value with methods came back without its type (r12a, r12d).
// WithValue is replaced by vmValueCtx, a native context that keeps the key
// and the value as VM values, and interpreted calls of Value on native
// contexts pass the key in a form that compares correctly on both sides:
//
//   - keys of a native type, or of a predeclared type (string, int...), as
//     native values, so that they match the keys of native code;
//   - keys of an interpreted named type as a vmKey, which vmValueCtx
//     compares with VM equality and no native key equals.

import (
	"context"
	"fmt"
	"go/types"
	"net/http"
	"reflect"
	"strings"
)

// vmValueCtx is the context returned by context.WithValue called from
// interpreted code.
type vmValueCtx struct {
	context.Context // parent
	m               *Machine
	key, val        Value // interpreted values of type any (*Iface or nil)
	nkey            any   // the key as native code passes it (keyNative)
}

// vmKey carries an interpreted key of an interpreted named type through
// native contexts (cancelCtx.Value delegates to its parent with it).
type vmKey struct {
	m  *Machine
	iv *Iface
}

// vmKeyRT is the native type of vmKey.
var vmKeyRT = reflect.TypeFor[vmKey]()

func init() {
	vmHooks["context.WithValue"] = func(m *Machine, a []Value) Value {
		parent, _ := m.toNative(a[0], m.contextType()).Interface().(context.Context)
		if parent == nil {
			panic("cannot create context from nil parent")
		}
		kiv, _ := a[1].R.(*Iface)
		if kiv == nil {
			panic("nil key")
		}
		if kiv.T.T != nil && !types.Comparable(kiv.T.T) || kiv.T.T == nil && !kiv.T.Native.Comparable() {
			panic("key is not comparable")
		}
		return m.natIface(&vmValueCtx{Context: parent, m: m, key: a[1], val: a[2], nkey: m.keyNative(kiv)})
	}
}

// contextType is context.Context.
func (m *Machine) contextType() types.Type {
	m.ctxTypeOnce.Do(func() {
		if p := m.prog.ImportedPackage("context"); p != nil {
			m.ctxType = p.Pkg.Scope().Lookup("Context").Type()
		}
	})
	if m.ctxType == nil {
		panic("context.WithValue without package context")
	}
	return m.ctxType
}

// keyNative returns the native form of an interpreted context key. A key
// of a native type keeps it, also when only go/types knows that type
// (http.LocalAddrContextKey is a *net/http.contextKey, an unexported type
// without a native RType: iv.T.Native is nil but the VM holds the native
// pointer), and a value of a named native type held in VM form
// (time.Duration(3)) becomes the native value. Without this, such keys
// became a vmKey that no native context matches: a plugin read nil for
// Value(http.LocalAddrContextKey) behind a real server, and native code
// did not see the values that a plugin stored with these keys.
//
// A restricted machine never gets the native http.ServerContextKey: with
// the *http.Server of the host, a plugin could set its Handler, and every
// later request of the entry point would run plugin code. The key stays a
// vmKey of the machine, so ctx.Value(http.ServerContextKey) is nil unless
// the plugin stored a value with it itself.
func (m *Machine) keyNative(iv *Iface) any {
	k := m.keyNativeOf(iv)
	if m.restricted && k == any(http.ServerContextKey) {
		return vmKey{m: m, iv: iv}
	}
	return k
}

// keyNativeOf is keyNative without the restriction on the server key.
func (m *Machine) keyNativeOf(iv *Iface) any {
	if rv, ok := iv.V.R.(reflect.Value); ok && rv.IsValid() && rv.CanInterface() && (iv.T.Native != nil || iv.T.T == nil || !m.mentionsInterpNamed(iv.T.T)) {
		return rv.Interface()
	}
	if iv.T.Native != nil {
		return iv.V.R.(reflect.Value).Interface()
	}
	t := iv.T.T
	if p, ok := types.Unalias(t).(*types.Pointer); m.isNativeNamed(t) || ok && m.isNativeNamed(p.Elem()) {
		if rv := m.toNative(iv.V, t); rv.IsValid() && rv.CanInterface() && rv.Type().Comparable() {
			return rv.Interface()
		}
	}
	if _, ok := types.Unalias(t).(*types.Basic); ok {
		return m.box(iv, newConv(nil)).Interface()
	}
	return vmKey{m: m, iv: iv}
}

// Value implements context.Context for native callers (and for the native
// contexts derived from c, which delegate to it).
func (c *vmValueCtx) Value(key any) any {
	if c.matches(key) {
		return c.nativeVal(ctxAsker())
	}
	return c.Context.Value(key)
}

// matches reports whether the native key is the key of c. A key of an
// interpreted named type is only a key of its own machine: the types of
// two machines are never the same type.
func (c *vmValueCtx) matches(key any) bool {
	switch k := key.(type) {
	case vmKey:
		return k.m == c.m && c.m.equal(c.key, Value{R: k.iv}, types.Universe.Lookup("any").Type())
	case vmBoxed:
		return k.obj().m == c.m && c.m.equal(c.key, Value{R: k.iface()}, types.Universe.Lookup("any").Type())
	}
	if _, ok := c.nkey.(vmKey); ok {
		return false
	}
	return reflect.TypeOf(key) == reflect.TypeOf(c.nkey) && reflect.TypeOf(key).Comparable() && key == c.nkey
}

// nativeVal is the stored value as the asker sees it. asker is the machine
// whose interpreted code called Value (ctxValue), or nil for native code.
//
// For its own machine, a value is a handle that converts back to the
// original VM value (the same slice, map or pointer, as in Go). For native
// code and for the other machines of the host (Traefik chains plugins, one
// machine each), it is the native form of the value: a native copy for a
// value of a type without names of the program ([]string, map[string]any,
// struct{ ID int }, func(string) string), a wrapper with its methods
// (stringerW, errorW...) for a value with methods. A value of a named type
// of the program without a hand wrapper stays a handle: another machine
// sees its methods through the handle (foreignRType), native code sees
// its Format and MarshalJSON. A value of a native named type, or a pointer
// to one, is the native value itself (of the real package, not of its AOT
// copy: hostTwin). The values nested in interfaces are converted the same
// way, also in AOT mode. Before, every value that was not basic was
// a handle: the host read false for ctx.Value("sl").([]string), and a
// plugin could not assert the claims of the plugin before it.
func (c *vmValueCtx) nativeVal(asker *Machine) any {
	iv, _ := c.val.R.(*Iface)
	switch {
	case iv == nil:
		return nil
	case iv.T.Native != nil:
		return iv.V.R.(reflect.Value).Interface()
	}
	if _, ok := types.Unalias(iv.T.T).(*types.Basic); ok {
		return c.m.box(iv, newConv(nil)).Interface()
	}
	if asker == c.m {
		return vmOpaque{vmObj{m: c.m, iv: iv}}
	}
	if p, ok := types.Unalias(iv.T.T).(*types.Pointer); c.m.isNativeNamed(iv.T.T) || ok && c.m.isNativeNamed(p.Elem()) {
		// A value of a native named type, or a pointer to one (time.Time,
		// http.Header, *url.URL, *sync.Map): the native value, as for keys
		// (keyNativeOf). A pointer aliases the VM cell (boxAs), so the
		// changes of the next ones are seen. Before, a type with methods
		// became a hand wrapper (stringerW for time.Time) and one without
		// a handle: ctx.Value("t").(time.Time) was false for the host and
		// the next plugin.
		if nv := c.m.boxAs(iv, ctxConv(), false); nv.IsValid() && nv.CanInterface() {
			return c.m.hostTwin(iv.T.T, nv).Interface()
		}
	}
	o := vmObj{m: c.m, iv: iv}
	if len(iv.T.Methods) > 0 {
		if hw, ok := c.m.handWrapper(o); ok {
			return hw.Interface()
		}
		return vmOpaque{o}
	}
	if c.m.mentionsInterpNamed(iv.T.T) {
		return vmOpaque{o}
	}
	// ctxConv: the values nested in interfaces are native copies too, also
	// in AOT mode (OpaqueAny). Before, jwt["groups"] of a map[string]any
	// was a handle for the host and the next plugin with -aot, and their
	// jwt["groups"].([]any) was false. A nested value of the program with
	// methods stays a handle, as at the top level.
	if nv := c.m.boxAs(iv, ctxConv(), false); nv.IsValid() && nv.CanInterface() {
		return nv.Interface()
	}
	return vmOpaque{o}
}

// ctxConv is the conversion of a context value: native copies at every
// level, not opaque handles (convGraph.plain), except for a nested value
// of the program with methods and no hand wrapper, which stays a handle of
// its machine (convGraph.owner). Before, with plain only, such a value in
// a map[string]any lost its methods: m["t"].(interface{ Tag() string })
// was false in the next plugin.
func ctxConv() *natConv {
	c := newConv(nil)
	c.g.plain, c.g.owner = true, true
	return c
}

// hostTwin converts rv, the native value of a value of type t, from a type
// of an AOT copy of a standard package to the same type of the real
// package, which the host knows: with -aot, the json.Number of a plugin is
// the json.Number of the copy of encoding/json, and the assertion of the
// host to json.Number was false. A value that does not convert (a struct
// with fields of types of the copy) stays as it is.
func (m *Machine) hostTwin(t types.Type, rv reflect.Value) reflect.Value {
	rt := rv.Type()
	et := rt
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
		if rt.Kind() != reflect.Pointer {
			return rv
		}
		et = rt.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || !strings.HasPrefix(et.PkgPath(), aotModPrefix) {
		return rv
	}
	sym, ok := m.stdSym(n.Obj().Pkg(), n.Obj().Name())
	if !ok || sym.Kind() != reflect.Pointer {
		return rv
	}
	dt := sym.Type().Elem()
	if et != rt {
		dt = reflect.PointerTo(dt)
	}
	if cv, ok := convertTwin(rv, dt); ok {
		return cv
	}
	return rv
}

// String is the String of context's valueCtx: the parent, then the key and
// the value with their String method, as a string, or as the name of
// their type.
func (c *vmValueCtx) String() string {
	return fmt.Sprintf("%v.WithValue(%s, %s)", c.Context, c.stringify(c.key), c.stringify(c.val))
}

// stringify is the stringify of package context for the interpreted value
// v of type any.
func (c *vmValueCtx) stringify(v Value) string {
	iv, _ := v.R.(*Iface)
	switch {
	case iv == nil:
		return "<nil>"
	case iv.T.Native != nil:
		switch s := iv.V.R.(reflect.Value).Interface().(type) {
		case interface{ String() string }:
			return s.String()
		case string:
			return s
		}
	default:
		o := vmObj{m: c.m, iv: iv}
		if _, ok := iv.T.Methods["String"]; ok && o.stringMethod("String") {
			return o.call("String")[0].String()
		}
		if types.Identical(iv.T.T, types.Typ[types.String]) {
			return iv.V.R.(string)
		}
	}
	return c.m.dynTypeString(v)
}

// ctxValue implements ctx.Value(key) called from interpreted code on a
// native context: the chain of vmValueCtx is walked with VM equality, and
// native contexts get the key in its native form.
//
// The walk stops at a vmValueCtx of another machine: m.equal never holds
// for its key (each machine has its own types), and the native call that
// follows asks it with the native key. Before, the walk went on past it:
// a string key that plugin A stored was nil in plugin B.
func (m *Machine) ctxValue(ctx context.Context, key Value) Value {
	if iv, _ := key.R.(*Iface); iv != nil && iv.T.Native == vmKeyRT {
		// The key went through native code and came back: a context
		// wrapper of the program (type pass struct{ context.Context })
		// under a native context gets the vmKey that the native context
		// got (WithValue(pass{c}, "o", 1).Value(k)), and asks its embedded
		// context with it. The walk compares VM keys: the vmKey of m is
		// its key again. Before, no key of a named, pointer or struct type
		// was found through such a wrapper.
		if k := iv.V.R.(reflect.Value).Interface().(vmKey); k.m == m {
			key = Value{R: k.iv}
		}
	}
	for {
		c, ok := ctx.(*vmValueCtx)
		if !ok || c.m != m {
			break
		}
		if m.equal(c.key, key, types.Universe.Lookup("any").Type()) {
			return c.val
		}
		ctx = c.Context
	}
	var nk any
	if iv, _ := key.R.(*Iface); iv != nil {
		nk = m.keyNative(iv)
	}
	defer setCtxAsker(m)()
	nv := m.copyTwin(ctx.Value(nk))
	return m.fromNative(reflect.ValueOf(&[]any{nv}[0]).Elem(), types.Universe.Lookup("any").Type())
}

// copyTwin is the reverse of hostTwin: rv, a value of a type of a real
// standard package (or a pointer to one), becomes a value of the same
// type of the AOT copy when m uses the copy for that package. A
// json.Number that the host or another machine stored in the context is
// then a json.Number for the plugin.
func (m *Machine) copyTwin(v any) any {
	if v == nil {
		return nil
	}
	rt := reflect.TypeOf(v)
	d, ok := m.copyTwins.Load(rt)
	if !ok {
		d, _ = m.copyTwins.LoadOrStore(rt, m.copyTwinType(rt))
	}
	dt, _ := d.(reflect.Type)
	if dt == nil {
		return v
	}
	if cv, ok := convertTwin(reflect.ValueOf(v), dt); ok {
		return cv.Interface()
	}
	return v
}

// copyTwinType is the type of the AOT copy for copyTwin, or nil (a
// reflect.Type in an any, for the cache).
func (m *Machine) copyTwinType(rt reflect.Type) any {
	et := rt
	if rt.Kind() == reflect.Pointer {
		et = rt.Elem()
	}
	path := et.PkgPath()
	if et.Name() == "" || path == "" || strings.HasPrefix(path, aotModPrefix) || m.modeOf(path) != ModeAOT {
		return nil
	}
	// The package from the import graph, not only from the SSA packages:
	// stdSSA creates no SSA package for a standard library package that the
	// plugin does not import itself. With Go 1.27 (jsonv2), json.RawMessage
	// is an alias of jsontext.Value, and a plugin that imports only
	// encoding/json has no SSA package for encoding/json/jsontext: the
	// RawMessage of the context kept the host type, and the assertion
	// v.(json.RawMessage) of the plugin was false.
	var tp *types.Package
	if p := m.prog.ImportedPackage(path); p != nil {
		tp = p.Pkg
	} else if m.vr != nil {
		tp = m.vr.typesPkg(path)
	}
	if tp == nil {
		return nil
	}
	tn, ok := tp.Scope().Lookup(et.Name()).(*types.TypeName)
	if !ok {
		return nil
	}
	dt := m.tryReflectType(tn.Type())
	if dt == nil || dt == et {
		return nil
	}
	if et != rt {
		dt = reflect.PointerTo(dt)
	}
	return dt
}

// setCtxAsker records that interpreted code of m calls Value on a native
// context on this goroutine, until the returned func runs. A vmValueCtx
// of m deeper in the chain then gives its value as a handle of m, which
// converts back to the same VM value (nativeVal).
func setCtxAsker(m *Machine) func() {
	gs := lookupG(getg())
	if gs == nil {
		return func() {}
	}
	prev := gs.ctxAsker
	gs.ctxAsker = m
	return func() { gs.ctxAsker = prev }
}

// ctxAsker is the machine recorded by setCtxAsker on this goroutine, or nil
// when native code calls Value. It is read once: a native context between
// the asker and the vmValueCtx does not call interpreted code, and a
// nested ctxValue sets its own.
func ctxAsker() *Machine {
	gs := lookupG(getg())
	if gs == nil {
		return nil
	}
	return gs.ctxAsker
}
