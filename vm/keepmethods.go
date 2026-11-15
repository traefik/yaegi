package vm

// Keeping the methods of the plugin types for native fmt and encoding/json.
//
// Native code cannot see the methods of interpreted types (the reflect
// wall, aotroute.go). A value given to native code in an interface is
// boxed in a wrapper that has some methods (box: Error, String, Format,
// MarshalJSON), but a value inside it (a struct field, a slice element, a
// map value, the value of a []any element) is a native copy of its
// underlying value: native fmt printed {1 myerr 7} for a struct with a
// field of a type with a String method (Go: {WARN myerr 7}), and native
// json wrote 3 for a value with a MarshalText method (Go: "c3"), with no
// error. The routes of aotroute.go send these calls to the AOT copies of
// fmt and encoding/json, which see the methods, but the copies cannot
// always run: a host built with a newer Go patch release than the copies
// (StaleAOTFallback, aotversion.go), a host without aot/all, or a host
// that forces the native mode of the package (Options.Modes).
//
// Then a native printer (printFuncs) and the encoders of encoding/json
// (lostSites) get a "keep" conversion: a value whose type has, inside it,
// a method that the native function calls, is converted to a native value
// of another shape, where each value of a type with such a method is a
// keptW, a wrapper whose Format and MarshalJSON call the method in the VM:
//
//   - a struct becomes a native struct with the same field names and tags,
//     whose fields of such types are keptW (json: *keptW for a field with
//     omitempty or omitzero, nil when the field is omitted);
//   - a slice, an array, a map, a pointer: the same, for their elements;
//   - a json map key with a MarshalText method: a keptKeyW.
//
// The cases that the conversion cannot keep (a recursive type, an embedded
// field with such a method, an embedded struct of an unexported type with
// such a method inside, which reflect cannot build, a fmt map key or byte
// slice of such a type) are never silent: encoding/json returns a
// *LostMethodError (and the load fails when the type is known at load
// time, lostProblem), and fmt prints "%!v(yaegi v2: ...)" in place of the
// value.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/types"
	"io"
	"reflect"
	"strings"
)

// keepMode is the native function that a keep conversion is for.
type keepMode uint8

const (
	keepNone keepMode = iota
	keepFmt           // a native printer: Format, Error, String, GoString
	keepJSON          // an encoder of encoding/json: MarshalJSON, MarshalText
)

// keepCtx is the place of a value in the converted value.
type keepCtx uint8

const (
	ctxTop       keepCtx = 1 << iota // the value of the interface itself (fmt: depth 0)
	ctxAddr                          // json: addressable, so the methods of *T count
	ctxKey                           // json: a map key
	ctxOmitEmpty                     // json: a field with omitempty
	ctxOmitZero                      // json: a field with omitzero
)

type keepKind uint8

const (
	kpWrap keepKind = iota + 1 // a keptW (or keptKeyW, *keptW)
	kpPtr
	kpSlice
	kpArray
	kpMap
	kpStruct
	kpIface // a non-empty interface: its dynamic value is checked
	kpDyn   // a type inside itself: an `any` that holds its own conversion
)

// keepPlan is the conversion of a type that holds a method to keep. A nil
// plan means the ordinary conversion (toNativeW): nothing to keep.
type keepPlan struct {
	kind   keepKind
	rt     reflect.Type // the native type
	addr   bool         // kpWrap: the methods of *T, on a copy
	key    bool         // kpWrap: a json map key (keptKeyW)
	omit   keepCtx      // kpWrap: ctxOmitEmpty and ctxOmitZero of a json field (rt is *keptW)
	elem   *keepPlan    // kpPtr, kpSlice, kpArray, kpMap
	kplan  *keepPlan    // kpMap: the keys
	fields []*keepPlan  // kpStruct
	err    string       // not nil: the value cannot be kept, why
	ctx    keepCtx      // kpDyn: the place of the value
}

type keepTKey struct {
	t    types.Type
	mode keepMode
	ctx  keepCtx
}

var (
	keptWType   = reflect.TypeFor[keptW]()
	keptKeyType = reflect.TypeFor[keptKeyW]()
)

