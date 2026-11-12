package vm

// Go-level twin of package reflect over VM values: types.
//
// Packages compiled ahead of time against the VM value model (cmd/aotgen,
// aot/...) import github.com/traefik/yaegi/v2/vreflect instead of reflect; that
// facade aliases the declarations of twin_*.go. Their reflect calls then run
// as native Go code on both kinds of data they meet:
//
//   - VM data (values of the interpreted program): a *vtype with a go/types
//     type (t) per machine, values are cells of the VM representation (*Value);
//   - native data (the AOT package's own values, native stdlib values): a
//     *vtype with a reflect.Type (nat), shared by all machines, values are
//     reflect.Value.
//
// Type identity: every RT handed to AOT code is canonical (vtype.cn): a VM
// type that has a native counterpart (basic types, native named types,
// composites of those: []string, map[string]any, *time.Time...) is reported as
// the native descriptor, so that comparisons such as
// `v.Type() == reflect.TypeFor[json.Number]()` hold; only types involving
// interpreted named types, unnamed structs or non-empty unnamed interfaces
// stay VM descriptors. The storage type of a value (RV.t) may differ from its
// canonical type: a VM cell of type []string is still a VM cell.

import (
	"go/types"
	"path"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// RT is a type descriptor of the twin (reflect.Type).
type RT = *vtype

var natTypes sync.Map // reflect.Type -> *vtype

// natT returns the canonical descriptor of a native type.
func natT(rt reflect.Type) *vtype {
	if rt == nil {
		return nil
	}
	if v, ok := natTypes.Load(rt); ok {
		return v.(*vtype)
	}
	v, _ := natTypes.LoadOrStore(rt, &vtype{nat: rt, kind: rt.Kind()})
	return v.(*vtype)
}

// isNat reports whether vt describes a native type.
func (vt *vtype) isNat() bool { return vt.t == nil }

// cn returns the canonical descriptor of vt (see the file comment).
func (vt *vtype) cn() *vtype {
	if vt == nil || vt.t == nil {
		return vt
	}
	if c := vt.canon.Load(); c != nil {
		return c
	}
	c := vt
	if rt := vt.vr.m.counterpart(vt.t); rt != nil {
		c = natT(rt)
	}
	vt.canon.Store(c)
	return c
}

// reflectBasicNamed are the named types of package reflect whose
// underlying type is basic, by name. With the VM reflect (progVR) the
// types of reflect are not native (isNativeNamed), but these ones have the
// layout of their native type in the VM too: they are their own native
// counterpart. Without it, a func of the program that takes a reflect.Kind
// had no counterpart, and the twin found it not convertible to the native
// func type of an AOT copy: mapstructure refused the decode hook
// func(reflect.Kind, reflect.Kind, any) (any, error) with "invalid decode
// hook signature".
var reflectBasicNamed = map[string]reflect.Type{
	"Kind":      reflect.TypeFor[reflect.Kind](),
	"ChanDir":   reflect.TypeFor[reflect.ChanDir](),
	"SelectDir": reflect.TypeFor[reflect.SelectDir](),
	"StructTag": reflect.TypeFor[reflect.StructTag](),
}

// counterpart returns the native type of t when it exists (nil otherwise).
func (m *Machine) counterpart(t types.Type) (rt reflect.Type) {
	defer func() {
		if recover() != nil {
			rt = nil
		}
	}()
	return m.cpart(t)
}

func (m *Machine) cpart(t types.Type) reflect.Type {
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		if t.Info()&types.IsUntyped != 0 {
			return nil
		}
		return basicRT[t.Kind()]
	case *types.Named:
		if t.Obj().Pkg() == nil && t.Obj().Name() == "error" {
			return errorType
		}
		if r := m.twinNamed(t); r != nil {
			return r
		}
		if r := reflectBasicNamed[t.Obj().Name()]; r != nil && isReflectPkg(t.Obj().Pkg()) {
			return r
		}
		if m.isNativeNamed(t) {
			return m.reflectType(t)
		}
		return nil
	case *types.Pointer:
		if e := m.cpart(t.Elem()); e != nil {
			return reflect.PointerTo(e)
		}
	case *types.Slice:
		if e := m.cpart(t.Elem()); e != nil {
			return reflect.SliceOf(e)
		}
	case *types.Array:
		if e := m.cpart(t.Elem()); e != nil {
			return reflect.ArrayOf(int(t.Len()), e)
		}
	case *types.Map:
		k, e := m.cpart(t.Key()), m.cpart(t.Elem())
		if k != nil && e != nil {
			return reflect.MapOf(k, e)
		}
	case *types.Chan:
		if e := m.cpart(t.Elem()); e != nil {
			return reflect.ChanOf(chanDir(t.Dir()), e)
		}
	case *types.Interface:
		if t.Empty() {
			return anyType
		}
	case *types.Signature:
		in := make([]reflect.Type, t.Params().Len())
		for i := range in {
			if in[i] = m.cpart(t.Params().At(i).Type()); in[i] == nil {
				return nil
			}
		}
		out := make([]reflect.Type, t.Results().Len())
		for i := range out {
			if out[i] = m.cpart(t.Results().At(i).Type()); out[i] == nil {
				return nil
			}
		}
		return reflect.FuncOf(in, out, t.Variadic())
	}
	return nil
}

