package vm

import (
	"errors"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math/bits"
	"net/http"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// Run-time errors raised by the VM. They copy the errors of the gc runtime:
// same messages, and they implement runtime.Error (a program may recover
// them and assert v.(runtime.Error)).

// runtimeError returns a runtime.errorString with the message msg: the
// panic value of gc for "comparing uncomparable type T" and the like. A
// program, or native code that recovers it (slog, errors.Is), sees %T
// runtime.errorString, not a type of the VM.
func runtimeError(msg string) error {
	v := reflect.New(errorStringType).Elem()
	v.SetString(msg)
	return v.Interface().(error)
}

// errorStringType is runtime.errorString, taken from a real panic of the
// host.
var errorStringType = reflect.TypeOf(hostRuntimePanic(func() { _ = make([]int, negShift) }))

var errNilDeref = runtimeError("invalid memory address or nil pointer dereference")

// Errors of the gc runtime itself, taken from a real panic of the host: a
// program that recovers them sees the type of gc (%T is
// runtime.plainError or runtime.errorString), not a type of the VM.
var (
	errNegShift     = hostRuntimePanic(func() { _ = 1 << negShift })
	errNilMapAssign = hostRuntimePanic(func() { nilMap[0] = 0 })
	errCloseNilChan = hostRuntimePanic(func() { close(nilChan) })
)

var (
	negShift = -1
	nilMap   map[int]int
	nilChan  chan struct{}
)

// hostRuntimePanic runs f, which makes the gc runtime panic, and returns the
// panic value.
func hostRuntimePanic(f func()) (err error) {
	defer func() { err = recover().(error) }()
	f()
	panic("hostRuntimePanic: no panic")
}

// plainError is a run-time error whose message has no "runtime error: "
// prefix, like "go of nil func value".
type plainError string

func (e plainError) Error() string { return string(e) }
func (plainError) RuntimeError()   {}

// typeAssertError is the error of a failed x.(T) (runtime.TypeAssertionError).
type typeAssertError struct {
	iface    string // static type of x
	concrete string // dynamic type of x, "" if x is nil
	asserted string
	missing  string // first missing method, when T is an interface
	otherPkg bool   // concrete and asserted have the same name, in different packages
}

func (*typeAssertError) RuntimeError() {}

func (e *typeAssertError) Error() string {
	if e.concrete == "" {
		return "interface conversion: " + e.iface + " is nil, not " + e.asserted
	}
	if e.missing == "" {
		msg := "interface conversion: " + e.iface + " is " + e.concrete + ", not " + e.asserted
		if e.concrete == e.asserted {
			if e.otherPkg {
				msg += " (types from different packages)"
			} else {
				msg += " (types from different scopes)"
			}
		}
		return msg
	}
	return "interface conversion: " + e.concrete + " is not " + e.asserted + ": missing method " + e.missing
}

// assertError returns the error of the failed assertion s on iv.
func (m *Machine) assertError(s *assertSite, iv *Iface) error {
	e := &typeAssertError{iface: typeString(s.x), asserted: typeString(s.t)}
	if iv == nil {
		if s.iface != nil {
			// gc has no static type here (runtime.assertE2I).
			e.iface = "interface"
		}
		return m.nativeAssertError(s, nil, e)
	}
	var pkg string
	if iv.T.T != nil {
		e.concrete, pkg = typeString(iv.T.T), typePkgPath(iv.T.T)
	} else {
		e.concrete, pkg = iv.T.Native.String(), nativePkgPath(iv.T.Native)
	}
	e.otherPkg = pkg != typePkgPath(s.t)
	if s.iface != nil {
		if iv.T.T != nil {
			if f, _ := types.MissingMethod(iv.T.T, s.iface, true); f != nil {
				e.missing = f.Name()
			}
		} else {
			for i := 0; i < s.iface.NumMethods(); i++ {
				if _, ok := iv.T.Native.MethodByName(s.iface.Method(i).Name()); !ok {
					e.missing = s.iface.Method(i).Name()
					break
				}
			}
		}
	}
	return m.nativeAssertError(s, iv, e)
}

// nativeAssertError returns the *runtime.TypeAssertionError of gc for e
// when its types are native types with the names that gc prints, else e: a
// program that recovers it may assert it to *runtime.TypeAssertionError
// (errors.As). Go cannot make the native types of the program, so a failed
// assertion that names one keeps the error of the VM.
func (m *Machine) nativeAssertError(s *assertSite, iv *Iface, e *typeAssertError) error {
	if !hostAssertOK {
		return e
	}
	var l typeAssertionLayout
	if s.iface == nil {
		// gc has the static type only for an assertion to a concrete type
		ifc, ok := m.sameNativeType(s.x)
		if !ok {
			return e
		}
		l.iface = rtypePtr(ifc)
	}
	at, ok := m.sameNativeType(s.t)
	if !ok {
		return e
	}
	l.asserted = rtypePtr(at)
	if iv != nil {
		concrete := iv.T.Native
		if iv.T.T != nil {
			if concrete, ok = m.sameNativeType(iv.T.T); !ok {
				return e
			}
		}
		l.concrete = rtypePtr(concrete)
	}
	l.missing = e.missing
	r := new(runtime.TypeAssertionError)
	*(*typeAssertionLayout)(unsafe.Pointer(r)) = l
	if r.Error() != e.Error() {
		return e // the message of the VM is the one of go run
	}
	return r
}

// sameNativeType returns the native type of t when it has the name that gc
// prints for t (int, []string, interface {}, time.Duration): a type of the
// program has no native type of its name.
func (m *Machine) sameNativeType(t types.Type) (rt reflect.Type, ok bool) {
	defer func() {
		if recover() != nil {
			rt, ok = nil, false
		}
	}()
	rt = m.reflectType(t)
	return rt, rt != nil && rt.String() == typeString(t)
}

// typeAssertionLayout mirrors runtime.TypeAssertionError: the *_type of the
// static interface, of the dynamic type and of the asserted type, and the
// missing method.
type typeAssertionLayout struct {
	iface, concrete, asserted unsafe.Pointer
	missing                   string
}

// hostAssertOK reports whether runtime.TypeAssertionError has the layout of
// typeAssertionLayout: checked on a real failed assertion of the host.
var hostAssertOK = func() bool {
	e, ok := hostRuntimePanic(func() { _ = hostAny.(string) }).(*runtime.TypeAssertionError)
	if !ok || reflect.TypeFor[runtime.TypeAssertionError]().Size() != reflect.TypeFor[typeAssertionLayout]().Size() {
		return false
	}
	l := (*typeAssertionLayout)(unsafe.Pointer(e))
	return l.iface == rtypePtr(reflect.TypeFor[any]()) && l.concrete == rtypePtr(reflect.TypeFor[int]()) &&
		l.asserted == rtypePtr(reflect.TypeFor[string]()) && l.missing == ""
}()

var hostAny any = 1

// rtypePtr returns the *_type of the gc runtime for t (nil for nil).
func rtypePtr(t reflect.Type) unsafe.Pointer {
	if t == nil {
		return nil
	}
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1]
}

