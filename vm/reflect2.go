package vm

// The functions of the runtime that github.com/modern-go/reflect2 (used by
// json-iterator) reaches with //go:linkname, on interpreted values. They
// have no Go body, and the runtime versions read gc's memory layout, which
// interpreted values do not have. The rtype arguments are the data words of
// reflect.Type values, which are type words in the VM (ifaceview.go).

import (
	"fmt"
	"go/types"
	"reflect"
	"unsafe"
)

func init() {
	const p = "github.com/modern-go/reflect2."
	vmHooks[p+"mapiterinit"] = func(m *Machine, a []Value) Value { m.r2IterInit(a[0], a[1], a[2]); return Value{} }
	vmHooks[p+"mapiternext"] = func(m *Machine, a []Value) Value { m.r2IterNext(a[0]); return Value{} }
	vmHooks[p+"unsafe_New"] = func(m *Machine, a []Value) Value { return m.r2New(r2Type(a[0])) }
	vmHooks[p+"unsafe_NewArray"] = func(m *Machine, a []Value) Value { return m.r2NewArray(r2Type(a[0]), a[1].N) }
	vmHooks[p+"typedmemmove"] = func(m *Machine, a []Value) Value {
		t := r2Type(a[0])
		s := newMemSite(t)
		m.store(a[1], m.load(a[2], s), s)
		return Value{}
	}
	vmHooks[p+"typedslicecopy"] = func(m *Machine, a []Value) Value { return Value{N: int64(m.r2SliceCopy(r2Type(a[0]), a[1], a[2]))} }
	vmHooks[p+"makemap"] = func(m *Machine, a []Value) Value {
		r2Map(r2Type(a[0]))
		return Value{R: newMapV(0)}
	}
	vmHooks[p+"mapassign"] = func(m *Machine, a []Value) Value { m.r2MapAssign(a[0], a[1], a[2], a[3]); return Value{} }
	vmHooks[p+"mapaccess"] = func(m *Machine, a []Value) Value { return m.r2MapAccess(a[0], a[1], a[2]) }
	vmHooks[p+"ifaceE2I"] = func(m *Machine, a []Value) Value {
		// *dst = src, dst a non-empty interface of type rtype
		t := r2Type(a[0])
		if iv, ok := a[1].R.(*Iface); ok && iv != nil {
			if it, ok := t.Underlying().(*types.Interface); ok && (iv.T.T == nil || !types.Implements(iv.T.T, it)) {
				panic(plainError(fmt.Sprintf("interface conversion: %s is not %s", iv.T, t)))
			}
		}
		m.store(a[2], a[1], newMemSite(t))
		return Value{}
	}
}

// r2Type returns the type that rtype (the data word of a reflect.Type of
// the program) names; refused for a native type.
func r2Type(rtype Value) types.Type {
	t := typeOfWord2(rtype)
	if t == nil {
		unsupported("reflect2: a native reflect.Type (the VM emulates the runtime of reflect2 for the types of the program)")
	}
	return t
}

func r2Map(t types.Type) *types.Map {
	mt, ok := t.Underlying().(*types.Map)
	if !ok {
		unsupported("reflect2: a map function on a %s", t)
	}
	return mt
}

// r2New is reflect.unsafe_New(rtype): a pointer to a new zero value.
func (m *Machine) r2New(t types.Type) Value {
	c := m.zeroValue(t)
	p := Value{R: &c}
	m.rememberPointee(p, types.NewPointer(t))
	return p
}