// keepPlanOf returns the plan of a value of type t at place ctx (cached).
func (m *Machine) keepPlanOf(t types.Type, mode keepMode, ctx keepCtx) *keepPlan {
	k := keepTKey{t, mode, ctx}
	if p, ok := m.keepPlans.Load(k); ok {
		return p.(*keepPlan)
	}
	b := keepBuilder{m: m, mode: mode, open: map[keepTKey]bool{}}
	p := b.build(t, ctx)
	m.keepPlans.Store(k, p)
	return p
}

type keepBuilder struct {
	m    *Machine
	mode keepMode
	open map[keepTKey]bool
	cyc  bool // a type was met inside itself
	dyn  bool // a type met inside itself is a kpDyn (else nil)
	lost string
}

func (b *keepBuilder) build(t types.Type, ctx keepCtx) (p *keepPlan) {
	defer func() {
		if r := recover(); r != nil {
			ce, ok := r.(*CompileError)
			if !ok {
				panic(r)
			}
			p = &keepPlan{err: strings.TrimPrefix(ce.Msg, "unsupported: ")}
		}
	}()
	p = b.plan(t, ctx)
	if p != nil && p.err == "" && b.cyc {
		// A recursive type with a method to keep: reflect cannot build a
		// recursive type, so the value of the type inside itself is in
		// an `any`, which holds its own conversion (kpDyn). json and fmt
		// write an interface as the value it holds.
		b.cyc, b.dyn = false, true
		p = b.plan(t, ctx)
	}
	return p
}

// names returns the methods that the native function calls for a value
// at place ctx.
func (b *keepBuilder) names(ctx keepCtx) []string {
	switch {
	case b.mode == keepFmt:
		return []string{"Format", "Error", "String", "GoString"}
	case ctx&ctxKey != 0:
		return []string{"MarshalText"}
	}
	return []string{"MarshalJSON", "MarshalText"}
}

// method returns the first method of names in the run-time method set of t.
func keepMethod(t types.Type, names []string) string {
	ms := types.NewMethodSet(t) // not prog.MethodSets: this runs on any goroutine
	for _, n := range names {
		if sel := ms.Lookup(nil, n); sel != nil && runtimeMethod(sel) {
			return n
		}
	}
	return ""
}

// wrapOf returns the kpWrap plan of a value of the named type (or pointer
// to a named type) t that has a method to keep, or nil.
func (b *keepBuilder) wrapOf(t types.Type, ctx keepCtx) *keepPlan {
	names := b.names(ctx)
	if b.mode == keepFmt && ctx&ctxTop != 0 {
		// box gives the value itself a wrapper whose Format calls Error,
		// String or GoString, but not a Format method of its own
		names = names[:1]
	}
	p := &keepPlan{kind: kpWrap, rt: keptWType, key: ctx&ctxKey != 0, omit: ctx & (ctxOmitEmpty | ctxOmitZero)}
	n := keepMethod(t, names)
	if n == "" && b.mode == keepJSON && ctx&ctxAddr != 0 {
		if _, ok := t.(*types.Pointer); !ok {
			if n = keepMethod(types.NewPointer(t), names); n != "" {
				p.addr = true
			}
		}
	}
	if n == "" {
		return nil
	}
	b.lost = typeString(t) + "." + n
	switch {
	case p.key:
		p.rt = keptKeyType
	case p.omit != 0:
		p.rt = reflect.PointerTo(keptWType)
	}
	return p
}

func (b *keepBuilder) fail(format string, args ...any) *keepPlan {
	return &keepPlan{err: fmt.Sprintf(format, args...)}
}

// rtOf is the native type of a value of type t with plan p.
func (b *keepBuilder) rtOf(p *keepPlan, t types.Type) reflect.Type {
	if p != nil {
		return p.rt
	}
	return b.m.reflectType(t)
}