// isMethodValueNilCheck reports whether v is the nil check that go/ssa
// emits for the method value i.M of an interface i: v.(I) with I the type
// of v, or the embedded interface that declares M (rc.(io.Reader) for
// rc.Read of an io.ReadCloser), unused, followed by the closure of the
// bound method (or any(v).(any) with no position, for a type parameter).
func isMethodValueNilCheck(v *ssa.TypeAssert) bool {
	if v.CommaOk || !types.IsInterface(v.AssertedType) || !types.IsInterface(v.X.Type()) {
		return false
	}
	if it, ok := v.AssertedType.Underlying().(*types.Interface); !ok || !types.Implements(v.X.Type(), it) {
		return false
	}
	if refs := v.Referrers(); refs != nil && len(*refs) > 0 {
		return false
	}
	if !v.Pos().IsValid() {
		return true
	}
	// the next instruction binds v.X in the closure of the bound method
	// (I.M$bound)
	instrs := v.Block().Instrs
	for i, in := range instrs[:len(instrs)-1] {
		if in != v {
			continue
		}
		// with an embedded interface, v.X is converted to I first
		bound := v.X
		next := instrs[i+1]
		if ci, ok := next.(*ssa.ChangeInterface); ok && ci.X == v.X && i+2 < len(instrs) {
			bound, next = ci, instrs[i+2]
		}
		mc, ok := next.(*ssa.MakeClosure)
		if !ok {
			return false
		}
		f, _ := mc.Fn.(*ssa.Function)
		return f != nil && f.Synthetic != "" && strings.HasSuffix(f.Name(), "$bound") && len(mc.Bindings) == 1 && mc.Bindings[0] == bound
	}
	return false
}