// r2NewArray is reflect.unsafe_NewArray(rtype, n): a pointer to the first
// of n new zero values, in an array whose region is recorded, so that a
// slice header with this data gets the whole array (reflect2's
// UnsafeMakeSlice). n has the bounds of make([]T, n): the length check of
// gc, and the zero values of aggregate elements (checkMakeSlice). The
// elements of a type of size 0 cost nothing in gc but a Value each here:
// too many of them are refused instead of a fatal out-of-memory. The
// plugin may ship its own reflect2, so n is any value.
func (m *Machine) r2NewArray(et types.Type, n64 int64) Value {
	if n64 < 0 {
		panic(plainError("reflect: negative length"))
	}
	size := gcSizes.Sizeof(et)
	makeSliceCheck(n64, n64, uint64(size))
	if size == 0 && n64 > maxZeroElems {
		panic(tooLarge(fmt.Sprintf("unsafe_NewArray of %d elements", n64), types.NewSlice(et)))
	}
	checkMakeSlice(n64, m.zeroEntries(et), et)
	n := int(n64)
	if isByteType(et) {
		b := make([]byte, n+1)[:n] // a pointer to b[0] also for n == 0
		m.addRegion(&uregion{base: uintptr(unsafe.Pointer(&b[:1][0])), bytes: b[:n]})
		return bytePtr(&b[:1][0], false)
	}
	es := make([]Value, n+1)[:n]
	for i := range es {
		es[i] = m.zeroValue(et)
	}
	p := Value{R: &es[:1][0]}
	if n > 0 {
		m.addRegion(&uregion{base: addrOf(p), elems: es, t: et})
	}
	return p
}

// r2Cells returns the n elements of type et from the data pointer p of a
// slice header, in the memory the regions know.
func (m *Machine) r2Cells(p Value, n int, et types.Type) ([]Value, []byte) {
	if n == 0 {
		return nil, nil
	}
	r := m.findRegionPtr(p)
	if r == nil {
		unsupported("reflect2: slice data that the VM does not know")
	}
	i, _ := r.index(p)
	if r.bytes != nil {
		if i+n > len(r.bytes) {
			unsupported("reflect2: a slice header outside its array")
		}
		return nil, r.bytes[i : i+n]
	}
	if i+n > len(r.elems) {
		unsupported("reflect2: a slice header outside its array")
	}
	return r.elems[i : i+n], nil
}

// r2SliceCopy is reflect.typedslicecopy(et, dst, src): dst and src are
// reflect2 sliceHeader values (Data, Len, Cap).
func (m *Machine) r2SliceCopy(et types.Type, dst, src Value) int {
	d, _ := dst.R.([]Value)
	s, _ := src.R.([]Value)
	if len(d) != 3 || len(s) != 3 {
		unsupported("reflect2: typedslicecopy of %T", dst.R)
	}
	n := int(min(d[1].N, s[1].N))
	dc, db := m.r2Cells(d[0], n, et)
	sc, sb := m.r2Cells(s[0], n, et)
	switch {
	case db != nil && sb != nil:
		return copy(db, sb)
	case db == nil && sb == nil:
		return copyValues(dc, sc, et)
	}
	unsupported("reflect2: typedslicecopy between bytes and cells")
	return 0
}

// r2MapAssign is reflect.mapassign(rtype, m, key, val): m[*key] = *val.
func (m *Machine) r2MapAssign(rtype, mp, key, val Value) {
	mt := r2Map(r2Type(rtype))
	x, ok := mp.R.(*MapV)
	if !ok {
		if mp.R == nil {
			panic(errNilMapAssign)
		}
		unsupported("reflect2: mapassign on a %T", mp.R)
	}
	k := m.load(key, newMemSite(mt.Key()))
	v := m.load(val, newMemSite(mt.Elem()))
	kk := storeKey(keyOf(k, mt.Key()))
	x.M[kk] = v
	x.setKey(kk, k)
}

// r2MapAccess is reflect.mapaccess(rtype, m, key): a pointer to a copy of
// m[*key], or nil.
func (m *Machine) r2MapAccess(rtype, mp, key Value) Value {
	mt := r2Map(r2Type(rtype))
	x, ok := mp.R.(*MapV)
	if !ok {
		if mp.R == nil {
			return Value{}
		}
		unsupported("reflect2: mapaccess on a %T", mp.R)
	}
	k := m.load(key, newMemSite(mt.Key()))
	v, ok := x.M[keyOf(k, mt.Key())]
	if !ok {
		return Value{}
	}
	return m.r2Cell(v, mt.Elem())
}