func chanDir(d types.ChanDir) reflect.ChanDir {
	switch d {
	case types.SendOnly:
		return reflect.SendDir
	case types.RecvOnly:
		return reflect.RecvDir
	}
	return reflect.BothDir
}

// goTypeOf maps a native type to the go/types type of the interpreted
// program (nil when it has none: unnamed structs, non-empty interfaces,
// named types of packages absent from the program).
func (m *Machine) goTypeOf(rt reflect.Type) types.Type {
	m.mu.Lock()
	if m.goTypes == nil {
		m.goTypes = map[reflect.Type]types.Type{}
	}
	t, ok := m.goTypes[rt]
	m.mu.Unlock()
	if ok {
		return t
	}
	t = m.goTypeOf1(rt)
	m.mu.Lock()
	m.goTypes[rt] = t
	m.mu.Unlock()
	return t
}

var basicByKind = map[reflect.Kind]types.Type{
	reflect.Bool: types.Typ[types.Bool], reflect.Int: types.Typ[types.Int], reflect.Int8: types.Typ[types.Int8],
	reflect.Int16: types.Typ[types.Int16], reflect.Int32: types.Typ[types.Int32], reflect.Int64: types.Typ[types.Int64],
	reflect.Uint: types.Typ[types.Uint], reflect.Uint8: types.Typ[types.Uint8], reflect.Uint16: types.Typ[types.Uint16],
	reflect.Uint32: types.Typ[types.Uint32], reflect.Uint64: types.Typ[types.Uint64], reflect.Uintptr: types.Typ[types.Uintptr],
	reflect.Float32: types.Typ[types.Float32], reflect.Float64: types.Typ[types.Float64], reflect.String: types.Typ[types.String],
	reflect.Complex64: types.Typ[types.Complex64], reflect.Complex128: types.Typ[types.Complex128],
	reflect.UnsafePointer: types.Typ[types.UnsafePointer],
}

