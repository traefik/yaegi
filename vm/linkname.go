package vm

// Functions without a Go body that a package run from source links to the
// runtime or to syscall with //go:linkname.
//
// The VM cannot run assembly, and the runtime versions of these functions
// work on the memory of gc, which interpreted values do not have. Some of
// them are only small helpers that the VM can give with the same meaning,
// on interpreted values and with its own checks (runtimeLinks): they work
// for every plugin, as in Go.
//
// The others stay refused at load time (preflight), unless the host sets
// Options.StubAssembly: then the load does not fail on them, a call of one
// panics with "unsupported" (the panic that the load-time check prevents),
// and the system calls that go through the C library on darwin and openbsd
// (libcSyscalls), or through the assembly of golang.org/x/sys/unix on the
// other unix systems (rawSyscalls), fail with ENOSYS without calling the
// system.

import (
	"go/ast"
	"go/types"
	"strings"
	"syscall"
	_ "unsafe" // go:linkname

	"golang.org/x/tools/go/ssa"
)

//go:linkname runtimeNanotime runtime.nanotime
func runtimeNanotime() int64

//go:linkname runtimeGetAuxv runtime.getAuxv
func runtimeGetAuxv() []uintptr

// runtimeLinks are the targets of //go:linkname that the VM gives to every
// plugin, with the meaning they have in Go:
//
//   - runtime.nanotime: the monotonic clock (wazero's platform.Nanotime);
//   - runtime.getAuxv: a copy of the auxiliary vector of the process, read
//     only (golang.org/x/sys/cpu reads the CPU features from it on linux);
//   - runtime.memmove: a copy of n bytes between two places of bytes that
//     the VM knows (a Go []byte, the bytes of a string, plain data in real
//     bytes), with the checks of unsafe.Slice: a copy that reaches outside
//     these bytes, or cells of other values, is refused. wazero takes the
//     address of memmove in an init. When the SSA shows the same type T
//     behind both pointers and n is the size of T, the call is the
//     assignment *dst = *src, also for a T held in cells (memmoveTyped).
var runtimeLinks = map[string]NativeHook{
	"runtime.nanotime": func(m *Machine, a []Value) Value { return Value{N: runtimeNanotime()} },
	"runtime.getAuxv": func(m *Machine, a []Value) Value {
		aux := runtimeGetAuxv()
		if aux == nil {
			return Value{}
		}
		out := make([]Value, len(aux))
		for i, x := range aux {
			out[i] = Value{N: int64(x)}
		}
		return Value{R: out}
	},
	"runtime.memmove": func(m *Machine, a []Value) Value {
		m.memmove(a[0], a[1], uint64(a[2].N))
		return Value{}
	},
}

// linkrand.go adds the random and hash functions of the runtime to
// runtimeLinks.

// libcSyscalls are the targets of the functions that make a system call
// through the C library (darwin, openbsd): golang.org/x/sys/unix,
// golang.org/x/sys/cpu and wazero declare syscall_syscall6... without a body
// and pass the address of a C function, that an assembly file of the
// package holds. The VM has neither the assembly nor the address.
var libcSyscalls = map[string]bool{
	"syscall.syscall":      true,
	"syscall.syscall6":     true,
	"syscall.syscall6X":    true,
	"syscall.syscall9":     true,
	"syscall.syscallPtr":   true,
	"syscall.rawSyscall":   true,
	"syscall.rawSyscall6":  true,
	"syscall.rawSyscall9":  true,
	"syscall.rawsyscall10": true,
	"syscall.syscall10":    true,
	"syscall.syscall10X":   true,
}

// rawSyscalls are the functions of golang.org/x/sys/unix written in
// assembly that make a system call directly (linux, freebsd...: the
// assembly jumps to syscall.Syscall). They return an Errno, so the stub
// returns ENOSYS, as libcSyscalls on darwin. SyscallNoError and
// RawSyscallNoError have no error result: they stay stubs that panic.
var rawSyscalls = map[string]bool{
	"golang.org/x/sys/unix.Syscall":     true,
	"golang.org/x/sys/unix.Syscall6":    true,
	"golang.org/x/sys/unix.RawSyscall":  true,
	"golang.org/x/sys/unix.RawSyscall6": true,
}

