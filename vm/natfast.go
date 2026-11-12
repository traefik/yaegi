package vm

// Optimization 4: specialized native-call trampolines.
//
// The general path calls a native function through reflect.Value.Call with
// per-argument reflect conversions. A trampoline is a direct Go call for one
// signature that reads the VM values without reflect. zz_natsigs.go is
// generated (natsigs_gen.go) for every signature of the standard library made
// only of basic types; this file hand-writes the trampolines of a few other
// signatures that plugins call often (http.Error, io.ReadAll, fmt.Sprintf,
// methods of http.Header and sync). Any other signature takes the general
// path.

//go:generate go run natsigs_gen.go

import (
	"go/types"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type fastFn func(regs []Value, args []int32) Value

func vStr(v Value) string { s, _ := v.R.(string); return s }

// natAny returns the Go value of a native VM value (reflect.Value payload,
// or a native dynamic type inside an interface).
func natAny(v Value) any {
	switch r := v.R.(type) {
	case reflect.Value:
		return r.Interface()
	case *Iface:
		if r.T.Native != nil {
			if rv, ok := r.V.R.(reflect.Value); ok {
				return rv.Interface()
			}
		}
		if w, ok := r.V.R.(reflect.Value); ok {
			return w.Interface()
		}
	case nil:
		return nil
	}
	return v.R
}

func natVal(x any) Value { return Value{R: reflect.ValueOf(x)} }

// natIface converts a native value to an interpreted interface value.
func (m *Machine) natIface(x any) Value {
	if x == nil {
		return Value{}
	}
	rv := reflect.ValueOf(x)
	return Value{R: &Iface{T: m.nativeRType(rv.Type()), V: Value{R: rv}}}
}

func (m *Machine) errVal(err error) Value {
	if err == nil {
		return Value{}
	}
	return m.natIface(err)
}

// natOut converts the result x of type t of a generated trampoline as the
// general path does (a slice result gives a view of the native array).
func (m *Machine) natOut(x any, t types.Type) Value {
	return m.fromNative(reflect.ValueOf(x), t)
}

// natBytesOut is natOut for a []byte result, without the boxing of b in an
// any and in a reflect.Value: fromNative shares a []byte (optBytes).
func (m *Machine) natBytesOut(b []byte, t types.Type) Value {
	if b == nil {
		return Value{}
	}
	if optBytes {
		return Value{R: b}
	}
	return m.natOut(b, t)
}

// natErrOut is natOut for an error result.
func (m *Machine) natErrOut(err error, t types.Type) Value {
	if err == nil {
		return Value{}
	}
	return m.natErr(err, t)
}

// natErr is natErrOut for a non-nil error. It is apart so that the error
// of natErrOut does not go to the heap when it is nil (&err escapes).
func (m *Machine) natErr(err error, t types.Type) Value {
	return m.fromNative(reflect.ValueOf(&err).Elem(), t)
}

// natIfaceOut is natOut for a result x of the interface type T: fromNative
// gets it with its interface type, as from reflect.Value.Call.
func natIfaceOut[T any](m *Machine, x T, t types.Type) Value {
	if any(x) == nil {
		return Value{}
	}
	return natIfaceOutSlow(m, x, t)
}

// natIfaceOutSlow is natIfaceOut for a non-nil x, apart as natErr.
func natIfaceOutSlow[T any](m *Machine, x T, t types.Type) Value {
	return m.fromNative(reflect.ValueOf(&x).Elem(), t)
}

// natIfaceArg returns the value of the interface type T that v holds, when
// its dynamic type is a native type (or v is nil). ok is false otherwise:
// the general path makes the wrapper of an interpreted value, whose
// methods are the interpreted ones.
func natIfaceArg[T any](m *Machine, v Value) (x T, ok bool) {
	switch r := v.R.(type) {
	case nil:
		return x, true
	case *Iface:
		rv, isRV := r.V.R.(reflect.Value)
		if !isRV || !rv.CanInterface() || r.T.Native == nil && !m.nativeDynamic(r.T.T) {
			return x, false
		}
		x, ok = rv.Interface().(T)
	}
	return x, ok
}

// nativeDynamic reports whether t, the dynamic type of an interface value
// that holds a native value, is a native named type or a pointer to one:
// its methods are the native ones (an interpreted type has its own).
func (m *Machine) nativeDynamic(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	return m.isNativeNamed(t)
}

// natBytes returns the []byte that v holds, shared as on the general path
// (toNativeW): the VM form of a []byte, a native byte slice, or nil. ok is
// false for another form (a []Value).
func natBytes(v Value) ([]byte, bool) {
	switch r := v.R.(type) {
	case []byte:
		return r, true
	case nil:
		return nil, true
	case reflect.Value:
		if r.Kind() == reflect.Slice && r.Type().Elem().Kind() == reflect.Uint8 && r.CanInterface() {
			return r.Bytes(), true
		}
	}
	return nil, false
}

// natStrings returns the []string that v holds: a native slice is shared,
// the []Value of the VM is copied, and src is that []Value, for
// stringsBack. ok is false for another form.
func natStrings(v Value) (s []string, src []Value, ok bool) {
	switch r := v.R.(type) {
	case []Value:
		s = make([]string, len(r))
		for i, e := range r {
			if s[i], ok = e.R.(string); !ok && e.R != nil {
				return nil, nil, false
			}
		}
		return s, r, true
	case nil:
		return nil, nil, true
	case reflect.Value:
		if r.Kind() == reflect.Slice && r.Type().Elem() == reflect.TypeFor[string]() && r.CanInterface() {
			if s, ok = r.Interface().([]string); ok {
				return s, nil, true
			}
			// a named slice type (sort.StringSlice): the same array
			return r.Convert(reflect.TypeFor[[]string]()).Interface().([]string), nil, true
		}
	}
	return nil, nil, false
}

// stringsBack writes back into src the strings of s that native code
// changed (sort.Strings), as the write-back of the general path does.
func stringsBack(s []string, src []Value) {
	for i, e := range src {
		if old, _ := e.R.(string); old != s[i] {
			src[i] = Value{R: s[i]}
		}
	}
}

// genNativeInvoke calls the method of the invoke site s on the native
// value v (the payload of an interface) through its generated direct call
// (genFastInvoke), when there is one. ok is false when the general path
// must make the call. It is out of the loop of exec.
//
// It needs no flushShadows and no writeBack, unlike the general path: the
// generated call takes only native values (an argument that is not one
// takes the general path), so no interpreted memory is copied in or out.
//
//go:noinline
func (m *Machine) genNativeInvoke(s *invokeSite, v Value, regs []Value, a []int32) (Value, bool) {
	s.genOnce.Do(func() { s.gen = m.genInvoke(s) })
	if s.gen == nil {
		return Value{}, false
	}
	return m.natInvoke(s, v, regs, a)
}

// genInvoke returns the generated direct call (genFastInvoke) of the
// method of the invoke site s, or nil.
func (m *Machine) genInvoke(s *invokeSite) func(recv any, regs []Value, a []int32) (Value, bool) {
	return m.genFastInvoke(invokeKey(s.name, s.sig), tupleTypes(s.sig.Results()))
}

// natInvoke calls the method of the invoke site s on the native value v
// (the payload of an interface) through its generated direct call, when
// the method of the dynamic type is the real one: not a replacement of a
// restricted machine (refused, host or restricted methods), and not a
// method that the general path handles in a special way (channel readers,
// print and decode methods). The answer is kept for the first dynamic
// type of the site. ok is false when the general path must make the call.
func (m *Machine) natInvoke(s *invokeSite, v Value, regs []Value, a []int32) (Value, bool) {
	rv, ok := v.R.(reflect.Value)
	if !ok || !rv.IsValid() || !rv.CanInterface() {
		return Value{}, false
	}
	t := rv.Type()
	if h := s.natHit.Load(); h != nil && h.t == t {
		if !h.ok {
			return Value{}, false
		}
	} else {
		ok := m.natInvokeOK(t, s.name)
		if h == nil {
			s.natHit.Store(&natInvokeHit{t: t, ok: ok})
		}
		if !ok {
			return Value{}, false
		}
	}
	return s.gen(rv.Interface(), regs, a)
}

// natInvokeOK reports whether the method name of the native type t may be
// called directly (natInvoke).
func (m *Machine) natInvokeOK(t reflect.Type, name string) bool {
	k := methodKey{t, name}
	if readsChans(t, name) || printMethods[k] {
		return false
	}
	if _, ok := decodeDstMethods[k]; ok {
		return false
	}
	return !m.lookupMethod(t, name).replaced
}

// invokeKey is the key of a method in genFastInvoke: its name and its
// signature as Go source, written as natsigs_gen.go writes them.
func invokeKey(name string, sig *types.Signature) string {
	var ps, rs []string
	for i := 0; i < sig.Params().Len(); i++ {
		ps = append(ps, genTypeName(sig.Params().At(i).Type()))
	}
	for i := 0; i < sig.Results().Len(); i++ {
		rs = append(rs, genTypeName(sig.Results().At(i).Type()))
	}
	s := name + " func(" + strings.Join(ps, ", ") + ")"
	switch len(rs) {
	case 0:
	case 1:
		s += " " + rs[0]
	default:
		s += " (" + strings.Join(rs, ", ") + ")"
	}
	if sig.Variadic() {
		s += " ..." // never generated
	}
	return s
}

// genTypeName writes t as natsigs_gen.go does (typeName there).
func genTypeName(t types.Type) string {
	s := types.TypeString(t, func(p *types.Package) string {
		return strings.NewReplacer("/", "_", ".", "_").Replace(p.Path())
	})
	s = genByteRE.ReplaceAllString(s, "uint8")
	return genRuneRE.ReplaceAllString(s, "int32")
}

var (
	genByteRE = regexp.MustCompile(`\bbyte\b`)
	genRuneRE = regexp.MustCompile(`\brune\b`)
)

// fastSite returns a direct trampoline for a static native call site, or nil.
func (m *Machine) fastSite(s *nativeSite) fastFn {
	if s.method != "" {
		return m.fastMethodExpr(s)
	}
	fn := s.fn.Interface()
	if plainSite(s) {
		if f := m.genFastSite(s, fn); f != nil {
			return f
		}
	}
	switch f := fn.(type) {
	case func(http.ResponseWriter, string, int): // http.Error
		return func(regs []Value, a []int32) Value {
			rw, ok := natAny(regs[a[0]]).(http.ResponseWriter)
			if !ok && regs[a[0]].R != nil { // interpreted implementation
				return m.callNativeSlow(s, regs)
			}
			f(rw, vStr(regs[a[1]]), int(regs[a[2]].N))
			return Value{}
		}
	case func(string, ...any) string: // fmt.Sprintf
		return func(regs []Value, a []int32) Value {
			args, ok := basicAnys(regs[a[1]])
			if !ok {
				return m.callNativeSlow(s, regs)
			}
			return Value{R: f(vStr(regs[a[0]]), args...)}
		}
	case func(...any) string: // fmt.Sprint, fmt.Sprintln
		return func(regs []Value, a []int32) Value {
			args, ok := basicAnys(regs[a[0]])
			if !ok {
				return m.callNativeSlow(s, regs)
			}
			return Value{R: f(args...)}
		}
	case func(io.Reader) ([]byte, error): // io.ReadAll
		return func(regs []Value, a []int32) Value {
			r, ok := natAny(regs[a[0]]).(io.Reader)
			if !ok && regs[a[0]].R != nil { // interpreted implementation
				return m.callNativeSlow(s, regs)
			}
			b, err := f(r)
			return Value{R: []Value{BytesVal(b), m.errVal(err)}}
		}
	}
	return nil
}

// plainSite reports whether the general path does nothing special for the
// arguments of the call site s (a decoder destination, a printer, a value
// that a container stores): only then may a generated trampoline make the
// call.
func plainSite(s *nativeSite) bool {
	return s.dst.arg < 0 && !s.prints && s.stores < 0
}

// basicAnys returns the Go values of the variadic ...any argument v when
// each one is nil or has a predeclared basic dynamic type (int, string...).
// Such values have no methods for fmt to call, so they need none of the
// conversions of the general path. Otherwise it returns false.
func basicAnys(v Value) ([]any, bool) {
	s, ok := v.R.([]Value)
	if !ok && v.R != nil {
		return nil, false
	}
	out := make([]any, len(s))
	for i, e := range s {
		if e.R == nil {
			continue // nil interface
		}
		iv, ok := e.R.(*Iface)
		if !ok {
			return nil, false
		}
		if out[i], ok = basicAny(iv); !ok {
			return nil, false
		}
	}
	return out, true
}

// basicAny returns the Go value of iv when its dynamic type is a
// predeclared basic type held as a VM value.
func basicAny(iv *Iface) (any, bool) {
	b, ok := iv.T.T.(*types.Basic)
	if !ok {
		return nil, false
	}
	v := iv.V
	if b.Kind() == types.String {
		s, ok := v.R.(string)
		return s, ok || v.R == nil
	}
	if v.R != nil { // not the VM form of a number (a native payload)
		return nil, false
	}
	switch b.Kind() {
	case types.Bool:
		return v.N != 0, true
	case types.Int:
		return int(v.N), true
	case types.Int8:
		return int8(v.N), true
	case types.Int16:
		return int16(v.N), true
	case types.Int32:
		return int32(v.N), true
	case types.Int64:
		return v.N, true
	case types.Uint:
		return uint(v.N), true
	case types.Uint8:
		return uint8(v.N), true
	case types.Uint16:
		return uint16(v.N), true
	case types.Uint32:
		return uint32(v.N), true
	case types.Uint64:
		return uint64(v.N), true
	case types.Uintptr:
		return uintptr(v.N), true
	case types.Float32:
		return float32(f64(v)), true
	case types.Float64:
		return f64(v), true
	}
	return nil, false
}

// natRecv returns the native value of type T that v holds: a native value,
// or for a pointer type the address of a native value in a VM cell (a
// strings.Builder variable or field of interpreted code). ok is false for
// any other representation (an interpreted form of the type, a nil
// pointer): the caller then takes the general path, which converts it.
func natRecv[T any](v Value) (x T, ok bool) {
	switch r := v.R.(type) {
	case reflect.Value:
		if r.CanInterface() {
			x, ok = r.Interface().(T)
		}
	case *Value:
		if rt := reflect.TypeFor[T](); rt.Kind() == reflect.Pointer {
			x, ok = nativeCellPtr(v, rt.Elem()).(T)
		}
	}
	return x, ok
}

// fastMethodExpr handles static calls of methods of native named types.
func (m *Machine) fastMethodExpr(s *nativeSite) fastFn {
	rt := m.reflectType(s.params[0])
	if f := m.fastSyncMethod(s, rt); f != nil {
		return f
	}
	// The method that the general path calls (lookupMethod gives the
	// restricted or per-machine version, if any), with the receiver first.
	if mf := m.lookupMethod(rt, s.method); mf.fn.IsValid() && mf.fn.CanInterface() && plainSite(s) {
		if f := m.genFastSite(s, mf.fn.Interface()); f != nil {
			return f
		}
	}
	if rt == reflect.TypeOf(http.Header(nil)) {
		switch s.method {
		case "Get":
			return func(regs []Value, a []int32) Value {
				h, _ := natAny(regs[a[0]]).(http.Header)
				return Value{R: h.Get(vStr(regs[a[1]]))}
			}
		case "Set":
			return func(regs []Value, a []int32) Value {
				h, _ := natAny(regs[a[0]]).(http.Header)
				h.Set(vStr(regs[a[1]]), vStr(regs[a[2]]))
				return Value{}
			}
		case "Add":
			return func(regs []Value, a []int32) Value {
				h, _ := natAny(regs[a[0]]).(http.Header)
				h.Add(vStr(regs[a[1]]), vStr(regs[a[2]]))
				return Value{}
			}
		case "Del":
			return func(regs []Value, a []int32) Value {
				h, _ := natAny(regs[a[0]]).(http.Header)
				h.Del(vStr(regs[a[1]]))
				return Value{}
			}
		}
	}
	return nil
}

// nativeCellPtr returns the address of the native value that the
// interpreted pointer v points to (a sync.Mutex field of an interpreted
// struct lives in native memory, see toNativeW), or nil.
func nativeCellPtr(v Value, rt reflect.Type) any {
	if cell, ok := v.R.(*Value); ok {
		if rv, ok := cell.R.(reflect.Value); ok && rv.CanAddr() && rv.Type() == rt {
			return rv.Addr().Interface()
		}
	}
	return nil
}

// fastSyncMethod handles the methods without arguments of sync.Mutex,
// sync.RWMutex and sync.WaitGroup, called in loops by concurrent programs
// (GOROOT test fixedbugs/issue79186.go). Another receiver representation
// takes the general path. With PanicOnFatal, the unlocks are the ones of
// fatal.go.
func (m *Machine) fastSyncMethod(s *nativeSite, rt reflect.Type) fastFn {
	if rt.Kind() != reflect.Pointer {
		return nil
	}
	var call func(p any)
	switch rt.Elem() {
	case reflect.TypeFor[sync.Mutex]():
		switch s.method {
		case "Lock":
			call = func(p any) { p.(*sync.Mutex).Lock() }
		case "Unlock":
			call = func(p any) { p.(*sync.Mutex).Unlock() }
			if m.panicOnFatal {
				call = func(p any) { fatalUnlock(p.(*sync.Mutex)) }
			}
		}
	case reflect.TypeFor[sync.RWMutex]():
		switch s.method {
		case "Lock":
			call = func(p any) { p.(*sync.RWMutex).Lock() }
		case "Unlock":
			call = func(p any) { p.(*sync.RWMutex).Unlock() }
			if m.panicOnFatal {
				call = func(p any) { fatalRWUnlock(p.(*sync.RWMutex)) }
			}
		case "RLock":
			call = func(p any) { p.(*sync.RWMutex).RLock() }
		case "RUnlock":
			call = func(p any) { p.(*sync.RWMutex).RUnlock() }
			if m.panicOnFatal {
				call = func(p any) { fatalRUnlock(p.(*sync.RWMutex)) }
			}
		}
	case reflect.TypeFor[sync.WaitGroup]():
		switch s.method {
		case "Done":
			call = func(p any) { p.(*sync.WaitGroup).Done() }
		case "Wait":
			call = func(p any) { p.(*sync.WaitGroup).Wait() }
		}
	}
	if call == nil {
		return nil
	}
	elem := rt.Elem()
	return func(regs []Value, a []int32) Value {
		p := nativeCellPtr(regs[a[0]], elem)
		if p == nil {
			return m.callNativeSlow(s, regs)
		}
		call(p)
		return Value{}
	}
}

// fastInvoke returns a trampoline for a method call on a native value held
// in an interpreted interface, or nil. The receiver is checked at each call.
func (m *Machine) fastInvoke(name string) func(recv any, regs []Value, a []int32) (Value, bool) {
	switch name {
	case "Header":
		return func(recv any, regs []Value, a []int32) (Value, bool) {
			rw, ok := recv.(http.ResponseWriter)
			if !ok {
				return Value{}, false
			}
			return natVal(rw.Header()), true
		}
	case "WriteHeader":
		return func(recv any, regs []Value, a []int32) (Value, bool) {
			rw, ok := recv.(http.ResponseWriter)
			if !ok {
				return Value{}, false
			}
			rw.WriteHeader(int(regs[a[0]].N))
			return Value{}, true
		}
	case "Write":
		return func(recv any, regs []Value, a []int32) (Value, bool) {
			w, ok := recv.(io.Writer)
			if !ok {
				return Value{}, false
			}
			n, err := w.Write(valBytes(regs[a[0]]))
			return Value{R: []Value{{N: int64(n)}, m.errVal(err)}}, true
		}
	case "ServeHTTP":
		return func(recv any, regs []Value, a []int32) (Value, bool) {
			h, ok := recv.(http.Handler)
			if !ok {
				return Value{}, false
			}
			// An interpreted ResponseWriter (a plugin wrapping the
			// response) is not a native value: take the general path,
			// which converts it.
			rw, ok := natAny(regs[a[0]]).(http.ResponseWriter)
			if !ok {
				return Value{}, false
			}
			req, ok := natAny(regs[a[1]]).(*http.Request)
			if !ok {
				return Value{}, false
			}
			h.ServeHTTP(rw, req)
			return Value{}, true
		}
	case "Error":
		return func(recv any, regs []Value, a []int32) (Value, bool) {
			e, ok := recv.(error)
			if !ok {
				return Value{}, false
			}
			return Value{R: e.Error()}, true
		}
	}
	return nil
}

var _ = strconv.Itoa