// typePkgPath returns the package path of the type t as the gc runtime
// gives it for a type assertion error: the package of the named type t, or
// of *t; for a struct type without a name, the package of its first
// unexported field (gc keeps it in the struct type, so that struct{ x int }
// of two packages are two types, GOROOT test fixedbugs/issue18911.go); ""
// for other types, a pointer to a struct type without a name too (gc has
// no package in the pointer type: "types from different scopes").
func typePkgPath(t types.Type) string {
	t = types.Unalias(t)
	ptr := false
	if p, ok := t.(*types.Pointer); ok {
		t, ptr = types.Unalias(p.Elem()), true
	}
	switch t := t.(type) {
	case *types.Named:
		if t.Obj().Pkg() != nil {
			return t.Obj().Pkg().Path()
		}
	case *types.Struct:
		if ptr {
			break
		}
		for i := 0; i < t.NumFields(); i++ {
			if f := t.Field(i); !f.Exported() && f.Pkg() != nil {
				return f.Pkg().Path()
			}
		}
	}
	return ""
}

// nativePkgPath is typePkgPath for a native type.
func nativePkgPath(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return t.Elem().PkgPath() // "" for a struct type without a name
	}
	if t.Name() == "" && t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			if f := t.Field(i); !f.IsExported() {
				return f.PkgPath
			}
		}
	}
	return t.PkgPath()
}

// goexit is the value of the Go panic that runs runtime.Goexit in the VM:
// it unwinds the interpreted frames and runs their deferred calls, but
// recover() does not see it and cannot stop it. The goroutine ends when the
// unwinding reaches its first frame (see spawn, RunMain and Machine.Call).
type goexit struct{}

func (goexit) Error() string  { return "runtime.Goexit called" }
func (goexit) String() string { return "runtime.Goexit called" }

var errGoexit any = goexit{}

// ErrGoexit is returned by RunMain when the main goroutine of the program
// calls runtime.Goexit, and by the Plugin methods when the plugin code
// they run calls it. The deferred calls have run. With gc, the program goes
// on until its other goroutines end; cmd/yaegi does this.
var ErrGoexit = errors.New("vm: runtime.Goexit called")

func vmGoexit() { panic(errGoexit) }

func init() {
	vmHooks["runtime.Goexit"] = func(*Machine, []Value) Value { vmGoexit(); return Value{} }
}

// goexitValue is the func value f := runtime.Goexit. Native code may run it
// on a goroutine with no VM frame (time.AfterFunc(0, f)): there only the
// real Goexit ends the goroutine quietly. On a goroutine that runs VM code
// (sync.Once.Do(f) called by the VM), it is the VM Goexit, so the
// interpreted deferred calls run and RunMain or the Plugin methods return.
func goexitValue() {
	if onVMStack() {
		vmGoexit()
	}
	runtime.Goexit()
}

// goexitPtr is the code pointer of goexitValue. When the VM calls the value
// itself (callNativeValue), it uses the VM Goexit at once.
var goexitPtr uintptr

// protectedName is the name of the function that runs the VM code of a ctx
// (ctx.protected): a frame of it on the stack means an active ctx.
var protectedName string

func init() { // not in the var declarations: initialization cycle
	goexitPtr = reflect.ValueOf(goexitValue).Pointer()
	protectedName = runtime.FuncForPC(reflect.ValueOf((*ctx).protected).Pointer()).Name()
}

