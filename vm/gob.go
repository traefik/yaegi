package vm

// gob.Register and gob.RegisterName with interpreted types (default mode).
//
// Native gob cannot see interpreted types: a value crosses the boundary as
// a native copy of its underlying type (the "mirror", an unnamed struct for
// a struct type). gob.Register(User{}) called natively registered the
// mirror under its type string ("struct { Username string; ... }"), and a
// value decoded into an interface came back as that unnamed struct, so a
// type assertion to User failed (gorilla/sessions).
//
// The hooks below register the mirror under the name Go would compute for
// the interpreted type ("main.User", "*main.User"), so the encoded data is
// the same as with go run, and remember which interpreted type the mirror
// stands for. ifaceVM then turns a mirror value that native code gives back
// in an interface into a value of the interpreted type (gobType).
//
// Limits: two interpreted types with the same underlying type have the same
// mirror. Native gob knows one name per mirror, so the second one is not
// registered natively and its values are encoded and decoded as the first
// one. The same goes for a named type whose mirror gob already knows (type
// Tags []string: []string is registered by gob itself): it comes back as
// its underlying type. A value of a type with a String or Error method
// crosses the boundary in its hand-written wrapper (stringerW...), which gob
// cannot encode (the pointer given to Decoder.Decode is not wrapped:
// decodedst.go). Methods of interpreted types (GobEncode, MarshalBinary...)
// are not seen by native gob. The AOT
// copy of gob (-aot) sees the interpreted types and has none of these
// limits.

import (
	"encoding/gob"
	"fmt"
	"go/types"
	"reflect"
	"strings"
)

func init() {
	vmHooks["encoding/gob.Register"] = func(m *Machine, a []Value) Value {
		iv, _ := a[0].R.(*Iface)
		if !m.gobInterp(iv) {
			gob.Register(m.gobNative(iv))
			return Value{}
		}
		m.gobRegister(gobName(iv.T.T), iv)
		return Value{}
	}
	vmHooks["encoding/gob.RegisterName"] = func(m *Machine, a []Value) Value {
		name, _ := a[0].R.(string)
		iv, _ := a[1].R.(*Iface)
		if !m.gobInterp(iv) {
			gob.RegisterName(name, m.gobNative(iv))
			return Value{}
		}
		m.gobRegister(name, iv)
		return Value{}
	}
}

// gobInterp reports whether the registered value iv has an interpreted
// type that native gob would see as its mirror.
func (m *Machine) gobInterp(iv *Iface) bool {
	return iv != nil && iv.T.Native == nil && iv.T.T != nil && m.mentionsInterpNamed(iv.T.T)
}

// gobNative is the value native gob gets for a value without an
// interpreted type (as before the hooks).
func (m *Machine) gobNative(iv *Iface) any {
	if iv == nil {
		return nil
	}
	return m.box(iv, newConv(nil)).Interface()
}

// gobName is the name gob.Register gives to a value of type t: the package
// path and the name for a named type, the type string for other types
// ("*main.User": gob.Register does not strip the pointer for the name).
func gobName(t types.Type) string {
	if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil {
		p := n.Obj().Pkg()
		return reflectPkgPath(p) + "." + strings.TrimPrefix(typeString(t), p.Name()+".")
	}
	return typeString(t)
}

// gobBase is t without its pointers, the type gob uses to find the name of
// a value it encodes.
func gobBase(t types.Type) types.Type {
	for {
		p, ok := types.Unalias(t).(*types.Pointer)
		if !ok {
			return t
		}
		t = p.Elem()
	}
}

// gobRegister registers the mirror of the interpreted type of iv under
// name. It panics like gob on a name or a type registered twice with
// different values, checked on the interpreted types. When only the mirrors
// collide (see the limits above), the type is not registered natively.
func (m *Machine) gobRegister(name string, iv *Iface) {
	rt := m.reflectType(iv.T.T)
	base := m.rtypeOf(gobBase(iv.T.T))
	m.mu.Lock()
	if m.gobNames == nil {
		m.gobNames, m.gobBases = map[string]*RType{}, map[*RType]string{}
	}
	if old, dup := m.gobNames[name]; dup && old != iv.T {
		m.mu.Unlock()
		panic(fmt.Sprintf("gob: registering duplicate types for %q: %s != %s", name, typeString(old.T), typeString(iv.T.T)))
	}
	if old, dup := m.gobBases[base]; dup && old != name {
		m.mu.Unlock()
		panic(fmt.Sprintf("gob: registering duplicate names for %s: %q != %q", typeString(iv.T.T), old, name))
	}
	m.gobNames[name], m.gobBases[base] = iv.T, name
	m.mu.Unlock()
	if gobRegisterNative(name, reflect.New(rt).Elem().Interface()) {
		m.gobTypes.Store(rt, iv.T)
	}
}

// gobRegisterNative calls the native gob.RegisterName and reports false
// when gob refuses it: the mirror or the name is already registered for
// another value (by gob itself, by another interpreted type of the same
// shape, or by another program in the process).
func gobRegisterNative(name string, v any) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	gob.RegisterName(name, v)
	return true
}

// gobType returns, for a native value e given back by native code in an
// interface, the interpreted type registered with gob whose mirror is the
// type of e (nil when there is none).
func (m *Machine) gobType(e reflect.Value) *RType {
	if t, ok := m.gobTypes.Load(e.Type()); ok {
		return t.(*RType)
	}
	return nil
}