func (m *Machine) goTypeOf1(rt reflect.Type) types.Type {
	if rt == errorType {
		return types.Universe.Lookup("error").Type()
	}
	switch rt {
	case rtNative:
		return m.reflectNamed("Type")
	case rvNative:
		return m.reflectNamed("Value")
	}
	if rt.Name() != "" {
		if rt.PkgPath() == "" {
			return basicByKind[rt.Kind()]
		}
		if m.vr == nil {
			return nil
		}
		path := origPath(rt.PkgPath())
		if p := m.vr.typesPkg(path); p != nil {
			name := rt.Name()
			if i := strings.IndexByte(name, '['); i >= 0 {
				return nil // instance of a generic type
			}
			if obj, ok := p.Scope().Lookup(name).(*types.TypeName); ok {
				return obj.Type()
			}
		}
		return nil
	}
	sub := func(t reflect.Type) types.Type { return m.goTypeOf(t) }
	switch rt.Kind() {
	case reflect.Pointer:
		if e := sub(rt.Elem()); e != nil {
			return types.NewPointer(e)
		}
	case reflect.Slice:
		if e := sub(rt.Elem()); e != nil {
			return types.NewSlice(e)
		}
	case reflect.Array:
		if e := sub(rt.Elem()); e != nil {
			return types.NewArray(e, int64(rt.Len()))
		}
	case reflect.Map:
		k, e := sub(rt.Key()), sub(rt.Elem())
		if k != nil && e != nil {
			return types.NewMap(k, e)
		}
	case reflect.Interface:
		if rt.NumMethod() == 0 {
			return emptyIface
		}
	case reflect.Chan:
		if e := sub(rt.Elem()); e != nil {
			dir := types.SendRecv
			switch rt.ChanDir() {
			case reflect.SendDir:
				dir = types.SendOnly
			case reflect.RecvDir:
				dir = types.RecvOnly
			}
			return types.NewChan(dir, e)
		}
	case reflect.Struct:
		// an unnamed struct, made by reflect.StructOf for example
		vars := make([]*types.Var, rt.NumField())
		tags := make([]string, rt.NumField())
		for i := range vars {
			f := rt.Field(i)
			e := sub(f.Type)
			if e == nil {
				return nil
			}
			var p *types.Package
			if f.PkgPath != "" {
				if m.vr != nil {
					p = m.vr.typesPkg(f.PkgPath)
				}
				if p == nil {
					p = types.NewPackage(f.PkgPath, path.Base(f.PkgPath))
				}
			}
			vars[i] = types.NewField(0, p, f.Name, e, f.Anonymous)
			tags[i] = string(f.Tag)
		}
		return types.NewStruct(vars, tags)
	case reflect.Func:
		var ps, rs []*types.Var
		for i := 0; i < rt.NumIn(); i++ {
			e := sub(rt.In(i))
			if e == nil {
				return nil
			}
			ps = append(ps, types.NewParam(0, nil, "", e))
		}
		for i := 0; i < rt.NumOut(); i++ {
			e := sub(rt.Out(i))
			if e == nil {
				return nil
			}
			rs = append(rs, types.NewParam(0, nil, "", e))
		}
		return types.NewSignatureType(nil, nil, nil, types.NewTuple(ps...), types.NewTuple(rs...), rt.IsVariadic())
	}
	return nil
}

// ---- reflect-style type strings ----

// typeString formats t like reflect.Type.String.
func typeString(t types.Type) string { return typeStringL(t, nil) }

// typeStringL is typeString with the names of local types (lt may be nil).
func typeStringL(t types.Type, lt *localTypes) string {
	var b strings.Builder
	w := typeWriter{b: &b, lt: lt}
	w.typ(t)
	return b.String()
}

// typeWriter formats types like the runtime type strings of gc.
type typeWriter struct {
	b  *strings.Builder
	lt *localTypes // names of types declared inside functions (may be nil)
	// link: inside type arguments, gc writes the "link string" of a type:
	// packages by path, and local types with their "·N" number.
	link bool
}

