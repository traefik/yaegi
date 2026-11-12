package vm

// Optional methods of the values that native code gets in an interface.
// The standard library looks for methods beyond the interface of a
// parameter with type assertions: io.Copy uses WriteTo of the source and
// ReadFrom of the destination, flag treats a Value with IsBoolFlag as a
// bool flag, errors.Is follows Unwrap and asks Is, net/http retries when
// Timeout says so (docs/BOUNDARY.md lists every such place, by the static
// type of the value: the "detail" of its assert rows).
//
// The generic wrappers (wrappers.go) have only the methods of their
// interface, so native code did not see the other methods of a plugin
// type. Go cannot make a type with methods at run time, so, as for
// http.ResponseWriter (rwwriter.go), the wrappers are static types: for
// each family below (a base interface and the optional methods that the
// standard library looks for on it), one type per combination of the
// optional methods (zz_optcombos.go, made by optcombos_gen.go). The
// wrapper of a plugin value has exactly the optional methods of its
// dynamic type, as in Go: a type without WriteTo is not an io.WriterTo.
//
// The families (optcombos_gen.go): Reader (io.Reader with WriteTo,
// ReadByte, UnreadByte, ReadRune, UnreadRune, Close, Seek), ReadWriter
// (io.ReadWriter with WriteTo, ReadFrom, Close, Seek, and CloseWrite: the
// body of a 101 response), Writer
// (io.Writer with ReadFrom, WriteString, WriteByte, Close, Flush() error),
// Error (error with Unwrap, Is, As, Timeout, Temporary, String, LogValue),
// LogValuer (slog.LogValuer with String), FlagValue
// (flag.Value with IsBoolFlag and Get), Source (math/rand.Source with
// Uint64), FS (io/fs.FS with ReadDir, ReadFile, Stat, Glob, Sub, ReadLink,
// Lstat), File (fs.File with ReadDir, Readdir, Seek, ReadAt: also an
// http.File), Context (context.Context with AfterFunc), RoundTripper
// (http.RoundTripper with CancelRequest, CloseIdleConnections, and the
// RoundTripErr and NewClientConn of a TLSNextProto transport), ClientConn
// (the connection of NewClientConn: RoundTrip, Close, Err, Reserve,
// Release, Available and InFlight, with Ping and the optional methods of
// RoundTripper), Listener (net.Listener with
// the HTTP2Config, IdleTimeout and ServeConnFunc of Go 1.27), Image
// (image.Image with Opaque, RGBA64At, ColorIndexAt, Set and SetRGBA64: also
// an image.PalettedImage or a draw.Image), Signer (crypto.Signer with
// Decrypt, SignMessage and Equal) and Decrypter (crypto.Decrypter with
// Equal), Conn (net.Conn with the ReadFrom and WriteTo of a
// net.PacketConn, CloseWrite, the ReadFrom and WriteTo of io, and the
// ConnectionState and HandshakeContext of a TLS conn), Block
// (cipher.Block with NewCBCEncrypter, NewCBCDecrypter, NewCTR and NewGCM),
// Curve (elliptic.Curve with Unmarshal and UnmarshalCompressed), Hash
// (hash.Hash with MarshalBinary, UnmarshalBinary and Clone).
//
// Known limits: methods that are only optimizations
// are not added (Peek of a reader for image.Decode, SyscallConn for
// TCPConn.ReadFrom): the result is the same, only slower. net/http's
// RegisterProtocol reads the fields of a RoundTripper struct with reflect,
// which sees the wrapper.
//
// A plugin value given as `any` gets the wrapper of a family too, when no
// hand-written wrapper fits (handWrapper) and native code does not only
// print or decode it: crypto/tls and crypto/x509 take the private key as a
// crypto.PrivateKey (any) and look for crypto.Signer and crypto.Decrypter.

//go:generate go run optcombos_gen.go

import (
	"context"
	"fmt"
	"go/types"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"weak"
)

// optBase holds the plugin value of a wrapper of a family that does not
// embed vmObj: the wrapper then has no method of its own (Format,
// MarshalJSON), only the methods of the plugin type. It comes back
// unchanged when native code gives the wrapper back (vmBoxed).
type optBase struct {
	k any // first: == compares it first (see optKey)
	o vmObj
}