func (b *keepBuilder) plan(t types.Type, ctx keepCtx) *keepPlan {
	m := b.m
	t = types.Unalias(t)
	inner := ctx &^ (ctxTop | ctxKey | ctxOmitEmpty | ctxOmitZero)
	switch u := t.(type) {
	case *types.Named:
		if m.isNativeNamed(u) || m.twinNamed(u) != nil {
			return nil // native: native code sees its methods
		}
		if it, ok := u.Underlying().(*types.Interface); ok {
			return b.plan(it, ctx)
		}
		if w := b.wrapOf(u, ctx); w != nil {
			return w
		}
		k := keepTKey{u, b.mode, 0}
		if b.open[k] {
			if b.dyn {
				return &keepPlan{kind: kpDyn, rt: anyType, ctx: ctx}
			}
			b.cyc = true
			return nil
		}
		b.open[k] = true
		defer delete(b.open, k)
		return b.plan(u.Underlying(), ctx)
	case *types.Pointer:
		if n, ok := types.Unalias(u.Elem()).(*types.Named); ok && (m.isNativeNamed(n) || m.twinNamed(n) != nil) {
			return nil
		}
		if _, ok := types.Unalias(u.Elem()).(*types.Named); ok {
			if w := b.wrapOf(u, ctx); w != nil {
				return w
			}
		}
		if b.mode == keepFmt && ctx&ctxTop == 0 {
			return nil // fmt prints the address of a pointer inside a value
		}
		ep := b.plan(u.Elem(), inner|ctxAddr)
		if ep == nil || ep.err != "" {
			return ep
		}
		return &keepPlan{kind: kpPtr, rt: reflect.PointerTo(ep.rt), elem: ep}
	case *types.Slice:
		ep := b.plan(u.Elem(), inner|ctxAddr)
		if ep == nil || ep.err != "" {
			return ep
		}
		if b.mode == keepFmt && ep.kind == kpWrap && isByteKind(u.Elem()) {
			return b.fail("the method %s is in the elements of a byte slice", b.lost)
		}
		return &keepPlan{kind: kpSlice, rt: reflect.SliceOf(ep.rt), elem: ep}
	case *types.Array:
		ep := b.plan(u.Elem(), inner)
		if ep == nil || ep.err != "" {
			return ep
		}
		if b.mode == keepFmt && ep.kind == kpWrap && isByteKind(u.Elem()) {
			return b.fail("the method %s is in the elements of a byte array", b.lost)
		}
		return &keepPlan{kind: kpArray, rt: reflect.ArrayOf(int(u.Len()), ep.rt), elem: ep}
	case *types.Map:
		// the keys and the values of a map are never addressable: json
		// does not call the methods of *T for them
		inner &^= ctxAddr
		var kp *keepPlan
		if bt, ok := u.Key().Underlying().(*types.Basic); b.mode == keepFmt || !ok || bt.Info()&types.IsString == 0 {
			// json writes a key of string kind as that string
			kctx := inner
			if b.mode == keepJSON {
				kctx |= ctxKey
			}
			kp = b.plan(u.Key(), kctx)
		}
		if kp != nil && kp.err != "" {
			return kp
		}
		if kp != nil && b.mode == keepFmt {
			// fmt sorts the keys by their values: a fmtKeyW holds the
			// value first (basic kinds only)
			kt, ok := fmtKeyTypes[b.m.reflectType(u.Key()).Kind()]
			if kp.kind != kpWrap || !ok {
				return b.fail("the method %s is in the keys of a map", b.lost)
			}
			kp = &keepPlan{kind: kpWrap, rt: kt, key: true}
		}
		ep := b.plan(u.Elem(), inner)
		if ep != nil && ep.err != "" {
			return ep
		}
		if kp == nil && ep == nil {
			return nil
		}
		return &keepPlan{kind: kpMap, rt: reflect.MapOf(b.rtOf(kp, u.Key()), b.rtOf(ep, u.Elem())), kplan: kp, elem: ep}
	case *types.Struct:
		return b.structPlan(u, ctx)
	case *types.Interface:
		if u.Empty() || ctx&ctxTop != 0 {
			return nil // toNativeW sends the dynamic value to keepIface
		}
		return &keepPlan{kind: kpIface, rt: anyType}
	}
	return nil
}