func (w *typeWriter) typ(t types.Type) {
	b := w.b
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		if rt := basicRT[t.Kind()]; rt != nil && t.Kind() != types.UntypedNil {
			b.WriteString(rt.String())
			return
		}
		switch t.Kind() {
		case types.Complex64:
			b.WriteString("complex64")
		case types.Complex128:
			b.WriteString("complex128")
		case types.UnsafePointer:
			b.WriteString("unsafe.Pointer")
		default:
			b.WriteString(t.Name())
		}
	case *types.Named:
		if p := t.Obj().Pkg(); p != nil {
			if w.link {
				b.WriteString(reflectPkgPath(p))
			} else {
				b.WriteString(p.Name())
			}
			b.WriteByte('.')
		}
		b.WriteString(t.Obj().Name())
		w.typeArgs(t)
		if w.link {
			if g := w.lt.gen(t); g > 0 {
				b.WriteString("·" + strconv.Itoa(g))
			}
		}
	case *types.Pointer:
		b.WriteByte('*')
		w.typ(t.Elem())
	case *types.Slice:
		b.WriteString("[]")
		w.typ(t.Elem())
	case *types.Array:
		b.WriteString("[" + strconv.FormatInt(t.Len(), 10) + "]")
		w.typ(t.Elem())
	case *types.Map:
		b.WriteString("map[")
		w.typ(t.Key())
		b.WriteByte(']')
		w.typ(t.Elem())
	case *types.Chan:
		switch t.Dir() {
		case types.SendOnly:
			b.WriteString("chan<- ")
		case types.RecvOnly:
			b.WriteString("<-chan ")
		default:
			b.WriteString("chan ")
		}
		w.typ(t.Elem())
	case *types.Signature:
		b.WriteString("func")
		w.sig(t)
	case *types.Interface:
		if t.NumMethods() == 0 {
			b.WriteString("interface {}")
			return
		}
		b.WriteString("interface { ")
		for i := 0; i < t.NumMethods(); i++ {
			if i > 0 {
				b.WriteString("; ")
			}
			f := t.Method(i)
			b.WriteString(f.Name())
			w.sig(f.Type().(*types.Signature))
		}
		b.WriteString(" }")
	case *types.Struct:
		if t.NumFields() == 0 {
			b.WriteString("struct {}")
			return
		}
		b.WriteString("struct { ")
		for i := 0; i < t.NumFields(); i++ {
			if i > 0 {
				b.WriteString("; ")
			}
			f := t.Field(i)
			if !f.Embedded() {
				b.WriteString(f.Name())
				b.WriteByte(' ')
			}
			w.typ(f.Type())
			if tag := t.Tag(i); tag != "" {
				b.WriteString(" " + strconv.Quote(tag))
			}
		}
		b.WriteString(" }")
	case *types.TypeParam:
		b.WriteString(t.Obj().Name())
	default:
		b.WriteString(t.String())
	}
}

// typeArgs writes the type arguments of t. A type declared inside a
// generic function also gets the type arguments of the function, before
// its own ones and separated by ';' (gc: "T[int;string]").
func (w *typeWriter) typeArgs(t *types.Named) {
	implicit := w.lt.implicitArgs(t)
	ta := t.TypeArgs()
	if len(implicit) == 0 && (ta == nil || ta.Len() == 0) {
		return
	}
	// reflect writes type arguments as link strings.
	in := typeWriter{b: w.b, lt: w.lt, link: true}
	w.b.WriteByte('[')
	for i, x := range implicit {
		if i > 0 {
			w.b.WriteByte(',')
		}
		in.typ(x)
	}
	for i := 0; ta != nil && i < ta.Len(); i++ {
		if i > 0 {
			w.b.WriteByte(',')
		} else if len(implicit) > 0 {
			w.b.WriteByte(';')
		}
		in.typ(ta.At(i))
	}
	w.b.WriteByte(']')
}

func (w *typeWriter) sig(s *types.Signature) {
	b := w.b
	b.WriteByte('(')
	for i := 0; i < s.Params().Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		pt := s.Params().At(i).Type()
		if s.Variadic() && i == s.Params().Len()-1 {
			b.WriteString("...")
			pt = pt.(*types.Slice).Elem()
		}
		w.typ(pt)
	}
	b.WriteByte(')')
	switch s.Results().Len() {
	case 0:
	case 1:
		b.WriteByte(' ')
		w.typ(s.Results().At(0).Type())
	default:
		b.WriteString(" (")
		for i := 0; i < s.Results().Len(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			w.typ(s.Results().At(i).Type())
		}
		b.WriteByte(')')
	}
}

// reflectPkgPath is the package path reported by reflect: the main package
// of a program loaded from files is "command-line-arguments" for go/packages
// but "main" for reflect.
func reflectPkgPath(p *types.Package) string {
	if p.Path() == "command-line-arguments" || p.Name() == "main" {
		return "main"
	}
	return p.Path()
}

var sizes = types.SizesFor("gc", runtime.GOARCH)

// ---- reflect.Type methods ----

func (vt *vtype) Kind() reflect.Kind { return vt.kind }

func (vt *vtype) String() string {
	if vt.isNat() {
		return natString(vt.nat)
	}
	return typeStringL(vt.t, vt.vr.localTypes())
}

