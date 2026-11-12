package vm

// Go-level twin of package reflect over VM values: package functions.

import (
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"unsafe"
)

// ValueOf mirrors reflect.ValueOf: VM values reach AOT code boxed (vmBoxed).
func ValueOf(x any) RV {
	switch x := x.(type) {
	case nil:
		return RV{}
	case vmBoxed:
		return fromBoxed(x, 0)
	}
	return natRV(reflect.ValueOf(x))
}

// TypeOf mirrors reflect.TypeOf.
func TypeOf(x any) RT {
	switch x := x.(type) {
	case nil:
		return nil
	case vmBoxed:
		o := x.obj()
		return o.m.vr.vtypeOf(o.iv.T.T).cn()
	}
	return natT(reflect.TypeOf(x))
}

// TypeFor mirrors reflect.TypeFor.
func TypeFor[T any]() RT { return natT(reflect.TypeFor[T]()) }

// TypeAssert mirrors reflect.TypeAssert.
func TypeAssert[T any](v RV) (T, bool) {
	var zero T
	if v.t == nil {
		panic(&reflect.ValueError{Method: "reflect.TypeAssert", Kind: reflect.Invalid})
	}
	x := v.Interface()
	if x == nil {
		return zero, false
	}
	if reflect.TypeFor[T]().Kind() == reflect.Interface {
		return AssertI[T](x)
	}
	r, ok := x.(T)
	return r, ok
}

// Zero mirrors reflect.Zero.
func Zero(t RT) RV {
	if t == nil {
		panic("reflect: Zero(nil)")
	}
	if t.isNat() {
		return natRV(reflect.Zero(t.nat))
	}
	return mkVM(t, t.vr.zeroCell(t.t), 0)
}

// New mirrors reflect.New.
func New(t RT) RV {
	if t == nil {
		panic("reflect: New(nil)")
	}
	if t.isNat() {
		return natRV(reflect.New(t.nat))
	}
	return mkVM(t.ptrT(), &Value{R: t.vr.zeroCell(t.t)}, 0)
}

// NewAt mirrors reflect.NewAt (native types only: VM memory has no address).
func NewAt(t RT, p unsafe.Pointer) RV {
	if f, ok := unsafeField(t, p); ok {
		return f.Addr() // field of a VM struct whose address was taken (UnsafeAddr)
	}
	if !t.isNat() {
		unsupported("vm reflect: NewAt on interpreted type %s", t)
	}
	return natRV(reflect.NewAt(t.nat, p))
}

// Indirect mirrors reflect.Indirect.
func Indirect(v RV) RV {
	if v.Kind() != reflect.Pointer {
		return v
	}
	return v.Elem()
}

// PointerTo mirrors reflect.PointerTo.
func PointerTo(t RT) RT {
	if t.isNat() {
		return natT(reflect.PointerTo(t.nat))
	}
	return t.ptrT().cn()
}

// PtrTo mirrors reflect.PtrTo.
func PtrTo(t RT) RT { return PointerTo(t) }

// vmOf returns the machine of the first VM type among ts (nil if all native).
func vmOf(ts ...RT) *vmReflect {
	for _, t := range ts {
		if t != nil && !t.isNat() {
			return t.vr
		}
	}
	return nil
}

// goT returns the go/types type of t for vr (native types are mapped).
func (vr *vmReflect) goT(t RT) types.Type {
	if !t.isNat() {
		return t.t
	}
	gt := vr.m.goTypeOf(t.nat)
	if gt == nil {
		unsupported("vm reflect: native type %s has no interpreted counterpart", t)
	}
	return gt
}

// SliceOf mirrors reflect.SliceOf.
func SliceOf(t RT) RT {
	if t.isNat() {
		return natT(reflect.SliceOf(t.nat))
	}
	return t.vr.vtypeOf(types.NewSlice(t.t)).cn()
}

// ArrayOf mirrors reflect.ArrayOf.
func ArrayOf(n int, t RT) RT {
	if t.isNat() {
		return natT(reflect.ArrayOf(n, t.nat))
	}
	return t.vr.vtypeOf(types.NewArray(t.t, int64(n))).cn()
}

// MapOf mirrors reflect.MapOf.
func MapOf(k, e RT) RT {
	vr := vmOf(k, e)
	if vr == nil {
		return natT(reflect.MapOf(k.nat, e.nat))
	}
	return vr.vtypeOf(types.NewMap(vr.goT(k), vr.goT(e))).cn()
}

