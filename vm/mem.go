package vm

import (
	"go/types"
	"reflect"
	"unsafe"
)

// Generic (slow-path) memory helpers used by the superinstructions; they
// mirror OpFieldAddr / OpIndexAddr / OpLoad / OpStore.

func (m *Machine) fieldAddr(base Value, f int32) Value {
	switch p := base.R.(type) {
	case *Value:
		switch s := p.R.(type) {
		case []Value:
			return Value{R: &s[f]}
		case reflect.Value:
			return Value{R: fieldAddr(s, int(f))}
		}
	case reflect.Value:
		return Value{R: fieldAddr(p.Elem(), int(f))}
	case *hdrView:
		return Value{R: &hdrWord{v: p, word: int(f)}}
	case *byteView:
		return p.field(int(f))
	case badAddr:
		p.refuse()
	}
	panic(errNilDeref)
}

func (m *Machine) indexAddr(base Value, i int64, flags uint8) Value {
	switch s := base.R.(type) {
	case []Value:
		checkIndex(i, len(s), flags)
		return Value{R: &s[i]}
	case []byte:
		checkIndex(i, len(s), flags)
		return Value{R: &s[i]}
	case *Value:
		if s == nil {
			break
		}
		if z, ok := s.R.(*zslice); ok {
			checkIndex(i, len(z.s), flags)
			return z.addr(i)
		}
		if b, ok := s.R.([]byte); ok {
			// a byte array that is only sliced and written element by
			// element (byteArrayOnlySliced): real bytes
			checkIndex(i, len(b), flags)
			return Value{R: &b[i]}
		}
		arr := s.R.([]Value)
		checkIndex(i, len(arr), flags)
		return Value{R: &arr[i]}
	case *zslice:
		checkIndex(i, len(s.s), flags)
		return s.addr(i)
	case *hdrView:
		return s.wordAddr(i)
	case *byteView:
		return s.index(i, flags)
	case reflect.Value:
		if s.Kind() == reflect.Pointer {
			if s.IsNil() {
				break
			}
			s = s.Elem()
		}
		checkIndex(i, s.Len(), flags)
		return Value{R: s.Index(int(i)).Addr()}
	case nil:
		if flags&idxPtr == 0 {
			panic(indexError(i, 0, flags)) // nil slice
		}
	}
	panic(errNilDeref)
}

func (m *Machine) load(addr Value, s *memSite) Value {
	switch p := addr.R.(type) {
	case *Value:
		if s.copyLoad() {
			return deepCopy(*p, s.t)
		}
		if s.word {
			return loadWord(p)
		}
		return *p
	case *byte:
		return Value{N: int64(*p)}
	case roByte:
		return Value{N: int64(*p)}
	case reflect.Value:
		return m.loadNative(p, s)
	case *hdrView:
		return p.load(s.t)
	case *hdrWord:
		return p.load(s.t)
	case *byteView:
		return p.load(s.t)
	case badAddr:
		p.refuse()
	}
	panic(errNilDeref)
}

// loadNative loads the native memory that p points to (a field of a native
// struct, a native global). A native struct or array is a copy when the site
// copies (copyLoad, as a load from a cell): fromNative keeps the addressable
// reflect.Value of the place, and a Value that keeps the place follows later
// writes to it (e := c.Expires; c.Expires = t made e equal to t). A shallow
// load keeps the place: its consumer copies it before anything else runs.
func (m *Machine) loadNative(p reflect.Value, s *memSite) Value {
	return m.loadNativeValue(p.Elem(), s)
}

// loadNativeValue is loadNative of the variable e itself (p.Elem()).
func (m *Machine) loadNativeValue(e reflect.Value, s *memSite) Value {
	v := m.fromNative(e, s.t)
	if s.copyLoad() {
		if rv, ok := v.R.(reflect.Value); ok && rv.CanAddr() {
			return deepCopy(v, s.t)
		}
	}
	return v
}

func (m *Machine) store(addr Value, v Value, s *memSite) {
	switch p := addr.R.(type) {
	case *Value:
		if s.deep {
			assign(p, v, s.t)
		} else if s.word {
			storeWord(p, v)
		} else {
			*p = v
		}
		return
	case *byte:
		// No freePage cache here (cx.hostByte, as OpStore and OpIndexStore
		// in exec.go): store has no ctx, and some callers (errors.go,
		// reflect2.go) have none to give. This path is the fallback of
		// fieldAddr and indexAddr, and a full check costs no measurable
		// time on it.
		if m.restricted {
			m.hostWriteAt(unsafe.Pointer(p))
		}
		*p = byte(v.N)
		return
	case roByte:
		refuseROWrite()
	case reflect.Value:
		m.hostWritePtr(p)
		setNative(p.Elem(), m.toNative(v, s.t))
		return
	case *hdrView:
		p.store(v, s.t)
		return
	case *hdrWord:
		p.store(v, s.t)
		return
	case *byteView:
		p.store(v, s.t)
		return
	case badAddr:
		p.refuse()
	}
	panic(errNilDeref)
}

// assign stores v (of type t) into the cell p. A struct or an array is
// copied into the memory that p already holds, element by element, and is
// not replaced by a new []Value: pointers into that memory (&a[i], &s.f, an
// array pointer made from a slice) must see the new value, as in Go.
func assign(p *Value, v Value, t types.Type) {
	var elem func(i int) types.Type
	switch u := t.Underlying().(type) {
	case *types.Struct:
		if dst, ok := p.R.(reflect.Value); ok && dst.CanSet() {
			// A native struct in a cell (the field c of an interpreted
			// struct, c http.Cookie): the new value goes into the memory
			// the cell holds, so a pointer into it (&w.c.Name, a reflect
			// pointer into that memory) sees it. A cell that AOT code
			// shadows keeps its value in the shadow: replaced, as before.
			if src, ok := v.R.(reflect.Value); ok && src.CanInterface() && src.Type() == dst.Type() &&
				(ownedShadows.Load() == 0 || ownShadow(p) == nil) {
				setNative(dst, src) // the checks of a running server (lateNilServer)
				return
			}
		}
		elem = func(i int) types.Type { return u.Field(i).Type() }
	case *types.Array:
		if dst, ok := p.R.([]Value); ok {
			if src, ok := v.R.([]Value); ok && len(src) == len(dst) && !needsCopy(u.Elem()) {
				if wordKind(u.Elem()) {
					copyWords(dst, src)
				} else {
					copy(dst, src)
				}
				return
			}
		}
		elem = func(int) types.Type { return u.Elem() }
	default:
		if wordKind(t) {
			// a field or an element of a struct or array copied as a
			// whole: a racy reader of it uses loadWord (wordcell.go)
			storeWord(p, deepCopy(v, t))
		} else {
			*p = deepCopy(v, t)
		}
		return
	}
	dst, ok := p.R.([]Value)
	src, ok2 := v.R.([]Value)
	if !ok || !ok2 || len(dst) != len(src) || len(dst) == 0 {
		*p = deepCopy(v, t)
		return
	}
	for i := range dst {
		assign(&dst[i], src[i], elem(i))
	}
}