func (vt *vtype) Name() string {
	if vt.isNat() {
		return vt.nat.Name()
	}
	switch n := types.Unalias(vt.t).(type) {
	case *types.Named:
		var b strings.Builder
		b.WriteString(n.Obj().Name())
		w := typeWriter{b: &b, lt: vt.vr.localTypes()}
		w.typeArgs(n)
		return b.String()
	case *types.Basic:
		return basicRT[n.Kind()].Name()
	}
	return ""
}

func (vt *vtype) PkgPath() string {
	if vt.isNat() {
		return vt.nat.PkgPath()
	}
	switch n := types.Unalias(vt.t).(type) {
	case *types.Named:
		if n.Obj().Pkg() != nil {
			return reflectPkgPath(n.Obj().Pkg())
		}
	case *types.Basic:
		if rt := basicRT[n.Kind()]; rt != nil {
			return rt.PkgPath() // "unsafe" for unsafe.Pointer
		}
	}
	if b, ok := types.Unalias(vt.t).(*types.Basic); ok && basicRT[b.Kind()] != nil {
		return basicRT[b.Kind()].PkgPath() // "unsafe" for unsafe.Pointer
	}
	return ""
}

func (vt *vtype) Size() uintptr {
	if vt.isNat() {
		return vt.nat.Size()
	}
	return uintptr(sizes.Sizeof(vt.t))
}

func (vt *vtype) Align() int {
	if vt.isNat() {
		return vt.nat.Align()
	}
	return int(sizes.Alignof(vt.t))
}

func (vt *vtype) FieldAlign() int {
	if vt.isNat() {
		return vt.nat.FieldAlign()
	}
	return int(sizes.Alignof(vt.t))
}

func (vt *vtype) Elem() RT {
	if vt.isNat() {
		return natT(vt.nat.Elem())
	}
	return vt.elemT().cn()
}

func (vt *vtype) Key() RT {
	if vt.isNat() {
		return natT(vt.nat.Key())
	}
	mt, ok := vt.t.Underlying().(*types.Map)
	if !ok {
		panic("reflect: Key of non-map type " + vt.String())
	}
	return vt.vr.vtypeOf(mt.Key()).cn()
}

func (vt *vtype) Len() int {
	if vt.isNat() {
		return vt.nat.Len()
	}
	at, ok := vt.t.Underlying().(*types.Array)
	if !ok {
		panic("reflect: Len of non-array type " + vt.String())
	}
	return int(at.Len())
}

func (vt *vtype) ChanDir() reflect.ChanDir {
	if vt.isNat() {
		return vt.nat.ChanDir()
	}
	ct, ok := vt.t.Underlying().(*types.Chan)
	if !ok {
		panic("reflect: ChanDir of non-chan type " + vt.String())
	}
	return chanDir(ct.Dir())
}

func (vt *vtype) structT() *types.Struct {
	st, ok := vt.t.Underlying().(*types.Struct)
	if !ok {
		panic("reflect: Field of non-struct type " + vt.String())
	}
	return st
}

func (vt *vtype) NumField() int {
	if vt.isNat() {
		return vt.nat.NumField()
	}
	return vt.structT().NumFields()
}

// StructField mirrors reflect.StructField.
type StructField struct {
	Name      string
	PkgPath   string
	Type      RT
	Tag       reflect.StructTag
	Offset    uintptr
	Index     []int
	Anonymous bool
}

// IsExported mirrors reflect.StructField.IsExported.
func (f StructField) IsExported() bool { return f.PkgPath == "" }

func natField(f reflect.StructField) StructField {
	return StructField{Name: f.Name, PkgPath: f.PkgPath, Type: natT(f.Type), Tag: f.Tag, Offset: f.Offset, Index: f.Index, Anonymous: f.Anonymous}
}

func (vt *vtype) Field(i int) StructField {
	if vt.isNat() {
		return natField(vt.nat.Field(i))
	}
	st := vt.structT()
	f := st.Field(i)
	sf := StructField{Name: f.Name(), Type: vt.fieldT(i).cn(), Tag: reflect.StructTag(st.Tag(i)), Index: []int{i}, Anonymous: f.Embedded()}
	if !f.Exported() && f.Pkg() != nil {
		sf.PkgPath = reflectPkgPath(f.Pkg())
	}
	sf.Offset = uintptr(vt.fieldOffsets()[i])
	return sf
}