// ChanOf mirrors reflect.ChanOf.
func ChanOf(dir reflect.ChanDir, t RT) RT {
	if t.isNat() {
		return natT(reflect.ChanOf(dir, t.nat))
	}
	d := types.SendRecv
	switch dir {
	case reflect.SendDir:
		d = types.SendOnly
	case reflect.RecvDir:
		d = types.RecvOnly
	}
	return t.vr.vtypeOf(types.NewChan(d, t.t)).cn()
}

// FuncOf mirrors reflect.FuncOf.
func FuncOf(in, out []RT, variadic bool) RT {
	vr := vmOf(append(append([]RT{}, in...), out...)...)
	if vr == nil {
		ni := make([]reflect.Type, len(in))
		for i := range in {
			ni[i] = in[i].nat
		}
		no := make([]reflect.Type, len(out))
		for i := range out {
			no[i] = out[i].nat
		}
		return natT(reflect.FuncOf(ni, no, variadic))
	}
	var ps, rs []*types.Var
	for _, t := range in {
		ps = append(ps, types.NewParam(0, nil, "", vr.goT(t)))
	}
	for _, t := range out {
		rs = append(rs, types.NewParam(0, nil, "", vr.goT(t)))
	}
	return vr.vtypeOf(types.NewSignatureType(nil, nil, nil, types.NewTuple(ps...), types.NewTuple(rs...), variadic)).cn()
}

// StructOf mirrors reflect.StructOf. With an interpreted field type, the
// struct is a VM type built with go/types.
func StructOf(fields []StructField) RT {
	ts := make([]RT, len(fields))
	for i, f := range fields {
		ts[i] = f.Type
	}
	if vr := vmOf(ts...); vr != nil {
		vars := make([]*types.Var, len(fields))
		tags := make([]string, len(fields))
		for i, f := range fields {
			if f.Name == "" {
				panic("reflect.StructOf: field " + strconv.Itoa(i) + " has no name")
			}
			if !token.IsExported(f.Name) && f.PkgPath == "" {
				panic("reflect.StructOf: field \"" + f.Name + "\" is unexported but missing PkgPath")
			}
			vars[i] = types.NewField(0, nil, f.Name, vr.goT(f.Type), f.Anonymous)
			tags[i] = string(f.Tag)
		}
		return vr.vtypeOf(types.NewStruct(vars, tags)).cn()
	}
	nf := make([]reflect.StructField, len(fields))
	for i, f := range fields {
		nf[i] = reflect.StructField{Name: f.Name, PkgPath: f.PkgPath, Type: f.Type.nat, Tag: f.Tag, Offset: f.Offset, Index: f.Index, Anonymous: f.Anonymous}
	}
	return natT(reflect.StructOf(nf))
}

// MakeSlice mirrors reflect.MakeSlice.
func MakeSlice(t RT, n, c int) RV {
	if t.isNat() {
		return natRV(reflect.MakeSlice(t.nat, n, c))
	}
	if t.kind != reflect.Slice {
		panic("reflect.MakeSlice of non-slice type")
	}
	if n < 0 || c < n {
		panic("reflect.MakeSlice: len out of range")
	}
	et := t.t.Underlying().(*types.Slice).Elem()
	if isByteType(et) {
		// A []byte, as make([]byte, n) of the program (bytes.go): Bytes
		// returns the slice itself, and code that reads into it (the copy
		// of encoding/gob for a named []byte type) writes the slice.
		return mkVM(t, &Value{R: make([]byte, n, c)}, 0)
	}
	return mkVM(t, &Value{R: t.vr.makeSlice(et, n, c)}, 0)
}

// MakeMap mirrors reflect.MakeMap.
func MakeMap(t RT) RV { return MakeMapWithSize(t, 0) }

// MakeMapWithSize mirrors reflect.MakeMapWithSize.
func MakeMapWithSize(t RT, n int) RV {
	if t.isNat() {
		return natRV(reflect.MakeMapWithSize(t.nat, n))
	}
	if t.kind != reflect.Map {
		panic("reflect.MakeMapWithSize of non-map type")
	}
	return mkVM(t, &Value{R: newMapV(n)}, 0)
}

// MakeChan mirrors reflect.MakeChan.
func MakeChan(t RT, size int) RV {
	if t.isNat() {
		return natRV(reflect.MakeChan(t.nat, size))
	}
	return mkVM(t, &Value{R: make(chan Value, size)}, 0)
}

// MakeFunc mirrors reflect.MakeFunc.
func MakeFunc(t RT, fn func(args []RV) (results []RV)) RV {
	return makeFuncP(t, func(_ *panicState, args []RV) []RV { return fn(args) })
}