// r2Iter is the state of a map iteration of reflect2, kept in the h field
// of its hiter: the entries of the map when the iteration started.
type r2Iter struct {
	keys, vals []Value
	kt, et     types.Type
	i          int
}

// hiter fields that the iteration uses (reflect2's hiter, like the
// runtime's: key, value, t, h, ...).
const (
	hiterKey   = 0
	hiterValue = 1
	hiterState = 3
)

// typeOfWord2 returns the type that the type word w (the data word of a
// reflect.Type) names, or nil.
func typeOfWord2(w Value) types.Type {
	c, ok := w.R.(*Value)
	if !ok || c == nil {
		return nil
	}
	if tok, ok := c.R.(typeTok); ok {
		return tok.rt.T
	}
	return nil
}

// hiterFields returns the cells of the hiter it points to.
func hiterFields(it Value) []Value {
	c, ok := it.R.(*Value)
	if !ok || c == nil {
		unsupported("reflect2: map iteration on a %T", it.R)
	}
	fs, ok := c.R.([]Value)
	if !ok || len(fs) <= hiterState {
		unsupported("reflect2: map iteration on a hiter of %T", c.R)
	}
	return fs
}

// r2IterInit is reflect.mapiterinit(rtype, m, it): it starts an iteration
// over the map m. The key and value words point to copies of the entry
// (like the runtime's, they are valid until the next call; a write through
// them does not reach the map).
func (m *Machine) r2IterInit(rtype, mp, it Value) {
	mt, ok := typeOfWord2(rtype).(*types.Map)
	if !ok {
		if t := typeOfWord2(rtype); t != nil {
			mt, ok = t.Underlying().(*types.Map)
		}
	}
	if !ok {
		unsupported("reflect2: map iteration with a type that is not a map of the program")
	}
	s := &r2Iter{kt: mt.Key(), et: mt.Elem()}
	switch x := mp.R.(type) {
	case nil:
	case *MapV:
		x.each(func(k, v Value) {
			s.keys = append(s.keys, k)
			s.vals = append(s.vals, v)
		})
	case reflect.Value:
		for r := x.MapRange(); r.Next(); {
			s.keys = append(s.keys, m.fromNative(r.Key(), s.kt))
			s.vals = append(s.vals, m.fromNative(r.Value(), s.et))
		}
	default:
		unsupported("reflect2: map iteration over a %T", mp.R)
	}
	fs := hiterFields(it)
	fs[hiterState] = Value{R: s}
	m.r2IterSet(fs, s)
}

// r2IterNext is reflect.mapiternext(it).
func (m *Machine) r2IterNext(it Value) {
	fs := hiterFields(it)
	s, ok := fs[hiterState].R.(*r2Iter)
	if !ok {
		unsupported("reflect2: mapiternext without mapiterinit")
	}
	s.i++
	m.r2IterSet(fs, s)
}

// r2IterSet points the key and value words of the hiter to the current
// entry, or sets them to nil at the end.
func (m *Machine) r2IterSet(fs []Value, s *r2Iter) {
	if s.i >= len(s.keys) {
		fs[hiterKey], fs[hiterValue] = Value{}, Value{}
		return
	}
	fs[hiterKey] = m.r2Cell(s.keys[s.i], s.kt)
	fs[hiterValue] = m.r2Cell(s.vals[s.i], s.et)
}

// r2Cell returns a pointer to a copy of v, of type t, whose fields offsets
// from the pointer reach (a struct value of a map).
func (m *Machine) r2Cell(v Value, t types.Type) Value {
	c := deepCopy(v, t)
	p := Value{R: &c}
	m.rememberPointee(p, types.NewPointer(t))
	return p
}