func (vt *vtype) FieldByIndex(index []int) StructField {
	if vt.isNat() {
		return natField(vt.nat.FieldByIndex(index))
	}
	var f StructField
	t := vt
	for i, x := range index {
		if i > 0 && t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct {
			t = t.Elem()
		}
		f = t.Field(x)
		t = f.Type
	}
	if len(index) > 1 {
		f.Index = append([]int(nil), index...)
	}
	return f
}

func (vt *vtype) FieldByName(name string) (StructField, bool) {
	if vt.isNat() {
		f, ok := vt.nat.FieldByName(name)
		return natField(f), ok
	}
	return vt.FieldByNameFunc(func(s string) bool { return s == name })
}

// FieldByNameFunc follows reflect's breadth-first search over embedded
// structs (an ambiguous name at the shallowest depth is not found).
func (vt *vtype) FieldByNameFunc(match func(string) bool) (StructField, bool) {
	if vt.isNat() {
		f, ok := vt.nat.FieldByNameFunc(match)
		return natField(f), ok
	}
	type scan struct {
		t     RT
		index []int
	}
	var current []scan
	next := []scan{{t: vt}}
	visited := map[RT]bool{}
	for len(next) > 0 {
		current, next = next, nil
		count := map[RT]int{}
		var result StructField
		found := false
		for _, s := range current {
			if visited[s.t] {
				continue
			}
			visited[s.t] = true
			for i := 0; i < s.t.NumField(); i++ {
				f := s.t.Field(i)
				var ntyp RT
				if f.Anonymous {
					ntyp = f.Type
					if ntyp.Kind() == reflect.Pointer {
						ntyp = ntyp.Elem()
					}
				}
				if match(f.Name) {
					if count[s.t] > 0 || found {
						return StructField{}, false // ambiguous at this depth
					}
					result = f
					result.Index = append(append([]int(nil), s.index...), i)
					found = true
					count[s.t]++
					continue
				}
				if found || ntyp == nil || ntyp.Kind() != reflect.Struct {
					continue
				}
				next = append(next, scan{t: ntyp, index: append(append([]int(nil), s.index...), i)})
			}
		}
		if found {
			return result, true
		}
	}
	return StructField{}, false
}

// RMethod mirrors reflect.Method.
type RMethod struct {
	Name    string
	PkgPath string
	Type    RT
	Func    RV
	Index   int
}

// IsExported mirrors reflect.Method.IsExported.
func (m RMethod) IsExported() bool { return m.PkgPath == "" }

// exported returns the exported methods of vt.t sorted by name.
func (vt *vtype) exported() []*types.Selection {
	if p := vt.methods.Load(); p != nil {
		return *p
	}
	var out []*types.Selection
	for _, sel := range vt.vr.m.methodSet(vt.t) {
		if sel.Obj().Exported() {
			out = append(out, sel)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Obj().Name() < out[j].Obj().Name() })
	vt.methods.Store(&out)
	return out
}

func (vt *vtype) NumMethod() int {
	if vt.isNat() {
		return vt.nat.NumMethod()
	}
	if it, ok := vt.t.Underlying().(*types.Interface); ok {
		return it.NumMethods()
	}
	return len(vt.exported())
}

// natMethod is the twin of the method m of the native type t. The
// restricted machines get the replacements of vm/exit.go.
func natMethod(t reflect.Type, m reflect.Method) RMethod {
	m = restrictedMethodFunc(t, m)
	rm := RMethod{Name: m.Name, PkgPath: m.PkgPath, Type: natT(m.Type), Index: m.Index}
	if m.Func.IsValid() {
		rm.Func = natRV(m.Func)
	}
	return rm
}