// makeFuncP is MakeFunc for an fn that gets the panic that runs a deferred
// call of the made function (nil otherwise): see Closure.GoP.
func makeFuncP(t RT, fnp func(p *panicState, args []RV) (results []RV)) RV {
	fn := func(args []RV) []RV { return fnp(nil, args) }
	if t.kind != reflect.Func {
		panic("reflect: call of MakeFunc with non-Func type")
	}
	if t.isNat() {
		return natRV(reflect.MakeFunc(t.nat, func(in []reflect.Value) []reflect.Value {
			args := make([]RV, len(in))
			for i := range in {
				args[i] = natRV(in[i])
			}
			res := fn(args)
			checkMakeFuncResults(res, t.nat.NumOut())
			for i, r := range res {
				if rt, ot := r.Type(), t.Out(i); !twinAssignable(rt, ot) {
					panic("reflect.MakeFunc: value of type " + rt.String() + " is not assignable to type " + ot.String())
				}
			}
			out := make([]reflect.Value, len(res))
			for i := range res {
				out[i] = res[i].natVal(t.nat.Out(i))
			}
			return out
		}))
	}
	vr := t.vr
	sig := t.sig()
	body := func(p *panicState, in []Value) Value {
		args := make([]RV, len(in))
		for i := range in {
			c := new(Value)
			*c = in[i]
			args[i] = mkVM(vr.vtypeOf(sig.Params().At(i).Type()), c, 0)
		}
		res := fnp(p, args)
		checkMakeFuncResults(res, sig.Results().Len())
		for i, r := range res {
			if rt, ot := r.Type(), t.Out(i); !twinAssignable(rt, ot) {
				panic("reflect.MakeFunc: value of type " + rt.String() + " is not assignable to type " + ot.String())
			}
		}
		switch len(res) {
		case 0:
			return Value{}
		case 1:
			return res[0].vmVal(vr.m, sig.Results().At(0).Type())
		}
		out := make([]Value, len(res))
		for i := range res {
			out[i] = res[i].vmVal(vr.m, sig.Results().At(i).Type())
		}
		return Value{R: out}
	}
	cl := &Closure{Go: func(in []Value) Value { return body(nil, in) }, GoP: body}
	return mkVM(t, &Value{R: cl}, 0)
}

// checkMakeFuncResults does the checks of reflect on the results of a
// function made by MakeFunc (a zero Value would become a zero result).
func checkMakeFuncResults(res []RV, n int) {
	if len(res) != n {
		panic("reflect: wrong return count from function created by MakeFunc")
	}
	for _, r := range res {
		if r.t == nil {
			panic("reflect: function created by MakeFunc using closure returned zero Value")
		}
	}
}

// Append mirrors reflect.Append.
func Append(s RV, x ...RV) RV {
	if n, ok := s.native(); ok {
		nx := make([]reflect.Value, len(x))
		for i := range x {
			nx[i] = x[i].natVal(n.Type().Elem())
		}
		return natRV(reflect.Append(n, nx...))
	}
	if s.Kind() != reflect.Slice {
		panic(&reflect.ValueError{Method: "reflect.Append", Kind: s.Kind()})
	}
	m := s.mach()
	et := s.t.elemT().t
	switch b := s.p.R.(type) {
	case []byte:
		if len(x) > cap(b)-len(b) {
			// The elements go in one append, as in Go: when they do not
			// all fit, none of them is written into the array of b, which
			// another slice (or the host) may share.
			tmp := make([]byte, len(x))
			for i, e := range x {
				tmp[i] = byte(e.vmVal(m, et).N)
			}
			return mkVM(s.t, &Value{R: append(b, tmp...)}, 0)
		}
		m.hostBytesAppend(b, len(x))
		for _, e := range x {
			b = append(b, byte(e.vmVal(m, et).N))
		}
		return mkVM(s.t, &Value{R: b}, 0)
	}
	vs, _ := s.p.R.([]Value)
	if needsCopy(et) {
		// struct or array elements: the same rules as the builtin append
		xs := make([]Value, len(x))
		for i, e := range x {
			xs[i] = e.vmVal(m, et)
		}
		return mkVM(s.t, &Value{R: appendValues(m, vs, xs, et)}, 0)
	}
	for _, e := range x {
		vs = append(vs, e.vmVal(m, et))
	}
	return mkVM(s.t, &Value{R: vs}, 0)
}

