package vm

// Memory checks before native calls (Options.MaxMemory).
//
// The quota (memquota.go) counts the allocations of the VM instructions.
// Native code allocates without the VM: a plugin that calls
// strings.Repeat("x", 1<<40) asked the Go runtime for 1 TiB, and the
// runtime ended the host with a fatal "out of memory", which recover cannot
// stop. For the native functions whose result size is known from their
// arguments, the VM checks that size against the quota before the call
// (nativeMemSizes), and panics with ErrMemory instead: the plugin can
// recover it, and the host goes on. When the result is a string or a
// []byte, it is then counted as a live object of the plugin, like a buffer
// that make returned.
//
// io.ReadAll has no size in its arguments: it reads until the end. With a
// quota, the VM runs its own io.ReadAll (memReadAll), which checks the quota
// before each growth of its buffer, so an endless reader stops with
// ErrMemory at the quota instead of growing until the host dies.
// io/ioutil.ReadAll gets the same hook: it calls io.ReadAll natively.
//
// The check runs for each way a plugin calls these functions: a static
// call (compiler.call), a function value (var rep = strings.Repeat:
// nativeMemValue), a go or defer statement (callDesc), and a method called
// through an interface on a native receiver (OpInvoke, invokeSite.memCheck).
// The other calls pay nothing: the sites are chosen at compile time.
//
// What is still not checked: a call made by native code (a native function
// that calls strings.Repeat itself), reflect's Method or MethodByName, and
// the other native functions that allocate from their arguments or from
// what they read (encoding/json of a huge input, io.Copy into a bytes.Buffer, fmt.Sprintf("%*d", 1<<40, 0)...). A memory
// limit of the process (GOMEMLIMIT, a cgroup) stays the real bound.