// onVMStack reports whether the calling goroutine runs VM code below the
// caller: its stack has a ctx.protected frame. It walks the stack, so it is
// only used on the rare paths (a Goexit through native code).
func onVMStack() bool {
	pcs := make([]uintptr, 64)
	for skip := 2; ; skip += len(pcs) {
		n := runtime.Callers(skip, pcs)
		frames := runtime.CallersFrames(pcs[:n])
		for {
			f, more := frames.Next()
			if f.Function == protectedName {
				return true
			}
			if !more {
				break
			}
		}
		if n < len(pcs) {
			return false
		}
	}
}

// panicWrapError returns the error raised when the value method name is
// called through a nil pointer of type ptr (runtime.panicwrap).
func panicWrapError(ptr types.Type, name ssa.Value) error {
	meth := ""
	if k, ok := name.(*ssa.Const); ok && k.Value != nil && k.Value.Kind() == constant.String {
		meth = constant.StringVal(k.Value)
	}
	typ, pkg := typeString(ptr), ""
	if p, ok := ptr.Underlying().(*types.Pointer); ok {
		typ = typeString(p.Elem())
		if n, ok := types.Unalias(p.Elem()).(*types.Named); ok {
			typ = n.Obj().Name()
			if n.TypeArgs().Len() > 0 {
				typ += "[...]" // as gc
			}
			if tp := n.Obj().Pkg(); tp != nil {
				pkg = tp.Path()
				if tp.Name() == "main" {
					pkg = "main"
				}
			}
		}
	}
	return plainError("value method " + pkg + "." + typ + "." + meth + " called using nil *" + typ + " pointer")
}

// isNilPtr reports whether r is a nil pointer.
func isNilPtr(r any) bool {
	switch p := r.(type) {
	case nil:
		return true
	case *Value:
		return p == nil
	case reflect.Value:
		return p.Kind() == reflect.Pointer && p.IsNil()
	}
	return false
}

// unevaluatedRange reports whether the load v is the range expression of a
// "for range *p" or "for i := range *p" over an array whose length is
// constant. The spec says that such an expression is not evaluated (so a nil
// p does not panic), but ssa always loads it; the load has no use.
func (c *compiler) unevaluatedRange(v *ssa.UnOp) bool {
	for _, r := range *v.Referrers() {
		if _, ok := r.(*ssa.DebugRef); !ok {
			return false
		}
	}
	if _, ok := v.Type().Underlying().(*types.Array); !ok || !v.Pos().IsValid() {
		return false
	}
	syn := c.f.ssa.Syntax()
	if syn == nil {
		return false
	}
	found := false
	ast.Inspect(syn, func(n ast.Node) bool {
		rs, ok := n.(*ast.RangeStmt)
		if found || !ok || rs.Value != nil && !isBlank(rs.Value) {
			return !found
		}
		// ssa gives the load of the range expression the position of
		// its star, selector, bracket or identifier (go/ssa builder.go).
		x := ast.Unparen(rs.X)
		var pos token.Pos
		switch x := x.(type) {
		case *ast.StarExpr:
			pos = x.Star
		case *ast.SelectorExpr:
			pos = x.Sel.Pos()
		case *ast.IndexExpr:
			pos = x.Lbrack
		case *ast.Ident:
			pos = x.Pos()
		}
		if pos != v.Pos() {
			return true
		}
		// len(x) is constant only if x has no (non-constant) function
		// calls and no receives. A conversion like (*[3]int)(q) or a
		// constant builtin call is not a function call.
		found = true
		ast.Inspect(x, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if c.isCall(n) {
					found = false
				}
			case *ast.UnaryExpr:
				if n.Op == token.ARROW {
					found = false
				}
			}
			return found
		})
		return false
	})
	return found
}