// AppendSlice mirrors reflect.AppendSlice.
func AppendSlice(s, t RV) RV {
	if n, ok := s.native(); ok {
		return natRV(reflect.AppendSlice(n, t.natVal(n.Type())))
	}
	xs := make([]RV, t.Len())
	for i := range xs {
		xs[i] = t.Index(i)
	}
	return Append(s, xs...)
}

// Copy mirrors reflect.Copy.
func Copy(dst, src RV) int {
	dn, dok := dst.native()
	sn, sok := src.native()
	if dok && sok {
		dst.writer().hostWriteIn(dn)
		return reflect.Copy(dn, sn)
	}
	if dst.p != nil && !dok {
		if db, ok := dst.p.R.([]byte); ok {
			dst.writer().hostBytes(db, min(len(db), src.Len()))
			if src.Kind() == reflect.String {
				return copy(db, src.String())
			}
			return copy(db, src.Bytes())
		}
	}
	if src.Kind() == reflect.String && dst.Kind() == reflect.Slice {
		s := src.String()
		n := min(len(s), dst.Len())
		for i := 0; i < n; i++ {
			dst.Index(i).SetUint(uint64(s[i]))
		}
		return n
	}
	n := min(dst.Len(), src.Len())
	if dst.p != nil && src.p != nil && !dok && !sok {
		dv, dvok := dst.p.R.([]Value)
		sv, svok := src.p.R.([]Value)
		if dvok && svok {
			if et := dst.t.elemT().t; needsCopy(et) {
				return copyValues(dv, sv, et) // as the builtin copy
			}
			if wordKind(dst.t.elemT().t) {
				return copyWords(dv, sv)
			}
			return copy(dv, sv)
		}
	}
	tmp := make([]RV, n)
	for i := 0; i < n; i++ {
		tmp[i] = src.Index(i)
	}
	for i := 0; i < n; i++ {
		dst.Index(i).Set(tmp[i])
	}
	return n
}

// Swapper mirrors reflect.Swapper.
func Swapper(slice any) func(i, j int) {
	v := ValueOf(slice)
	if v.Kind() != reflect.Slice {
		panic(&reflect.ValueError{Method: "Swapper", Kind: v.Kind()})
	}
	if n, ok := v.native(); ok {
		return reflect.Swapper(n.Interface())
	}
	switch s := v.p.R.(type) {
	case []Value:
		if wordKind(v.t.elemT().t) {
			return func(i, j int) { swapWords(s, i, j) }
		}
		return func(i, j int) { s[i], s[j] = s[j], s[i] }
	case []byte:
		return func(i, j int) { s[i], s[j] = s[j], s[i] }
	}
	return func(i, j int) { panic("reflect: slice index out of range") }
}

// SelectCase mirrors reflect.SelectCase.
type SelectCase struct {
	Dir  reflect.SelectDir
	Chan RV
	Send RV
}

// Select mirrors reflect.Select.
func Select(cases []SelectCase) (chosen int, recv RV, recvOK bool) {
	return selectStop(cases)
}

// selectStop is Select, whose wait also ends when one of stops is closed
// (a nil stop never is): chosen is then -1.
func selectStop(cases []SelectCase, stops ...chan struct{}) (chosen int, recv RV, recvOK bool) {
	nc := make([]reflect.SelectCase, len(cases), len(cases)+len(stops))
	for i, c := range cases {
		nc[i].Dir = c.Dir
		if !c.Chan.IsValid() {
			continue
		}
		if n, ok := c.Chan.native(); ok {
			nc[i].Chan = n
			if c.Dir == reflect.SelectSend {
				nc[i].Send = c.Send.natVal(n.Type().Elem())
			}
			continue
		}
		ch := c.Chan.vmChan()
		if ch == nil {
			nc[i].Chan = reflect.Zero(chanValueType)
		} else {
			nc[i].Chan = reflect.ValueOf(ch)
		}
		if c.Dir == reflect.SelectSend {
			nc[i].Send = reflect.ValueOf(c.Send.vmVal(c.Chan.mach(), c.Chan.chanElem().t))
		}
	}
	for _, stop := range stops {
		if stop != nil {
			nc = append(nc, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(stop)})
		}
	}
	chosen, r, ok := reflect.Select(nc)
	if chosen >= len(cases) {
		return -1, RV{}, false
	}
	c := cases[chosen]
	if c.Dir != reflect.SelectRecv {
		return chosen, RV{}, false
	}
	if _, nat := c.Chan.native(); nat {
		return chosen, natRV(r), ok
	}
	if !ok {
		return chosen, c.Chan.zeroOf(c.Chan.chanElem()), false
	}
	x := r.Interface().(Value)
	return chosen, mkVM(c.Chan.chanElem(), &x, 0), true
}