import (
	"bytes"
	"io"
	"math"
	"reflect"
	"strings"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// nativeMemSize returns the bytes that a native call with the VM arguments a
// will allocate (0: nothing to check). For a method, a[0] is the receiver.
type nativeMemSize func(m *Machine, a []Value) int64

// nativeMemSizes are the native functions (keyed as RegisterNative names
// them) whose result size the VM checks before the call. The size counts
// what the call allocates at least: a too large request panics in the
// plugin; an overflow of the size is left to the native function, which
// panics itself.
var nativeMemSizes = map[string]nativeMemSize{}

// nativeMemHooks replace native functions for a machine with a quota.
var nativeMemHooks = map[string]NativeHook{}

func init() {
	nativeMemSizes["strings.Repeat"] = func(_ *Machine, a []Value) int64 {
		return repeatSize(int64(len(valStr(a[0]))), a[1].N)
	}
	nativeMemSizes["bytes.Repeat"] = func(_ *Machine, a []Value) int64 {
		return repeatSize(int64(valLen(a[0])), a[1].N)
	}
	// Grow allocates only when the free capacity is too small. The size is
	// a lower bound: Go then allocates about 2*cap+n, not n-free.
	nativeMemSizes["(*strings.Builder).Grow"] = func(_ *Machine, a []Value) int64 {
		if b := nativeRecv[strings.Builder](a[0]); b != nil {
			return a[1].N - int64(b.Cap()-b.Len())
		}
		return a[1].N
	}
	nativeMemSizes["(*bytes.Buffer).Grow"] = func(_ *Machine, a []Value) int64 {
		if b := nativeRecv[bytes.Buffer](a[0]); b != nil {
			return a[1].N - int64(b.Cap()-b.Len())
		}
		return a[1].N
	}
	nativeMemMethods[methodKey{reflect.TypeFor[*strings.Builder](), "Grow"}] = nativeMemSizes["(*strings.Builder).Grow"]
	nativeMemMethods[methodKey{reflect.TypeFor[*bytes.Buffer](), "Grow"}] = nativeMemSizes["(*bytes.Buffer).Grow"]
	for k := range nativeMemMethods {
		nativeMemMethodNames[k.name] = true
	}
	// The native reflect.MakeSlice (a program that keeps native reflect).
	// With the VM reflect, vmReflect.makeSlice checks the quota itself.
	nativeMemSizes["reflect.MakeSlice"] = func(m *Machine, a []Value) int64 {
		t := Arg[reflect.Type](m, a[0])
		if t == nil || t.Kind() != reflect.Slice || a[2].N < 0 {
			return 0
		}
		return satMulBytes(a[2].N, int64(t.Elem().Size()))
	}
	nativeMemHooks["io.ReadAll"] = func(m *Machine, a []Value) Value {
		b, err := m.memReadAll(Arg[io.Reader](m, a[0]))
		return Tuple(BytesVal(b), m.ErrorVal(err))
	}
	// io/ioutil.ReadAll calls io.ReadAll natively: the hook of io.ReadAll
	// would not run.
	nativeMemHooks["io/ioutil.ReadAll"] = nativeMemHooks["io.ReadAll"]
}

// nativeMemMethods are the methods of nativeMemSizes by native receiver
// type, for a call through an interface (OpInvoke), and
// nativeMemMethodNames their names: the compiler marks an invoke site of
// such a name (invokeSite.memCheck), so other sites check nothing.
var (
	nativeMemMethods     = map[methodKey]nativeMemSize{}
	nativeMemMethodNames = map[string]bool{}
)

// nativeRecv returns the native *T that the VM receiver v holds (a
// variable of type T in its cell, or a native *T), or nil.
func nativeRecv[T any](v Value) *T {
	switch r := v.R.(type) {
	case *Value:
		if r != nil {
			return nativeRecv[T](*r)
		}
	case reflect.Value:
		if !r.IsValid() || !r.CanInterface() {
			return nil
		}
		if p, ok := r.Interface().(*T); ok {
			return p
		}
		if r.Type() == reflect.TypeFor[T]() && r.CanAddr() {
			return r.Addr().Interface().(*T)
		}
	}
	return nil
}

// nativeMemGuard runs the size check of a native call with the arguments
// a: a size that does not fit panics with ErrMemory.
func (m *Machine) nativeMemGuard(size nativeMemSize, a []Value) {
	if n := size(m, a); n >= memMin {
		m.memCheck(n)
	}
}

// nativeMemValue returns the function value of the native function f for a
// machine with a quota, when its calls are checked (nativeMemHook,
// nativeMemCheck): a call through the value (var rep = strings.Repeat)
// runs the same check as a static call. ok is false for the other
// functions.
func (m *Machine) nativeMemValue(f *ssa.Function) (v Value, ok bool) {
	h := m.nativeMemHook(f)
	if h == nil {
		h = m.nativeMemCall(f)
	}
	if h == nil {
		return Value{}, false
	}
	return Value{R: &Closure{Go: func(a []Value) Value { return h(m, a) }}}, true
}

// nativeMemCall returns the native call of f with its size check, for a
// call that is not static (a function value, go, defer), or nil when the
// calls of f are not checked. The arguments are the VM arguments, the
// receiver first for a method.
func (m *Machine) nativeMemCall(f *ssa.Function) NativeHook {
	size := m.nativeMemCheck(f)
	if size == nil {
		return nil
	}
	n := f.Signature.Params().Len()
	if f.Signature.Recv() != nil {
		n++
	}
	ident := make([]int32, n)
	for i := range ident {
		ident[i] = int32(i)
	}
	site := m.nativeSite(f, ident)
	return func(m *Machine, a []Value) Value {
		m.nativeMemGuard(size, a)
		r := m.callNativeSite(site, a)
		m.memResult(r)
		return r
	}
}

// nativeMemInvoke runs the size check of the call of the method name of
// the native receiver recv through an interface (an invoke site marked
// memCheck). fn is the method when the receiver is not a native value
// (callBodyless), else nil.
func (m *Machine) nativeMemInvoke(fn *ssa.Function, recv Value, name string, regs []Value, args []int32) {
	var size nativeMemSize
	if fn != nil {
		size = m.nativeMemCheck(fn)
	} else if rv, ok := recv.R.(reflect.Value); ok && rv.IsValid() {
		size = nativeMemMethods[methodKey{rv.Type(), name}]
	}
	if size == nil {
		return
	}
	a := make([]Value, 1+len(args))
	a[0] = recv
	for i, r := range args {
		a[i+1] = regs[r]
	}
	m.nativeMemGuard(size, a)
}

// repeatSize is the size of the result of Repeat of n bytes count times (0
// when it overflows or count is negative: Repeat panics itself).
func repeatSize(n, count int64) int64 {
	if count < 0 || n > 0 && count > math.MaxInt64/n {
		return 0
	}
	return n * count
}

// valLen is the length of an interpreted []byte or string.
func valLen(v Value) int {
	switch s := v.R.(type) {
	case []byte:
		return len(s)
	case []Value:
		return len(s)
	case string:
		return len(s)
	}
	return 0
}

// nativeMemCheck returns the size check of the native function f for the
// machine (nil: no quota, or nothing to check).
func (m *Machine) nativeMemCheck(f *ssa.Function) nativeMemSize {
	if m.mem == nil || f.Pkg == nil || m.isInterp(f) {
		return nil
	}
	if m.progVR && isReflectPkg(f.Pkg.Pkg) {
		return nil // the VM reflect (vmReflect.makeSlice)
	}
	return nativeMemSizes[hookName(f)]
}

// emitNativeMemCheck emits the check of size before the call of a native
// function with the arguments args.
func (c *compiler) emitNativeMemCheck(size nativeMemSize, args []int32) {
	m := c.m
	c.emit(Instr{Op: OpFunc, A: c.newReg(), X: func(regs []Value) Value {
		av := make([]Value, len(args))
		for i, r := range args {
			av[i] = regs[r]
		}
		if n := size(m, av); n >= memMin {
			m.memCheck(n)
		}
		return Value{}
	}})
}

// emitNativeMemCount emits the count of the result in dst of a checked
// native call, when it is a large string or []byte.
func (c *compiler) emitNativeMemCount(dst int32) {
	m := c.m
	c.emit(Instr{Op: OpFunc, A: c.newReg(), X: func(regs []Value) Value {
		m.memResult(regs[dst])
		return Value{}
	}})
}

// memResult counts v, the result of a native call, when it is a large
// string or []byte. It does not panic: the quota was checked before the
// call, and a result that another goroutine of the plugin pushed past the
// quota in between is counted all the same.
func (m *Machine) memResult(v Value) {
	switch r := v.R.(type) {
	case string:
		if n := int64(len(r)); n >= memMin {
			m.mem.mu.Lock()
			m.mem.add(n, unsafe.Pointer(unsafe.StringData(r)))
			m.mem.mu.Unlock()
		}
	case []byte:
		if n := int64(cap(r)); n >= memMin {
			m.mem.mu.Lock()
			m.mem.add(n, unsafe.Pointer(unsafe.SliceData(r[:cap(r)])))
			m.mem.mu.Unlock()
		}
	}
}

// nativeMemHook returns the replacement of the native function f for the
// machine (nil: no quota, or none).
func (m *Machine) nativeMemHook(f *ssa.Function) NativeHook {
	if m.mem == nil || f.Pkg == nil || m.isInterp(f) || f.Signature.Recv() != nil {
		return nil
	}
	return nativeMemHooks[hookName(f)]
}

// memReadAll is io.ReadAll of Go 1.26 (the same chunks, the same final
// size) with a check of the quota before each new buffer: the buffers that
// it holds and the new one must fit, or it panics with ErrMemory. The result
// is counted when it is large.
func (m *Machine) memReadAll(r io.Reader) ([]byte, error) {
	b := make([]byte, 0, 512)
	next := 256
	chunks := make([][]byte, 0, 4)
	var finalSize int
	held := int64(cap(b)) // the bytes of chunks and b
	check := func(n int) {
		if held+int64(n) >= memMin {
			m.memCheck(held + int64(n))
		}
	}
	for {
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			if len(chunks) == 0 {
				return b, err
			}
			finalSize += len(b)
			check(finalSize)
			final := append([]byte(nil), make([]byte, finalSize)...)[:0]
			for _, chunk := range chunks {
				final = append(final, chunk...)
			}
			final = append(final, b...)
			m.memResult(Value{R: final})
			return final, err
		}
		if cap(b)-len(b) < cap(b)/16 {
			chunks = append(chunks, b)
			finalSize += len(b)
			check(next)
			b = append([]byte(nil), make([]byte, next)...)[:0]
			held += int64(cap(b))
			next += next / 2
		}
	}
}