func (vt *vtype) Method(i int) RMethod {
	if vt.isNat() {
		return natMethod(vt.nat, vt.nat.Method(i))
	}
	vr := vt.vr
	if it, ok := vt.t.Underlying().(*types.Interface); ok {
		f := it.Method(i)
		rm := RMethod{Name: f.Name(), Type: vr.vtypeOf(f.Type()).cn(), Index: i}
		if !f.Exported() && f.Pkg() != nil {
			rm.PkgPath = reflectPkgPath(f.Pkg())
		}
		return rm
	}
	sel := vt.exported()[i]
	sig := sel.Obj().Type().(*types.Signature)
	ps := []*types.Var{types.NewParam(0, nil, "", vt.t)}
	for j := 0; j < sig.Params().Len(); j++ {
		ps = append(ps, sig.Params().At(j))
	}
	ft := types.NewSignatureType(nil, nil, nil, types.NewTuple(ps...), sig.Results(), sig.Variadic())
	fvt := vr.vtypeOf(ft)
	fn := vr.m.prog.MethodValue(sel)
	m := vr.m
	cl := &Closure{Go: func(args []Value) Value { return m.Call(m.funcOf(fn), args, nil) }}
	if m.isInterp(fn) {
		cl.Direct = m.funcOf(fn)
	}
	return RMethod{Name: sel.Obj().Name(), Type: fvt.cn(), Func: RV{t: fvt, p: &Value{R: cl}}, Index: i}
}

func (vt *vtype) MethodByName(name string) (RMethod, bool) {
	if vt.isNat() {
		m, ok := vt.nat.MethodByName(name)
		if !ok {
			return RMethod{}, false
		}
		return natMethod(vt.nat, m), true
	}
	if it, ok := vt.t.Underlying().(*types.Interface); ok {
		for i := 0; i < it.NumMethods(); i++ {
			if it.Method(i).Name() == name {
				return vt.Method(i), true
			}
		}
		return RMethod{}, false
	}
	for i, sel := range vt.exported() {
		if sel.Obj().Name() == name {
			return vt.Method(i), true
		}
	}
	return RMethod{}, false
}

func (vt *vtype) sig() *types.Signature {
	s, ok := vt.t.Underlying().(*types.Signature)
	if !ok {
		panic("reflect: In/Out of non-func type " + vt.String())
	}
	return s
}

func (vt *vtype) NumIn() int {
	if vt.isNat() {
		return vt.nat.NumIn()
	}
	return vt.sig().Params().Len()
}

func (vt *vtype) In(i int) RT {
	if vt.isNat() {
		return natT(vt.nat.In(i))
	}
	return vt.vr.vtypeOf(vt.sig().Params().At(i).Type()).cn()
}

func (vt *vtype) NumOut() int {
	if vt.isNat() {
		return vt.nat.NumOut()
	}
	return vt.sig().Results().Len()
}

func (vt *vtype) Out(i int) RT {
	if vt.isNat() {
		return natT(vt.nat.Out(i))
	}
	return vt.vr.vtypeOf(vt.sig().Results().At(i).Type()).cn()
}

func (vt *vtype) IsVariadic() bool {
	if vt.isNat() {
		return vt.nat.IsVariadic()
	}
	return vt.sig().Variadic()
}

// ifaceOfRT returns the go/types interface of u for the machine of vt.
func (vr *vmReflect) ifaceOfRT(u RT) *types.Interface {
	if u.isNat() {
		return vr.resolveIface(u.nat)
	}
	it, _ := u.t.Underlying().(*types.Interface)
	return it
}

func (vt *vtype) Implements(u RT) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.Implements")
	}
	if u.kind != reflect.Interface {
		panic("reflect: non-interface type passed to Type.Implements")
	}
	switch {
	case vt.isNat() && u.isNat():
		if vt.nat.Implements(u.nat) {
			return true
		}
		// a json.Number of the host is the copy's (copyValTypeOf)
		ct := copyValTypeOf(vt.nat)
		return ct != nil && ct.Implements(u.nat)
	case vt.isNat():
		// Native type, VM interface: compare method names and arities.
		it := u.t.Underlying().(*types.Interface)
		for i := 0; i < it.NumMethods(); i++ {
			f := it.Method(i)
			m, ok := vt.nat.MethodByName(f.Name())
			if !ok {
				return false
			}
			s := f.Type().(*types.Signature)
			off := 0
			if vt.nat.Kind() != reflect.Interface {
				off = 1
			}
			if m.Type.NumIn()-off != s.Params().Len() || m.Type.NumOut() != s.Results().Len() {
				return false
			}
		}
		return true
	}
	it := vt.vr.ifaceOfRT(u)
	if it == nil {
		return false
	}
	return vt.vr.m.typesImplements(vt.t, it)
}