func isByteKind(t types.Type) bool {
	bt, ok := t.Underlying().(*types.Basic)
	return ok && (bt.Kind() == types.Uint8)
}

// structPlan is plan for a struct: the fields that json and fmt read
// (walkFields of lostmethods.go).
func (b *keepBuilder) structPlan(u *types.Struct, ctx keepCtx) *keepPlan {
	fps := make([]*keepPlan, u.NumFields())
	found := false
	for i := range fps {
		f := u.Field(i)
		if !f.Exported() && (b.mode == keepFmt || !f.Embedded()) {
			continue // fmt and json do not call the methods of an unexported field
		}
		fctx := ctx & ctxAddr
		flat := false
		if b.mode == keepJSON {
			tag := reflect.StructTag(u.Tag(i)).Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			flat = f.Embedded() && name == ""
			if hasOpt(opts, "omitempty") {
				fctx |= ctxOmitEmpty
			}
			if hasOpt(opts, "omitzero") {
				fctx |= ctxOmitZero
			}
		}
		var fp *keepPlan
		if flat {
			fp = b.flatPlan(f.Type(), ctx&ctxAddr)
		} else {
			fp = b.plan(f.Type(), fctx)
		}
		if fp == nil {
			continue
		}
		if fp.err != "" {
			return fp
		}
		fps[i], found = fp, true
	}
	if !found {
		return nil
	}
	fs := make([]reflect.StructField, u.NumFields())
	for i := range fs {
		f := u.Field(i)
		fs[i] = reflect.StructField{Name: f.Name(), Tag: reflect.StructTag(u.Tag(i)), Anonymous: f.Embedded(), Type: b.rtOf(fps[i], f.Type())}
		if !f.Exported() {
			fs[i].PkgPath = "main"
		}
		if fs[i].Anonymous && fs[i].Type.Kind() != reflect.Interface && fs[i].Type.NumMethod() > 0 {
			fs[i].Anonymous = false // see reflectType
		}
	}
	return &keepPlan{kind: kpStruct, rt: structOf(fs), fields: fps}
}

// flatPlan is the plan of an embedded field without a json name: json
// flattens an embedded struct (or pointer to a struct) into the outer one,
// and does not call its methods (the methods promoted to the outer type
// were counted there). A field of another type is a field as the others.
func (b *keepBuilder) flatPlan(t types.Type, ctx keepCtx) *keepPlan {
	et, ptr := types.Unalias(t), false
	if pt, ok := et.Underlying().(*types.Pointer); ok {
		et, ptr = types.Unalias(pt.Elem()), true
	}
	st, ok := et.Underlying().(*types.Struct)
	if !ok {
		return b.plan(t, ctx)
	}
	if n, ok := et.(*types.Named); ok {
		if b.m.isNativeNamed(n) || b.m.twinNamed(n) != nil {
			return nil
		}
		k := keepTKey{n, b.mode, 0}
		if b.open[k] {
			return b.fail("the struct %s is embedded in itself", typeString(n))
		}
		b.open[k] = true
		defer delete(b.open, k)
	}
	if ptr {
		ctx |= ctxAddr
	}
	sp := b.structPlan(st, ctx)
	if sp == nil || sp.err != "" || !ptr {
		return sp
	}
	return &keepPlan{kind: kpPtr, rt: reflect.PointerTo(sp.rt), elem: sp}
}

// keepIface converts iv, the dynamic value of an interface given to a
// native function of mode c.g.keep (ctx: ctxTop for an argument, 0 for an
// interface inside a value), when its type holds a method to keep (ok
// false: the ordinary conversion).
func (m *Machine) keepIface(iv *Iface, ctx keepCtx, c *natConv) (rv reflect.Value, ok bool) {
	if iv.T.Native != nil || iv.T.T == nil {
		return reflect.Value{}, false
	}
	mode := c.g.keep
	p := m.keepPlanOf(iv.T.T, mode, ctx)
	if p == nil {
		return reflect.Value{}, false
	}
	if mode == keepFmt {
		defer func() {
			if r := recover(); r != nil {
				e, isLost := r.(*LostMethodError)
				if !isLost {
					panic(r)
				}
				rv, ok = reflect.ValueOf(keptErrW{e.Msg}), true
			}
		}()
	}
	if p.err != "" {
		m.keepFail(mode, p.err, iv.T.T)
	}
	return m.keepValue(iv.V, iv.T.T, p, c), true
}