func (b optBase) iface() *Iface { return b.o.iv }
func (b optBase) obj() vmObj    { return b.o }

// optMethod is a method of a family, with its native type without the
// receiver.
type optMethod struct {
	name string
	typ  reflect.Type
}

// optCombo is the wrapper type of one combination of optional methods,
// and its constructor. A combination of two methods with the same name
// has none (t is nil).
type optCombo struct {
	t    reflect.Type
	make func(o vmObj, k any) any
}

// uncomparable is the k of the wrappers of a type that is not comparable:
// == of two such wrappers in native code panics, as == of the plugin
// values does in Go (image/draw compares dst == src). The message names
// []struct {}, not the plugin type.
var uncomparable any = []struct{}(nil)

// serveConnFunc is the parameter of ServeConnFunc of the Listener family
// (net/http's http2ExternalServerConfig): one name, as optcombos_gen.go
// splits the parameters at the commas.
type serveConnFunc = func(context.Context, net.Conn, http.Handler, bool, *http.Request, []byte)

// optKey returns the k field of a wrapper of rt: nil for a comparable
// type, uncomparable otherwise. Two wrappers of the same interpreted value
// are equal, as the wrapper also holds the *Iface.
func optKey(rt *RType) any {
	if rt.T != nil && !types.Comparable(rt.T) {
		return uncomparable
	}
	return nil
}

type optFamily struct {
	name   string
	base   []optMethod
	feats  []optMethod // bit i of a mask is feats[i]
	combos []optCombo  // by mask
}

var optFamilyByName = sync.OnceValue(func() map[string]*optFamily {
	r := map[string]*optFamily{}
	for _, f := range optFamilies {
		r[f.name] = f
	}
	return r
})

// optMask returns the mask of the optional methods of rt in the family f,
// and false when rt does not have every base method of f. The signatures
// must be those of Go: a Flush() method does not make the Flush() error of
// the Writer family.
func (m *Machine) optMask(rt *RType, f *optFamily) (uint, bool) {
	for _, b := range f.base {
		if mt := m.methodType(rt, b.name); mt != b.typ && !(b.name == "LogValue" && isLogValueSig(mt)) {
			return 0, false
		}
	}
	var mask uint
	for i, o := range f.feats {
		if mt := m.methodType(rt, o.name); mt == o.typ || o.name == "LogValue" && isLogValueSig(mt) {
			mask |= 1 << i
		}
	}
	return mask, true
}

// isLogValueSig reports whether the method type t is func() slog.Value of
// the native log/slog or of its AOT copy. In AOT mode the LogValue method of
// a plugin type returns the slog.Value of the copy: the wrapper converts it
// (logValue), so that the host's log/slog does not lose the method.
func isLogValueSig(t reflect.Type) bool {
	if t == nil || t.Kind() != reflect.Func || t.NumIn() != 0 || t.NumOut() != 1 {
		return false
	}
	v := t.Out(0)
	return v == logValueType || v.Name() == "Value" && strings.HasPrefix(v.PkgPath(), aotPkgPrefix) && strings.HasSuffix(v.PkgPath(), "/log/slog")
}

// logValueType is the native slog.Value.
var logValueType = reflect.TypeFor[slog.Value]()

// aotPkgPrefix is the start of the paths of the AOT copies.
const aotPkgPrefix = "github.com/traefik/yaegi/v2/aot/"

// logValue calls the LogValue method of the plugin value of o, for the
// native log/slog: the host logs a plugin value, or a plugin without the
// AOT copy of log/slog logs one. Without it, native slog printed the fields
// of the value, the secret that LogValue hides. A slog.Value of the AOT
// copy (a plugin in AOT mode) cannot become a native one: the native value
// is the text of the resolved value, so what LogValue hides stays hidden.
func (o vmObj) logValue() slog.Value {
	if o.m == nil {
		if h, ok := o.host(); ok {
			if lv, ok := h.Interface().(slog.LogValuer); ok {
				return lv.LogValue()
			}
		}
		return slog.Value{}
	}
	out := o.call("LogValue")
	if len(out) != 1 || !out[0].IsValid() {
		return slog.Value{}
	}
	r := out[0]
	if r.Type() == logValueType {
		return r.Interface().(slog.Value)
	}
	if res := r.MethodByName("Resolve"); res.IsValid() && res.Type().NumIn() == 0 && res.Type().NumOut() == 1 {
		r = res.Call(nil)[0]
	}
	if s, ok := r.Interface().(fmt.Stringer); ok {
		return slog.StringValue(s.String())
	}
	return slog.StringValue(fmt.Sprint(r.Interface()))
}