// linknamesOf returns the target of each //go:linkname directive that names
// a package-level function of src ("runtime.nanotime" for
// "//go:linkname nanotime runtime.nanotime"). As in gc, the directive
// counts only in a file that imports "unsafe" (gc refuses to compile the
// others): a restricted plugin without Options.UseUnsafe cannot use it.
func linknamesOf(src []sourcePkg) map[*types.Func]string {
	var out map[*types.Func]string
	for _, p := range src {
		for _, f := range p.files {
			if !fileImportsUnsafe(f) {
				continue
			}
			for _, cg := range f.Comments {
				for _, c := range cg.List {
					local, target, ok := parseLinkname(c)
					if !ok {
						continue
					}
					fn, _ := p.types.Scope().Lookup(local).(*types.Func)
					if fn == nil {
						continue
					}
					if out == nil {
						out = map[*types.Func]string{}
					}
					out[fn] = target
				}
			}
		}
	}
	return out
}

// fileImportsUnsafe reports whether the file imports "unsafe".
func fileImportsUnsafe(f *ast.File) bool {
	for _, im := range f.Imports {
		if im.Path != nil && im.Path.Value == `"unsafe"` {
			return true
		}
	}
	return false
}

func parseLinkname(c *ast.Comment) (local, target string, ok bool) {
	rest, ok := strings.CutPrefix(c.Text, "//go:linkname ")
	if !ok {
		return "", "", false
	}
	fs := strings.Fields(rest)
	if len(fs) != 2 {
		return "", "", false
	}
	return fs[0], fs[1], true
}

// linkTarget returns the //go:linkname target of the function f without
// body, "" when there is none.
func (m *Machine) linkTarget(f *ssa.Function) string {
	if obj, ok := f.Object().(*types.Func); ok {
		return m.linknames[obj]
	}
	return ""
}

// bodylessHook returns the function that the VM runs for f, a function
// without a Go body of a package run from source: a runtimeLinks entry, or
// with Options.StubAssembly a stub. nil when f has a body, has a native
// implementation, or stays refused.
// maxUintptr is ^uintptr(0), a variable: as a constant, int64(^uintptr(0))
// does not compile on 64-bit.
var maxUintptr = ^uintptr(0)

func (m *Machine) bodylessHook(f *ssa.Function) NativeHook {
	if len(f.Blocks) != 0 || f.Pkg == nil || !m.interpPkg(f.Pkg.Pkg) || f.Signature.Recv() != nil {
		return nil
	}
	target := m.linkTarget(f)
	if h := runtimeLinks[target]; h != nil && linkSignatureOK(target, f.Signature) {
		return h
	}
	if !m.stubAsm || m.hasNativeFunc(f) {
		return nil
	}
	if (libcSyscalls[target] || rawSyscalls[f.Pkg.Pkg.Path()+"."+f.Name()]) && f.Signature.Results().Len() == 3 {
		// r1, r2 uintptr, err Errno: the call fails, as on a system
		// without this call; nothing is called. r1 is ^uintptr(0) at the
		// size of uintptr (truncN): -1 was another value on 32-bit.
		return func(m *Machine, a []Value) Value {
			return Value{R: []Value{{N: int64(maxUintptr)}, {}, {N: int64(syscall.ENOSYS)}}}
		}
	}
	name := f.String()
	return func(m *Machine, a []Value) Value {
		unsupported("%s has no Go body (it is written in assembly): Options.StubAssembly loaded the plugin, but the VM cannot run the function", name)
		return Value{}
	}
}

// linkSignatureOK reports whether sig, the signature a program declares
// for the runtime function target, is the one the hook of runtimeLinks
// expects (the arguments it reads, the result it gives). With another
// signature the function stays refused, or is a stub with StubAssembly.
func linkSignatureOK(target string, sig *types.Signature) bool {
	isInt := func(t types.Type) bool {
		b, ok := t.Underlying().(*types.Basic)
		return ok && b.Info()&types.IsInteger != 0
	}
	isPtr := func(t types.Type) bool {
		switch u := t.Underlying().(type) {
		case *types.Pointer:
			return true
		case *types.Basic:
			return u.Kind() == types.UnsafePointer
		}
		return false
	}
	ps, rs := sig.Params(), sig.Results()
	switch target {
	case "runtime.nanotime":
		return ps.Len() == 0 && rs.Len() == 1 && isInt(rs.At(0).Type())
	case "runtime.getAuxv":
		if ps.Len() != 0 || rs.Len() != 1 {
			return false
		}
		s, ok := rs.At(0).Type().Underlying().(*types.Slice)
		return ok && isInt(s.Elem())
	case "runtime.memmove":
		return ps.Len() == 3 && rs.Len() == 0 && !sig.Variadic() &&
			isPtr(ps.At(0).Type()) && isPtr(ps.At(1).Type()) && isInt(ps.At(2).Type())
	}
	return randLinkSignatureOK(target, sig)
}