// keepFail panics with the *LostMethodError of a value of type t that a
// keep conversion of mode cannot convert (why).
func (m *Machine) keepFail(mode keepMode, why string, t types.Type) {
	fn := "encoding/json"
	if mode == keepFmt {
		fn = "fmt"
	}
	panic(&LostMethodError{Msg: fmt.Sprintf("in default mode without the AOT copy, native %s cannot keep the methods of a value of type %s (%s); run the plugin in AOT mode (Options.AOT) with AOT copies that can run in this host", fn, typeString(t), why)})
}

// keepValue converts v, of type t, with the plan p.
func (m *Machine) keepValue(v Value, t types.Type, p *keepPlan, c *natConv) reflect.Value {
	if p == nil {
		return m.toNativeW(v, t, c.nested())
	}
	if p.err != "" {
		m.keepFail(c.g.keep, p.err, t)
	}
	c.g.use(1)
	switch p.kind {
	case kpWrap:
		return m.keepWrap(v, t, p, c)
	case kpPtr:
		if v.R == nil {
			return reflect.Zero(p.rt)
		}
		cell, ok := v.R.(*Value)
		if !ok {
			m.keepFail(c.g.keep, fmt.Sprintf("a pointer held as %T", v.R), t)
		}
		if out, ok := c.g.kept[cell]; ok {
			return out // a cycle: json says so, as in Go
		}
		out := reflect.New(p.rt.Elem())
		if c.g.kept == nil {
			c.g.kept = map[*Value]reflect.Value{}
		}
		c.g.kept[cell] = out
		et := t.Underlying().(*types.Pointer).Elem()
		out.Elem().Set(m.keepValue(loadCell(cell, et), et, p.elem, c))
		return out
	case kpSlice, kpArray:
		et := elemOf(t)
		es, ok := v.R.([]Value)
		if !ok && v.R != nil {
			// another representation ([]byte, a native slice view): the
			// elements of its ordinary native copy
			nv := m.toNativeW(v, t, c.nested())
			es = make([]Value, nv.Len())
			for i := range es {
				es[i] = m.fromNative(nv.Index(i), et)
			}
		}
		var out reflect.Value
		if p.kind == kpSlice {
			if v.R == nil {
				return reflect.Zero(p.rt)
			}
			out = reflect.MakeSlice(p.rt, len(es), len(es))
		} else {
			out = reflect.New(p.rt).Elem()
		}
		for i := range es {
			out.Index(i).Set(m.keepValue(loadCell(&es[i], et), et, p.elem, c))
		}
		return out
	case kpMap:
		mp, _ := v.R.(*MapV)
		if mp == nil {
			if v.R != nil {
				m.keepFail(c.g.keep, fmt.Sprintf("a map held as %T", v.R), t)
			}
			return reflect.Zero(p.rt)
		}
		u := t.Underlying().(*types.Map)
		out := reflect.MakeMapWithSize(p.rt, len(mp.M))
		mp.each(func(k, e Value) {
			out.SetMapIndex(m.keepValue(k, u.Key(), p.kplan, c), m.keepValue(e, u.Elem(), p.elem, c))
		})
		return out
	case kpStruct:
		u := t.Underlying().(*types.Struct)
		fs, ok := v.R.([]Value)
		if !ok {
			m.keepFail(c.g.keep, fmt.Sprintf("a struct held as %T", v.R), t)
		}
		out := reflect.New(p.rt).Elem()
		for i := range fs {
			f := out.Field(i)
			if !f.CanSet() {
				f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
			}
			ft := u.Field(i).Type()
			fv := loadCell(&fs[i], ft)
			if p.fields[i] == nil {
				setNative(f, m.toNativeW(fv, ft, c.nested()))
				continue
			}
			f.Set(m.keepValue(fv, ft, p.fields[i], c))
		}
		return out
	case kpDyn:
		if p.ctx&(ctxOmitEmpty|ctxOmitZero) != 0 && m.keepOmitted(v, t, p.ctx) {
			return reflect.Zero(anyType) // a nil interface: json omits it
		}
		out := reflect.New(anyType).Elem()
		out.Set(m.keepValue(v, t, m.keepPlanOf(t, c.g.keep, p.ctx&^(ctxOmitEmpty|ctxOmitZero)), c))
		return out
	case kpIface:
		iv, _ := v.R.(*Iface)
		if iv == nil {
			return reflect.Zero(anyType)
		}
		if rv, ok := m.keepIface(iv, 0, c); ok {
			out := reflect.New(anyType).Elem()
			out.Set(rv)
			return out
		}
		return m.toNativeW(v, t, c.nested())
	}
	panic("vm: bad keep plan")
}