// isCall reports whether the call expression e of the function being
// compiled is a function call, not a conversion or a constant builtin call:
// ssa builds a Call instruction (whose position is e.Lparen) only for the
// real calls. Without go/types information, this is how we tell them apart.
func (c *compiler) isCall(e *ast.CallExpr) bool {
	for _, b := range c.f.ssa.Blocks {
		for _, ins := range b.Instrs {
			if call, ok := ins.(*ssa.Call); ok && call.Pos() == e.Lparen {
				return true
			}
		}
	}
	return false
}

func isBlank(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}

// Kinds of bounds errors, as in the gc runtime (runtime/error.go).
const (
	boundsIndex uint8 = iota
	boundsSliceAlen
	boundsSliceAcap
	boundsSliceB
	boundsSlice3Alen
	boundsSlice3Acap
	boundsSlice3B
	boundsSlice3C
)

var boundsErrorFmt = [...]string{
	boundsIndex:      "index out of range [%x] with length %y",
	boundsSliceAlen:  "slice bounds out of range [:%x] with length %y",
	boundsSliceAcap:  "slice bounds out of range [:%x] with capacity %y",
	boundsSliceB:     "slice bounds out of range [%x:%y]",
	boundsSlice3Alen: "slice bounds out of range [::%x] with length %y",
	boundsSlice3Acap: "slice bounds out of range [::%x] with capacity %y",
	boundsSlice3B:    "slice bounds out of range [:%x:%y]",
	boundsSlice3C:    "slice bounds out of range [%x:%y:]",
}

// boundsNegErrorFmt are the formats used when x is negative (y is not shown).
var boundsNegErrorFmt = [...]string{
	boundsIndex:      "index out of range [%x]",
	boundsSliceAlen:  "slice bounds out of range [:%x]",
	boundsSliceAcap:  "slice bounds out of range [:%x]",
	boundsSliceB:     "slice bounds out of range [%x:]",
	boundsSlice3Alen: "slice bounds out of range [::%x]",
	boundsSlice3Acap: "slice bounds out of range [::%x]",
	boundsSlice3B:    "slice bounds out of range [:%x:]",
	boundsSlice3C:    "slice bounds out of range [%x::]",
}

// boundsError is an index or slice bounds error (runtime.boundsError).
type boundsError struct {
	x      int64
	y      int
	signed bool // x has a signed type
	code   uint8
}

func (boundsError) RuntimeError() {}

func (e boundsError) Error() string {
	f := boundsErrorFmt[e.code]
	if e.signed && e.x < 0 {
		f = boundsNegErrorFmt[e.code]
	}
	x := strconv.FormatUint(uint64(e.x), 10)
	if e.signed {
		x = strconv.FormatInt(e.x, 10)
	}
	b := []byte("runtime error: ")
	for i := 0; i < len(f); i++ {
		if f[i] == '%' && i+1 < len(f) {
			switch f[i+1] {
			case 'x':
				b = append(b, x...)
				i++
				continue
			case 'y':
				b = strconv.AppendInt(b, int64(e.y), 10)
				i++
				continue
			}
		}
		b = append(b, f[i])
	}
	return string(b)
}

// Flags of the index instructions (Instr.F).
const (
	idxPtr      = 1 // the indexed value is a pointer to an array
	idxUnsigned = 2 // the index has an unsigned type
)

// Flag of the shift instructions (Instr.F).
const shiftSigned = 1 // the count has a signed type: a negative count panics

// isUnsigned reports whether t is an unsigned integer type.
func isUnsigned(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsUnsigned != 0
}

func indexFlags(x, index types.Type) uint8 {
	var f uint8
	if _, ok := x.Underlying().(*types.Pointer); ok {
		f |= idxPtr
	}
	if isUnsigned(index) {
		f |= idxUnsigned
	}
	return f
}

// indexError returns the error of index i on a value of length n.
func indexError(i int64, n int, flags uint8) error {
	return boundsError{x: i, y: n, signed: flags&idxUnsigned == 0, code: boundsIndex}
}

// checkIndex panics if i is out of range for a value of length n.
func checkIndex(i int64, n int, flags uint8) {
	if uint64(i) >= uint64(n) {
		panic(indexError(i, n, flags))
	}
}