// VisibleFields mirrors reflect.VisibleFields.
func VisibleFields(t RT) []StructField {
	if t == nil {
		panic("reflect: VisibleFields(nil)")
	}
	if t.Kind() != reflect.Struct {
		panic("reflect.VisibleFields of non-struct type")
	}
	if t.isNat() {
		fs := reflect.VisibleFields(t.nat)
		out := make([]StructField, len(fs))
		for i := range fs {
			out[i] = natField(fs[i])
		}
		return out
	}
	w := &visibleFieldsWalker{byName: map[string]int{}, visiting: map[RT]bool{}}
	w.walk(t)
	j := 0
	for i := range w.fields {
		f := &w.fields[i]
		if f.Name == "" {
			continue
		}
		if i != j {
			w.fields[j] = *f
		}
		j++
	}
	return w.fields[:j]
}

type visibleFieldsWalker struct {
	byName   map[string]int
	visiting map[RT]bool
	fields   []StructField
	index    []int
}

func (w *visibleFieldsWalker) walk(t RT) {
	if w.visiting[t] {
		return
	}
	w.visiting[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		w.index = append(w.index, i)
		add := true
		if oldIndex, ok := w.byName[f.Name]; ok {
			old := &w.fields[oldIndex]
			if len(w.index) == len(old.Index) {
				old.Name = ""
				add = false
			} else if len(w.index) < len(old.Index) {
				old.Name = ""
			} else {
				add = false
			}
		}
		if add {
			f.Index = append([]int(nil), w.index...)
			w.byName[f.Name] = len(w.fields)
			w.fields = append(w.fields, f)
		}
		if f.Anonymous {
			if f.Type.Kind() == reflect.Pointer {
				f.Type = f.Type.Elem()
			}
			if f.Type.Kind() == reflect.Struct {
				w.walk(f.Type)
			}
		}
		w.index = w.index[:len(w.index)-1]
	}
	delete(w.visiting, t)
}

// DeepEqual mirrors reflect.DeepEqual.
func DeepEqual(x, y any) bool {
	_, bx := x.(vmBoxed)
	_, by := y.(vmBoxed)
	if !bx && !by {
		return reflect.DeepEqual(x, y)
	}
	if x == nil || y == nil {
		return x == y
	}
	v1, v2 := ValueOf(x), ValueOf(y)
	if v1.Type() != v2.Type() {
		return false
	}
	return deepValueEqual(v1, v2, map[[2]uintptr]bool{})
}

func deepValueEqual(v1, v2 RV, visited map[[2]uintptr]bool) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}
	switch v1.Kind() {
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface:
		if v1.Kind() != reflect.Interface && !v1.IsNil() && !v2.IsNil() {
			k := [2]uintptr{v1.Pointer(), v2.Pointer()}
			if k[0] > k[1] {
				k[0], k[1] = k[1], k[0]
			}
			if visited[k] {
				return true
			}
			visited[k] = true
		}
	}
	switch v1.Kind() {
	case reflect.Array:
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if v1.IsNil() != v2.IsNil() || v1.Len() != v2.Len() {
			return false
		}
		if v1.Pointer() == v2.Pointer() {
			return true
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case reflect.Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case reflect.Pointer:
		if v1.Pointer() == v2.Pointer() {
			return true
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case reflect.Struct:
		for i := 0; i < v1.NumField(); i++ {
			if !deepValueEqual(v1.Field(i), v2.Field(i), visited) {
				return false
			}
		}
		return true
	case reflect.Map:
		if v1.IsNil() != v2.IsNil() || v1.Len() != v2.Len() {
			return false
		}
		if v1.Pointer() == v2.Pointer() {
			return true
		}
		it := v1.MapRange()
		for it.Next() {
			val2 := v2.MapIndex(it.Key())
			if !val2.IsValid() || !deepValueEqual(it.Value(), val2, visited) {
				return false
			}
		}
		return true
	case reflect.Func:
		return v1.IsNil() && v2.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v1.Int() == v2.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v1.Uint() == v2.Uint()
	case reflect.String:
		return v1.String() == v2.String()
	case reflect.Bool:
		return v1.Bool() == v2.Bool()
	case reflect.Float32, reflect.Float64:
		return v1.Float() == v2.Float()
	}
	return v1.Equal(v2)
}