func elemOf(t types.Type) types.Type {
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return u.Elem()
	case *types.Array:
		return u.Elem()
	}
	panic("vm: elemOf " + t.String())
}

// keepWrap is the wrapper of v, of type t, whose methods native code calls.
func (m *Machine) keepWrap(v Value, t types.Type, p *keepPlan, c *natConv) reflect.Value {
	if p.omit != 0 && m.keepOmitted(v, t, p.omit) {
		return reflect.Zero(p.rt)
	}
	wt := t
	if p.addr {
		cell := new(Value)
		*cell = v
		v, wt = Value{R: cell}, types.NewPointer(t)
	}
	o := vmObj{m: m, iv: &Iface{T: m.rtypeOf(wt), V: v}, origin: m.originAcct()}
	switch {
	case p.key && c.g.keep == keepFmt:
		k := reflect.New(p.rt).Elem()
		f := k.Field(0)
		f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
		f.Set(m.toNative(v, t).Convert(f.Type()))
		k.Field(1).Set(reflect.ValueOf(keptW{o}))
		return k
	case p.key:
		return reflect.ValueOf(keptKeyW{o})
	case p.omit != 0:
		return reflect.ValueOf(&keptW{o})
	}
	return reflect.ValueOf(keptW{o})
}

// keepOmitted reports whether json omits the field value v of type t with
// the options omit (encoding/json: isEmptyValue, and the isZero of a
// field with omitzero).
func (m *Machine) keepOmitted(v Value, t types.Type, omit keepCtx) bool {
	nv := m.toNative(v, t)
	if omit&ctxOmitEmpty != 0 && jsonEmpty(nv) {
		return true
	}
	if omit&ctxOmitZero == 0 {
		return false
	}
	isZero := []string{"IsZero"}
	_, isPtr := t.Underlying().(*types.Pointer)
	switch {
	case keepMethod(t, isZero) != "":
		if isPtr && v.R == nil {
			return true // encoding/json does not call IsZero on a nil pointer
		}
	case !isPtr && keepMethod(types.NewPointer(t), isZero) != "":
		cell := new(Value)
		*cell = v
		v, t = Value{R: cell}, types.NewPointer(t)
	default:
		return nv.IsZero()
	}
	o := vmObj{m: m, iv: &Iface{T: m.rtypeOf(t), V: v}, origin: m.originAcct()}
	res := o.call("IsZero")
	return len(res) == 1 && res[0].Kind() == reflect.Bool && res[0].Bool()
}

// jsonEmpty is isEmptyValue of encoding/json.
func jsonEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

// keptW is a value of a plugin type inside a value converted for native fmt
// or encoding/json (see keepIface): its Format and MarshalJSON (vmObj) call
// the methods of the value in the VM.
type keptW struct{ vmObj }

