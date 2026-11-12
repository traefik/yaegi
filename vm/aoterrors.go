package vm

// Errors of the AOT copies of routed packages (aotroute.go).
//
// In default mode the program is compiled against the native package: it
// checks the errors of encoding/json with errors.As(err, &*json.SyntaxError)
// and so on. A call routed to the AOT copy returns the error types of the
// copy, which are other types: errors.As failed. The error is given back as
// a value of the native type of the same name: both types come from the
// same source, so they have the same fields.

import (
	"encoding/json"
	"go/types"
	"reflect"
	"strings"
	"unsafe"
)

// routeErrTypes are the native error types of the routed packages, by name.
var routeErrTypes = map[string]map[string]reflect.Type{
	"encoding/json": {
		"SyntaxError":           reflect.TypeFor[json.SyntaxError](),
		"UnmarshalTypeError":    reflect.TypeFor[json.UnmarshalTypeError](),
		"InvalidUnmarshalError": reflect.TypeFor[json.InvalidUnmarshalError](),
		"UnsupportedTypeError":  reflect.TypeFor[json.UnsupportedTypeError](),
		"UnsupportedValueError": reflect.TypeFor[json.UnsupportedValueError](),
		// The message of a MarshalerError names the type of the value
		// (Type.String()): its Type is a namedType.
		"MarshalerError": reflect.TypeFor[json.MarshalerError](),
	},
}

var (
	reflectTypeT  = reflect.TypeFor[reflect.Type]()
	reflectValueT = reflect.TypeFor[reflect.Value]()

	reflectStructFieldT = reflect.TypeFor[reflect.StructField]()
)

// nativeRouteErrs replaces, in the results out of a call routed to the AOT
// copy of path, the errors of the copy with errors of the native types.
func (m *Machine) nativeRouteErrs(path string, out []reflect.Value) {
	if routeErrTypes[path] == nil || aotPkg(path) == nil {
		return
	}
	for i, r := range out {
		if r.Kind() != reflect.Interface || r.IsNil() || r.Type() != errorType {
			continue
		}
		if e := m.nativeRouteErr(path, r.Interface().(error)); e != nil {
			out[i] = reflect.ValueOf(&e).Elem()
		}
	}
}

// routeValTypes are the native basic named types of the routed packages
// whose values an AOT copy gives to the program in an interface (Token of a
// json.Decoder, json.Number with UseNumber), by name. In default mode the
// program knows the native types: the value takes the native type of the
// same name (routedValType).
var routeValTypes = map[string]map[string]reflect.Type{
	"encoding/json": {
		"Delim":  reflect.TypeFor[json.Delim](),
		"Number": reflect.TypeFor[json.Number](),
	},
}

// routedValType returns the native type of the same name as t, a basic named
// type of the AOT copy of a routed package listed in routeValTypes, or nil.
func (m *Machine) routedValType(t reflect.Type) reflect.Type {
	pp := t.PkgPath()
	if len(m.routed) == 0 || !strings.HasPrefix(pp, aotModPrefix) {
		return nil
	}
	for path := range m.routed {
		if p := aotPkg(path); p != nil && p.AOTPath == pp {
			return routeValTypes[path][t.Name()]
		}
	}
	return nil
}

// sameValType reports whether the native types a and b are a type listed in
// routeValTypes (json.Number, json.Delim) and the type of the same name of
// the AOT copy of its package. Native code can give a json.Number of the
// host to the AOT copy: a value of the program in default mode, where the
// program uses the host's json (aotroute.go), or a value made by native
// code. The copy compares the types of values with its own Number
// (v.Type() == numberType): without this, it encoded a json.Number field as
// the string "5" instead of the number 5, and refused to decode a number
// into it. Both types have the same kind and the same methods, so the copy
// can treat one as the other.
//
// The AOT copy of json compares the type of every string value with its
// Number (EqualAny): the host type is found by identity first, so that the
// common case (two other types) costs two comparisons and no reflect call.
func sameValType(a, b reflect.Type) bool {
	host, path := routeValTypeOf(a)
	if host == nil {
		if host, path = routeValTypeOf(b); host == nil {
			return false
		}
		a, b = b, a // a is the host type, b the candidate copy
	}
	if a.Kind() != b.Kind() || a.Name() != b.Name() {
		return false
	}
	pb := b.PkgPath()
	return strings.HasPrefix(pb, aotModPrefix) && origPath(pb) == path
}