// hasNativeFunc reports whether nativeFunc finds an implementation of f.
func (m *Machine) hasNativeFunc(f *ssa.Function) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	m.nativeFunc(f)
	return true
}

// memmove copies n bytes from src to dst (runtime.memmove), when both are
// bytes that the VM knows and the n bytes are inside them.
func (m *Machine) memmove(dst, src Value, n uint64) {
	if n == 0 {
		return
	}
	d := m.knownBytes(dst, n, true, "runtime.memmove")
	s := m.knownBytes(src, n, false, "runtime.memmove")
	copy(d, s)
}

// typedMemmove returns T when the call of callee is runtime.memmove (a
// runtimeLinks entry) and the SSA shows that both pointers are
// unsafe.Pointer(p) of a *T, with the same T on both sides. nil otherwise.
func (c *compiler) typedMemmove(callee *ssa.Function, args []ssa.Value) types.Type {
	if c.m.linkTarget(callee) != "runtime.memmove" || len(args) != 3 {
		return nil
	}
	elem := func(v ssa.Value) types.Type {
		cv, ok := v.(*ssa.Convert)
		if !ok {
			return nil
		}
		pt, ok := cv.X.Type().Underlying().(*types.Pointer)
		if !ok {
			return nil
		}
		return pt.Elem()
	}
	dt, st := elem(args[0]), elem(args[1])
	if dt == nil || st == nil || !types.Identical(dt, st) {
		return nil
	}
	return dt
}

// memmoveTyped is runtime.memmove(dst, src, n) when the SSA shows that dst
// and src are pointers to values of type t. A copy of the whole value
// (n == unsafe.Sizeof of t) is the assignment *dst = *src, as in gc, for
// every type: a value held in cells (a struct with a string) has no bytes
// to copy, but it has the same meaning. Other sizes go to memmove (the
// hook h), which copies only bytes that the VM knows and refuses the rest.
func (m *Machine) memmoveTyped(a []Value, t types.Type, h NativeHook) Value {
	if uint64(a[2].N) != uint64(gcSizes.Sizeof(t)) || a[2].N == 0 {
		return h(m, a)
	}
	if a[0].R == nil || a[1].R == nil {
		panic(errNilDeref)
	}
	s := newMemSite(t)
	m.store(a[0], m.load(a[1], s), s)
	return Value{}
}

// knownBytes returns the n bytes from the interpreted pointer p, refused
// when the VM does not know that many bytes there. name is the runtime
// function that reads them, for the refusals (memmove, memhash...).
func (m *Machine) knownBytes(p Value, n uint64, write bool, name string) []byte {
	if p.R == nil {
		panic(errNilDeref)
	}
	r := m.findRegionPtr(p)
	if r == nil {
		r = m.extents.region(p)
	}
	if r == nil {
		r = lone(p, nil)
	}
	if r == nil || r.bytes == nil {
		unsupported("%s of a %T: the VM can only copy the bytes of a []byte, of a string or of plain data in real bytes", name, p.R)
	}
	if write && r.ro {
		refuseROWrite()
	}
	i, ok := r.index(p)
	if !ok {
		unsupported("%s on a pointer the VM cannot follow", name)
	}
	b := r.bytes[i:]
	if n > uint64(len(b)) {
		unsupported("%s: %d bytes reach outside the memory known to the VM (%d bytes)", name, n, len(b))
	}
	return b[:n:n]
}

// stubAsmHint ends the load error of a function without a Go body.
const stubAsmHint = " (Options.StubAssembly loads the plugin anyway: a call then fails at run time)"