// Format calls the Format method of the value, or formats it as vmObj
// does (Error, String, GoString). As fmt, it prints <nil> for a nil
// pointer whose method panics.
func (w keptW) Format(f fmt.State, verb rune) {
	if w.iv.V.R == nil {
		if _, ok := w.iv.T.T.Underlying().(*types.Pointer); ok {
			defer func() {
				if r := recover(); r != nil {
					io.WriteString(f, "<nil>")
				}
			}()
		}
	}
	if fn, ok := w.iv.T.Methods["Format"]; ok && fn.ssa.Signature.Params().Len() == 2 && fn.ssa.Signature.Results().Len() == 0 {
		w.call("Format", reflect.ValueOf(&f).Elem(), reflect.ValueOf(verb))
		return
	}
	w.vmObj.Format(f, verb)
}

// MarshalJSON is the one of vmObj, but writes null for a nil pointer, as
// encoding/json v1 does without calling the method (json v2 with the
// legacy options calls it: vmObj does not check).
func (w keptW) MarshalJSON() ([]byte, error) {
	if _, isPtr := w.iv.T.T.Underlying().(*types.Pointer); isPtr && w.iv.V.R == nil {
		return []byte("null"), nil
	}
	return w.vmObj.MarshalJSON()
}

// keptKeyW is a json map key of a plugin type with a MarshalText method.
type keptKeyW struct{ vmObj }

func (w keptKeyW) MarshalText() ([]byte, error) {
	res := w.call("MarshalText")
	err, _ := res[1].Interface().(error)
	return res[0].Bytes(), err
}

// fmtKeyW is a fmt map key of a plugin type with a method to keep: fmt
// sorts the keys by their fields, so by the value k first, as it sorts the
// values of the plugin type.
type fmtKeyW[K any] struct {
	k K
	W keptW
}

func (w fmtKeyW[K]) Format(f fmt.State, verb rune) { w.W.Format(f, verb) }

// fmtKeyTypes are the fmtKeyW types by the kind of the key.
var fmtKeyTypes = map[reflect.Kind]reflect.Type{
	reflect.Bool:    reflect.TypeFor[fmtKeyW[bool]](),
	reflect.Int:     reflect.TypeFor[fmtKeyW[int]](),
	reflect.Int8:    reflect.TypeFor[fmtKeyW[int8]](),
	reflect.Int16:   reflect.TypeFor[fmtKeyW[int16]](),
	reflect.Int32:   reflect.TypeFor[fmtKeyW[int32]](),
	reflect.Int64:   reflect.TypeFor[fmtKeyW[int64]](),
	reflect.Uint:    reflect.TypeFor[fmtKeyW[uint]](),
	reflect.Uint8:   reflect.TypeFor[fmtKeyW[uint8]](),
	reflect.Uint16:  reflect.TypeFor[fmtKeyW[uint16]](),
	reflect.Uint32:  reflect.TypeFor[fmtKeyW[uint32]](),
	reflect.Uint64:  reflect.TypeFor[fmtKeyW[uint64]](),
	reflect.Uintptr: reflect.TypeFor[fmtKeyW[uintptr]](),
	reflect.Float32: reflect.TypeFor[fmtKeyW[float32]](),
	reflect.Float64: reflect.TypeFor[fmtKeyW[float64]](),
	reflect.String:  reflect.TypeFor[fmtKeyW[string]](),
}

// keptErrW is printed by fmt in place of a value that the keep conversion
// cannot convert.
type keptErrW struct{ msg string }

func (w keptErrW) Format(f fmt.State, verb rune) {
	fmt.Fprintf(f, "%%!%c(%s)", verb, "yaegi v2: "+w.msg)
}

// jsonText is the JSON string of the text of a MarshalText method, as
// encoding/json writes it (the encoder escapes HTML after, if it does).
func jsonText(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(string(b)); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// keepIfaceOf is keepIface for the dynamic value of an interface of type u
// in a conversion: only for the `any` arguments of a call with c.g.keep
// (an io.Writer given to fmt.Fprintf keeps its ordinary conversion).
func (m *Machine) keepIfaceOf(u *types.Interface, iv *Iface, c *natConv) (reflect.Value, bool) {
	if c.g.keep == keepNone || !u.Empty() {
		return reflect.Value{}, false
	}
	return m.keepIface(iv, ctxTop, c)
}