// routeValEntry is a native type of routeValTypes and its package.
type routeValEntry struct {
	t    reflect.Type
	path string
}

// routeValList is routeValTypes as a list.
var routeValList = func() (l []routeValEntry) {
	for path, ts := range routeValTypes {
		for _, t := range ts {
			l = append(l, routeValEntry{t, path})
		}
	}
	return l
}()

// routeValTypeOf returns t and its package when t is a native type of
// routeValTypes, else nil.
func routeValTypeOf(t reflect.Type) (reflect.Type, string) {
	for _, e := range routeValList {
		if e.t == t {
			return t, e.path
		}
	}
	return nil, ""
}

// copyVal returns the native value x of a type of routeValTypes as a value
// of the type of the same name of the AOT copy, for AOT code (the reverse of
// routedValType), or x. A json.Number that a Decoder gave to the program
// (as a native json.Number, routedValType) went back to the copy of json in
// json.Marshal: the copy compares the type with its own Number, did not
// find it, and wrote the number as a string ({"id":7} became {"id":"7"}).
func copyVal(x any) any {
	rv := reflect.ValueOf(x)
	if c := copyRV(rv); c != rv {
		return c.Interface()
	}
	return x
}

// isRouteValType reports whether t is a native type of routeValTypes.
func isRouteValType(t reflect.Type) bool {
	return t.PkgPath() != "" && routeValTypes[t.PkgPath()][t.Name()] == t
}

// copyRV is copyVal for the dynamic value rv of an interface: rv itself,
// or a new value of the type of the copy.
func copyRV(rv reflect.Value) reflect.Value {
	if !rv.IsValid() || rv.Type().PkgPath() == "" {
		return rv
	}
	path := rv.Type().PkgPath()
	nt := routeValTypes[path][rv.Type().Name()]
	if nt != rv.Type() {
		return rv
	}
	p := aotPkg(path)
	if p == nil {
		return rv
	}
	ct, ok := p.Types[nt.Name()]
	if !ok || !rv.CanConvert(ct) {
		return rv
	}
	return rv.Convert(ct)
}

// copyValTypeOf returns the type of the AOT copy for t, a native type of
// routeValTypes or a pointer to one, else nil. json/v2 (Go 1.27) finds the
// methods of json.Number (MarshalJSONTo, UnmarshalJSONFrom) through its
// interfaces, whose methods take the copy's jsontext types: the host's
// Number does not implement them, and the copy wrote a *json.Number field
// of a plugin struct as a string and refused to decode a number into it.
func copyValTypeOf(t reflect.Type) reflect.Type {
	ptr := t.Kind() == reflect.Pointer
	if ptr {
		t = t.Elem()
	}
	if !isRouteValType(t) {
		return nil
	}
	p := aotPkg(t.PkgPath())
	if p == nil {
		return nil
	}
	ct, ok := p.Types[t.Name()]
	if !ok {
		return nil
	}
	if ptr {
		return reflect.PointerTo(ct)
	}
	return ct
}

// copyValAny returns x, a value of a native type of routeValTypes or a
// pointer to one, as the type of the AOT copy (copyValTypeOf). A pointer
// keeps its address: the methods of the copy write the host's value.
func copyValAny(x any) (any, bool) {
	rv := reflect.ValueOf(x)
	if !rv.IsValid() {
		return nil, false
	}
	ct := copyValTypeOf(rv.Type())
	if ct == nil {
		return nil, false
	}
	if ct.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return reflect.Zero(ct).Interface(), true
		}
		return reflect.NewAt(ct.Elem(), rv.UnsafePointer()).Interface(), true
	}
	return rv.Convert(ct).Interface(), true
}

// aotModPrefix is the start of the import paths of the AOT copies.
const aotModPrefix = "github.com/traefik/yaegi/v2/aot/"