// optLogValuer returns the wrapper of the LogValuer family for o, a plugin
// value with a LogValue method given to native code as any (handWrapper).
func (m *Machine) optLogValuer(o vmObj) (reflect.Value, bool) {
	if !isLogValueSig(m.methodType(o.iv.T, "LogValue")) {
		return reflect.Value{}, false
	}
	f := optFamilyByName()["LogValuer"]
	mask, ok := m.optMask(o.iv.T, f)
	if !ok || f.combos[mask].t == nil {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(f.combos[mask].make(vmObj{m: m, iv: m.optPayload(o.iv), origin: o.origin}, optKey(o.iv.T))), true
}

// optFor returns the wrapper with optional methods for the plugin type rt
// given to native code as the interface target, or nil when no family
// fits: no family has its base methods, or the wrapper does not implement
// target. A wrapper that has every method of need (the methods of the
// generic wrapper of rt, see optWrapper) wins over one that has not. A
// type without optional methods gets the wrapper of the family too: a
// generic wrapper holds funcs and is not comparable, so == in native code
// panicked for any plugin value (runtime/trace compares its io.Writer),
// where the family wrapper compares as the plugin value does (optKey).
// Then the wrapper with the most methods wins. The cache is by rt and
// target: need depends only on them.
func (m *Machine) optFor(rt *RType, target reflect.Type, need []string) *optCombo {
	k := wrapKey{rt, target}
	if c, ok := m.optCache.Load(k); ok {
		return c.(*optCombo)
	}
	var best *optCombo
	bestCovers := false
	for _, f := range optFamilies {
		mask, ok := m.optMask(rt, f)
		if !ok {
			continue
		}
		c := &f.combos[mask]
		if c.t == nil || !c.t.Implements(target) {
			continue
		}
		if covers := optHasAll(c.t, need); best == nil || covers && !bestCovers ||
			covers == bestCovers && c.t.NumMethod() > best.t.NumMethod() {
			best, bestCovers = c, covers
		}
	}
	m.optCache.Store(k, best)
	return best
}

// optWrapper boxes iv, a plugin value given to native code as the
// interface target, in the wrapper with its optional methods. ok is false
// when no wrapper with optional methods fits: the generic wrappers are
// then used, as before.
func (m *Machine) optWrapper(iv *Iface, target reflect.Type) (reflect.Value, bool) {
	if target.NumMethod() == 0 || len(iv.T.Methods) == 0 || m.isNativeNamed(iv.T.T) || m.vr != nil && iv.T == m.vr.typeRT {
		return reflect.Value{}, false
	}
	if p, ok := iv.T.T.(*types.Pointer); ok && m.isNativeNamed(p.Elem()) {
		return reflect.Value{}, false
	}
	// The wrapper of a family should have the methods of the generic
	// wrapper: a body of a 101 response with Read, Write and Close must be
	// an io.ReadWriteCloser (httputil.ReverseProxy), so the ReadWriter
	// family fits it better than the Reader family, which has no Write.
	// When no family has them all, the wrapper with the most methods of
	// the type wins: the family keeps WriteTo and the rune methods of a
	// reader with Seek, Close and ReadAt, and loses ReadAt; the generic
	// wrapper is used when it has more methods.
	var need []string
	if w := m.wrapperFor(iv.T, target); w != nil {
		need = w.methods
	}
	c := m.optFor(iv.T, target, need)
	if c == nil || !optHasAll(c.t, need) && len(need) > c.t.NumMethod() {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(c.make(vmObj{m: m, iv: m.optPayload(iv), origin: m.originAcct()}, optKey(iv.T))), true
}

// optPayload returns the *Iface that the wrappers of the value iv hold: one
// per type and value of a comparable type, so that == of two wrappers of
// equal values is true in native code, as == of the values in Go (hmac.New
// checks that two hashes are not the same value; ecdsa.PublicKey.Equal
// compares the curves). The table holds weak pointers: an entry goes away
// with its last wrapper.
//
// A value that holds a float or a complex keeps its own payload: +0 == -0
// in Go, but the methods of the two values do not give the same results,
// and a NaN is never equal to itself, so its entry would never be found.
func (m *Machine) optPayload(iv *Iface) *Iface {
	if iv.T.T == nil || iv.T.noCmp || !types.Comparable(iv.T.T) {
		return iv
	}
	var k any
	switch optFloats(iv.T) {
	case optFloat:
		return iv
	case optIface:
		// the dynamic types of the interfaces decide: check the key first
		if k = optValKey(iv); k == nil || keyHasFloat(k) {
			return iv
		}
	}
	if w := iv.T.optLast.Load(); w != nil {
		if p := w.Value(); p != nil && (p == iv || m.optSameShallow(p, iv)) {
			return p
		}
	}
	if k == nil {
		if k = optValKey(iv); k == nil {
			return iv
		}
	}
	p := iv
	if w, ok := m.optVals.Load(k); ok && w.(weak.Pointer[Iface]).Value() != nil {
		p = w.(weak.Pointer[Iface]).Value()
	} else if w, loaded := m.optVals.LoadOrStore(k, weak.Make(iv)); loaded && w.(weak.Pointer[Iface]).Value() != nil {
		p = w.(weak.Pointer[Iface]).Value()
	} else {
		if loaded {
			// A dead entry. Two goroutines can both get here and both
			// Store: the last one wins, and the other payload is only
			// not shared, which is not wrong.
			m.optVals.Store(k, weak.Make(iv))
		}
		// The cleanup holds m weakly: a cleanup is a root, and m may reach
		// iv (a global of type any), so iv and m would never be freed.
		wm := weak.Make(m)
		runtime.AddCleanup(iv, func(k any) {
			m := wm.Value()
			if m == nil {
				return
			}
			if w, ok := m.optVals.Load(k); ok && w.(weak.Pointer[Iface]).Value() == nil {
				m.optVals.CompareAndDelete(k, w)
			}
		}, k)
	}
	if p != nil {
		w := weak.Make(p)
		iv.T.optLast.Store(&w)
	} else {
		p = iv // collected between Load and Value
	}
	return p
}

// optSameShallow reports whether p and iv, values of the same type, are
// equal, when this can be known without reading memory that the values
// only point to. p is the last payload of the type (RType.optLast): it may
// belong to another goroutine, which may write the variables it points to
// at the same time (errors.As(err, &target) on many requests writes each
// target). == of pointers does not read them in Go, but == of the VM reads
// the pointed cell (ptrKeyOf: an array or a native struct). So:
//   - a value without pointers or interfaces (optFlat) is compared with
//     ==: an interface holds a copy that nobody writes;
//   - a pointer is the same only when it is the same cell;
//   - any other value is not the same here: optPayload then looks for its
//     key, which reads only iv, the value of this goroutine.
func (m *Machine) optSameShallow(p, iv *Iface) bool {
	if optFlat(iv.T) {
		return m.optEqual(p, iv)
	}
	if _, ok := iv.T.T.Underlying().(*types.Pointer); ok {
		c, ok := iv.V.R.(*Value)
		return ok && c != nil && p.V.R == any(c) && p.V.N == iv.V.N
	}
	return false
}

// optFlat tells whether values of rt hold no pointer and no interface
// (cached in rt): only basic values (not unsafe.Pointer), strings, and
// arrays and structs of them. == of two such values reads only the values.
func optFlat(rt *RType) bool {
	if f := rt.optFlat.Load(); f != 0 {
		return f == 2
	}
	var flat func(t types.Type, seen map[types.Type]bool) bool
	flat = func(t types.Type, seen map[types.Type]bool) bool {
		if seen[t] {
			return true
		}
		seen[t] = true
		switch u := t.Underlying().(type) {
		case *types.Basic:
			return u.Kind() != types.UnsafePointer
		case *types.Array:
			return flat(u.Elem(), seen)
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if !flat(u.Field(i).Type(), seen) {
					return false
				}
			}
			return true
		}
		return false
	}
	f := int32(1)
	if flat(rt.T, map[types.Type]bool{}) {
		f = 2
	}
	rt.optFlat.Store(f)
	return f == 2
}

// The values of RType.optFlt (0: not computed yet).
const (
	optNoFloat = 1 + iota // no float or complex in the type
	optFloat              // a float or complex, outside of interfaces
	optIface              // no float outside of interfaces, but an interface
)

// optFloats tells whether values of rt hold a float or a complex, for
// optPayload (cached in rt). Pointers are not followed: == compares them
// by address.
func optFloats(rt *RType) int32 {
	if f := rt.optFlt.Load(); f != 0 {
		return f
	}
	f := int32(optNoFloat)
	var walk func(t types.Type, seen map[types.Type]bool)
	walk = func(t types.Type, seen map[types.Type]bool) {
		if seen[t] {
			return
		}
		seen[t] = true
		switch u := t.Underlying().(type) {
		case *types.Basic:
			if u.Info()&(types.IsFloat|types.IsComplex) != 0 {
				f = optFloat
			}
		case *types.Interface:
			if f == optNoFloat {
				f = optIface
			}
		case *types.Array:
			walk(u.Elem(), seen)
		case *types.Struct:
			for i := 0; i < u.NumFields() && f != optFloat; i++ {
				if u.Field(i).Name() != "_" { // blank fields are not compared
					walk(u.Field(i).Type(), seen)
				}
			}
		}
	}
	walk(rt.T, map[types.Type]bool{})
	rt.optFlt.Store(f)
	return f
}

// keyHasFloat reports whether the key k (of keyOf) holds a float or a
// complex. A native value is judged by its type, and an interface in it
// counts as a float, to stay on the safe side.
func keyHasFloat(k any) bool {
	switch k := k.(type) {
	case nil:
		return false
	case tupleKey:
		return keyHasFloat(k.head) || keyHasFloat(k.tail)
	case ifKey:
		return keyHasFloat(k.K)
	case nativeKey:
		return keyHasFloat(k.V)
	}
	return reflectHasFloat(reflect.TypeOf(k))
}

// reflectHasFloat is keyHasFloat for a native type t.
func reflectHasFloat(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.Interface:
		return true
	case reflect.Array:
		return reflectHasFloat(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).Name != "_" && reflectHasFloat(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}

// optEqual reports whether a and b, values of the same type, are equal
// (false when == would panic: an interface field of an uncomparable type).
func (m *Machine) optEqual(a, b *Iface) (eq bool) {
	defer func() {
		if recover() != nil {
			eq = false
		}
	}()
	return m.equal(a.V, b.V, a.T.T)
}

// optValKey is the key of iv in optVals, or nil when the value holds an
// interface whose dynamic type is not comparable (keyOf panics, as == does).
func optValKey(iv *Iface) (k any) {
	defer func() {
		if recover() != nil {
			k = nil
		}
	}()
	return keyOf(Value{R: iv}, emptyIface)
}

// optHasAll reports whether t has every method of names.
func optHasAll(t reflect.Type, names []string) bool {
	for _, n := range names {
		if _, ok := t.MethodByName(n); !ok {
			return false
		}
	}
	return true
}

// optError is the wrapper of the Error family for a plugin error given to
// native code as any (fmt.Errorf("%w"), a context value): the hand-written
// errorW and errStringerW without optional methods.
func (m *Machine) optError(o vmObj) (reflect.Value, bool) {
	f := optFamilyByName()["Error"]
	mask, ok := m.optMask(o.iv.T, f)
	if !ok || mask == 0 || f.combos[mask].t == nil {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(f.combos[mask].make(o, optKey(o.iv.T))), true
}

// optAny is optWrapper for a plugin value given to native code as any: the
// wrapper of the family with the most methods whose base methods the type
// has.
func (m *Machine) optAny(iv *Iface) (reflect.Value, bool) {
	if len(iv.T.Methods) == 0 || m.isNativeNamed(iv.T.T) {
		return reflect.Value{}, false
	}
	if p, ok := iv.T.T.(*types.Pointer); ok && m.isNativeNamed(p.Elem()) {
		return reflect.Value{}, false
	}
	c := m.optFor(iv.T, anyType, nil)
	if c == nil {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(c.make(vmObj{m: m, iv: m.optPayload(iv), origin: m.originAcct()}, optKey(iv.T))), true
}
