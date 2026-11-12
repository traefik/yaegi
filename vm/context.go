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
	"reflect"
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

// keyNative returns the native form of an interpreted context key.
func (m *Machine) keyNative(iv *Iface) any {
	if iv.T.Native != nil {
		return iv.V.R.(reflect.Value).Interface()
	}
	if _, ok := types.Unalias(iv.T.T).(*types.Basic); ok {
		return m.box(iv, newConv(nil)).Interface()
	}
	return vmKey{m: m, iv: iv}
}

// Value implements context.Context for native callers (and for the native
// contexts derived from c, which delegate to it).
func (c *vmValueCtx) Value(key any) any {
	if c.matches(key) {
		return c.nativeVal()
	}
	return c.Context.Value(key)
}

func (c *vmValueCtx) matches(key any) bool {
	switch k := key.(type) {
	case vmKey:
		return c.m.equal(c.key, Value{R: k.iv}, types.Universe.Lookup("any").Type())
	case vmBoxed:
		return c.m.equal(c.key, Value{R: k.iface()}, types.Universe.Lookup("any").Type())
	}
	if _, ok := c.nkey.(vmKey); ok {
		return false
	}
	return reflect.TypeOf(key) == reflect.TypeOf(c.nkey) && reflect.TypeOf(key).Comparable() && key == c.nkey
}

// nativeVal is the stored value as native code sees it; it converts back to
// the original VM value (vmBoxed) when it returns to interpreted code.
func (c *vmValueCtx) nativeVal() any {
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
	return vmOpaque{vmObj{m: c.m, iv: iv}}
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
func (m *Machine) ctxValue(ctx context.Context, key Value) Value {
	for {
		c, ok := ctx.(*vmValueCtx)
		if !ok {
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
	return m.fromNative(reflect.ValueOf(&[]any{ctx.Value(nk)}[0]).Elem(), types.Universe.Lookup("any").Type())
}