// nativeRouteErr returns err, a *T of the AOT copy of path, as a *T of the
// native package, field by field (unexported ones too: SyntaxError.msg).
// A reflect.Type or a reflect.Value of the twin gives its native
// counterpart (the underlying native type for an interpreted type, as
// native json gave). It returns nil when err is not such an error.
func (m *Machine) nativeRouteErr(path string, err error) error {
	rv := reflect.ValueOf(err)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return nil
	}
	src := rv.Elem()
	p := aotPkg(path)
	if p == nil || src.Type().PkgPath() != p.AOTPath {
		return nil
	}
	nt, ok := routeErrTypes[path][src.Type().Name()]
	if !ok {
		return nil
	}
	dst := reflect.New(nt).Elem()
	for i := 0; i < nt.NumField(); i++ {
		df := nt.Field(i)
		sf := src.FieldByName(df.Name)
		if !sf.IsValid() {
			continue
		}
		sv := reflect.NewAt(sf.Type(), unsafe.Pointer(sf.UnsafeAddr())).Elem()
		dv := reflect.NewAt(df.Type, unsafe.Pointer(dst.Field(i).UnsafeAddr())).Elem()
		switch {
		case sv.Type().AssignableTo(df.Type):
			dv.Set(sv)
		case df.Type == reflectTypeT:
			if t, ok := sv.Interface().(RT); ok && t != nil {
				dv.Set(reflect.ValueOf(nativeType(t)))
			}
		case df.Type == reflectValueT:
			if x, ok := sv.Interface().(RV); ok {
				if n, ok := x.native(); ok {
					dv.Set(reflect.ValueOf(n))
				}
			}
		}
	}
	return dst.Addr().Interface().(error)
}

// namedType is the native type of a value of an interpreted type (nil when
// there is none), with the name of the interpreted type: the field Type of
// the errors of a routed call. The messages of the errors print
// Type.String(): the native type alone made "json: cannot unmarshal string
// into Go struct field Wrapper.err of type struct { Message string ... }"
// where gc prints "of type main.ResponseError". Name, String and PkgPath
// give the names of the interpreted type; the other methods are those of
// the native type (a reflect.Type has unexported methods: only a type that
// embeds one implements it). So the errors keep their native types
// (errors.As and a type assertion work) and the messages of gc. Native
// reflect functions do not take a namedType (reflect.New needs a type of
// the runtime): when the program reads the field, it gets its own
// reflect.Type of the interpreted type gt (natTypeVal), on which
// reflect.New, == and PkgPath work as in gc.
type namedType struct {
	reflect.Type
	gt             types.Type // the interpreted type
	rt             RT         // its twin type
	name, str, pkg string
}

func (t namedType) Name() string    { return t.name }
func (t namedType) String() string  { return t.str }
func (t namedType) PkgPath() string { return t.pkg }

// nativeType returns the native reflect.Type that stands for the twin type
// t in native memory (a field of type reflect.Type): the native type of the
// values, or a namedType with the names of t when the native type has
// other names (an interpreted type). nil for nil.
func nativeType(t RT) reflect.Type {
	if t == nil {
		return nil
	}
	if t.nat != nil {
		return t.nat
	}
	nt := t.vr.m.nativeTypeOf(t)
	if nt == nil || nt.Name() != t.Name() || nt.String() != t.String() {
		return namedType{nt, t.t, t, t.Name(), t.String(), t.PkgPath()}
	}
	return nt
}

// twinType is the reverse of nativeType: the twin type of the native type
// rt read from native memory (a reflect.StructField with -aot).
func twinType(rt reflect.Type) RT {
	if nt, ok := rt.(namedType); ok && nt.rt != nil {
		return nt.rt
	}
	return natT(rt)
}

// nativeTypeOf returns the native type of the twin type t: itself for a
// native type, the native type of the values for an interpreted one.
func (m *Machine) nativeTypeOf(t RT) reflect.Type {
	if t.nat != nil {
		return t.nat
	}
	if t.t != nil {
		return m.reflectType(t.t)
	}
	return nil
}