// sliceBounds checks the bounds of s[lo:hi:max] like the gc runtime (same
// order of checks, same errors) and returns them. n and c are the length and
// the capacity of the sliced value.
func (s *sliceSite) bounds(regs []Value, n, c int) (lo, hi, mx int) {
	lo, hi, mx = 0, n, c
	if s.max >= 0 {
		k := regs[s.max].N
		code := boundsSlice3Acap
		if s.kind != sliceOfSlice {
			code = boundsSlice3Alen
		}
		if uint64(k) > uint64(c) {
			panic(boundsError{x: k, y: c, signed: !s.maxU, code: code})
		}
		mx = int(k)
	}
	if s.hi >= 0 {
		j := regs[s.hi].N
		if s.max >= 0 {
			if uint64(j) > uint64(mx) {
				panic(boundsError{x: j, y: mx, signed: !s.hiU, code: boundsSlice3B})
			}
		} else {
			code := boundsSliceAcap
			if s.kind != sliceOfSlice {
				code = boundsSliceAlen
			}
			lim := c
			if s.kind == sliceOfString {
				lim = n
			}
			if uint64(j) > uint64(lim) {
				if s.makeslice {
					makeSliceCheck(j, int64(lim), s.esize)
				}
				panic(boundsError{x: j, y: lim, signed: !s.hiU, code: code})
			}
		}
		hi = int(j) // s[lo::max] is not valid Go: hi is always present with max
	}
	if s.lo >= 0 {
		i := regs[s.lo].N
		if uint64(i) > uint64(hi) {
			code := boundsSliceB
			if s.max >= 0 {
				code = boundsSlice3C
			}
			panic(boundsError{x: i, y: hi, signed: !s.loU, code: code})
		}
		lo = int(i)
	}
	return lo, hi, mx
}

// maxAlloc is the largest allocation of the gc runtime (runtime.maxAlloc on
// linux/amd64 and most 64-bit systems; 32-bit systems use 1<<32 - 1).
const maxAlloc = (1<<48)*(strconv.IntSize/64) + (1<<32-1)*(1-strconv.IntSize/64)

// tooBig reports whether n elements of esize bytes are more than maxAlloc.
func tooBig(n int64, esize uint64) bool {
	hi, lo := bits.Mul64(uint64(n), esize)
	return hi != 0 || lo > maxAlloc
}

// makeSliceCheck panics like the gc runtime when make([]T, n, c) has an
// invalid length or capacity: gc compares n and c times the size of T
// (esize) to maxAlloc. The VM also refuses a slice whose own memory (one
// Value per element) is more than maxAlloc. A type of size 0 is not refused,
// like with gc: a large slice of it is a zslice, which uses no memory per
// element, and a small one fits in memory. On a 32-bit platform, a length
// or a capacity of type int64 past the int range is refused too, as gc's
// makeslice64 does, before int(n) cuts it.
func makeSliceCheck(n, c int64, esize uint64) {
	vsize := uint64(valueSize) // a Value is the element of the slices the VM makes
	if esize == 0 {
		vsize = 0
	}
	if n < 0 || int64(int(n)) != n || tooBig(n, esize) || tooBig(n, vsize) {
		panic(runtimeError("makeslice: len out of range"))
	}
	if c < n || int64(int(c)) != c || tooBig(c, esize) || tooBig(c, vsize) {
		panic(runtimeError("makeslice: cap out of range"))
	}
}

// newPanic returns the Go panic value of panic(v) in interpreted code. Since
// Go 1.21, panic(nil) panics with a *runtime.PanicNilError, so that
// recover() does not return nil. GODEBUG=panicnil=1 keeps the old behavior.
// The host runtime decides: the VM runs a real panic(nil) and takes what
// recover gives (it reads GODEBUG, also after an os.Setenv).
func (m *Machine) newPanic(v Value) *VMPanic {
	if iv, ok := v.R.(*Iface); v.R == nil || ok && iv == nil {
		if r := hostNilPanic(); r != nil {
			v = m.fromNative(reflect.ValueOf(&r).Elem(), emptyIface)
		}
	}
	return &VMPanic{V: v}
}