func (vt *vtype) AssignableTo(u RT) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.AssignableTo")
	}
	if vt == u {
		return true
	}
	switch {
	case vt.isNat() && u.isNat():
		return vt.nat.AssignableTo(u.nat)
	case u.kind == reflect.Interface:
		return vt.Implements(u)
	case vt.isNat() || u.isNat():
		return false
	}
	return types.AssignableTo(vt.t, u.t)
}

func (vt *vtype) ConvertibleTo(u RT) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.ConvertibleTo")
	}
	switch {
	case vt.isNat() && u.isNat():
		return vt.nat.ConvertibleTo(u.nat)
	case !vt.isNat() && !u.isNat():
		return types.ConvertibleTo(vt.t, u.t)
	case u.kind == reflect.Interface:
		return vt.Implements(u)
	}
	// One side native, the other a VM type: compare the native counterpart of
	// the VM type's underlying type (e.g. string -> interpreted `type S string`).
	a, b := vt, u
	if !a.isNat() {
		a, b = b, a
	}
	cu := b.vr.m.counterpart(b.t.Underlying())
	if cu == nil {
		return false
	}
	return a.nat.ConvertibleTo(cu) && cu.ConvertibleTo(a.nat)
}

func (vt *vtype) Comparable() bool {
	if vt.isNat() {
		return vt.nat.Comparable()
	}
	return types.Comparable(vt.t)
}

func (vt *vtype) Bits() int {
	if vt.isNat() {
		return vt.nat.Bits()
	}
	switch vt.kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return bitsOf(vt.t)
	}
	panic("reflect: Bits of non-arithmetic Type " + vt.String())
}

func (vt *vtype) OverflowInt(x int64) bool {
	if vt.isNat() {
		return vt.nat.OverflowInt(x)
	}
	return overflowInt(vt.t, x)
}

func (vt *vtype) OverflowUint(x uint64) bool {
	if vt.isNat() {
		return vt.nat.OverflowUint(x)
	}
	return overflowUint(vt.t, x)
}

func (vt *vtype) OverflowFloat(x float64) bool {
	if vt.isNat() {
		return vt.nat.OverflowFloat(x)
	}
	return overflowFloat(vt.t, x)
}

func (vt *vtype) OverflowComplex(x complex128) bool {
	if vt.isNat() {
		return vt.nat.OverflowComplex(x)
	}
	return natT(basicRT[types.Float64]).OverflowFloat(real(x))
}

func (vt *vtype) CanSeq() bool {
	if vt.isNat() {
		return vt.nat.CanSeq()
	}
	switch vt.kind {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint, reflect.Uintptr,
		reflect.Array, reflect.Slice, reflect.Chan, reflect.String, reflect.Map:
		return true
	case reflect.Func:
		return vt.canRangeFunc(1)
	}
	return false
}

func (vt *vtype) CanSeq2() bool {
	if vt.isNat() {
		return vt.nat.CanSeq2()
	}
	switch vt.kind {
	case reflect.Array, reflect.Slice, reflect.String, reflect.Map:
		return true
	case reflect.Func:
		return vt.canRangeFunc(2)
	}
	return false
}

// canRangeFunc reports whether the func type vt is func(yield func(...) bool)
// with n parameters for yield, as canRangeFunc and canRangeFunc2 of package
// reflect (an iter.Seq or iter.Seq2 of the program). In gc, the out count
// of a func type holds its variadic bit: a variadic func or yield is
// refused.
func (vt *vtype) canRangeFunc(n int) bool {
	if vt.NumIn() != 1 || vt.NumOut() != 0 || vt.IsVariadic() {
		return false
	}
	y := vt.In(0)
	return y.Kind() == reflect.Func && y.NumIn() == n && y.NumOut() == 1 && !y.IsVariadic() && y.Out(0).Kind() == reflect.Bool
}