// hostNilPanic returns what recover gives after panic(nil) in the host:
// a *runtime.PanicNilError, or nil with GODEBUG=panicnil=1.
func hostNilPanic() (r any) {
	defer func() { r = recover() }()
	panic(nil)
}

// hostSentinels are native panic values that host code compares by
// identity: net/http stops the response without a log when a handler
// panics with http.ErrAbortHandler (r != ErrAbortHandler in conn.serve).
var hostSentinels = []any{http.ErrAbortHandler}

// nativePanic returns the value that a panic r of interpreted code has when
// it leaves a callback for the native code that called it (a String method
// called by fmt, a func of a template): the value itself when it is of a
// native type or of a basic type without a name, as in gc, where fmt
// recovers "boom" from panic("boom") and prints %!v(PANIC=String method:
// boom). A *VMPanic printed "panic: boom" there. A value of a type of the
// program is boxed as for a call of an AOT copy, so that fmt sees its
// methods (Error, String). The goroutine keeps the *VMPanic (gstate.plain):
// when the same value comes back into the VM through the native code,
// restorePanic gives the *VMPanic back, so that recover, the host and its
// GoPanic see what they saw before.
func (m *Machine) nativePanic(r any) any {
	p, ok := r.(*VMPanic)
	if !ok {
		return r
	}
	v := m.plainPanicValue(p)
	if v == nil {
		return r
	}
	if gs := lookupG(getg()); gs != nil {
		gs.plain = plainPanic{val: v, orig: p}
	}
	return v
}

// plainPanicValue is the value of nativePanic for p, or nil to keep p.
func (m *Machine) plainPanicValue(p *VMPanic) any {
	iv, _ := p.V.R.(*Iface)
	if iv == nil || iv.T == nil {
		return nil
	}
	if iv.T.T == nil {
		rv, _ := iv.V.R.(reflect.Value)
		if rv.IsValid() && rv.Kind() == reflect.Interface {
			rv = rv.Elem()
		}
		if !rv.IsValid() || !rv.CanInterface() {
			return nil
		}
		return rv.Interface()
	}
	if _, basic := iv.T.T.(*types.Basic); basic {
		return m.toNativeW(iv.V, iv.T.T, newConv(nil)).Interface()
	}
	return m.routeArg(p.V, newConv(nil))
}

// plainPanic is the last panic value that nativePanic gave to native code on
// a goroutine, and the *VMPanic it stands for.
type plainPanic struct {
	val  any
	orig *VMPanic
}

// restorePanic returns the *VMPanic that nativePanic turned into r, when
// the panic r that native code raised into the VM is that value (native
// code that did not recover it, or raised it again), else r.
func restorePanic(r any) any {
	if _, ok := r.(*VMPanic); ok {
		return r
	}
	gs := lookupG(getg())
	if gs == nil || gs.plain.orig == nil {
		return r
	}
	pl := gs.plain
	gs.plain = plainPanic{}
	if sameValue(r, pl.val) {
		return pl.orig
	}
	return r
}

// hostPanic returns the value that a panic r of interpreted code has when
// it leaves the VM for the Go code that called it: the native sentinel
// when r is panic(s) of one of hostSentinels, else r (a *VMPanic stays a
// *VMPanic). If the panic comes back into the VM, recover gives the same
// value as before (panicValue).
func hostPanic(r any) any {
	p, ok := r.(*VMPanic)
	if !ok {
		return r
	}
	var rv reflect.Value
	switch x := p.V.R.(type) {
	case *Iface:
		if x == nil || x.T == nil || x.T.T != nil {
			return r // nil, or an interpreted dynamic type
		}
		rv, _ = x.V.R.(reflect.Value)
	case reflect.Value:
		rv = x
	}
	if rv.IsValid() && rv.Kind() == reflect.Interface {
		rv = rv.Elem()
	}
	if !rv.IsValid() || !rv.CanInterface() {
		return r
	}
	v := rv.Interface()
	for _, s := range hostSentinels {
		if sameValue(v, s) {
			return s
		}
	}
	return r
}
