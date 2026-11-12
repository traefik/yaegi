package vm

// The golang.org/x/sys/unix subset of a restricted plugin with
// Options.UseUnsafe (xsys.go).
//
// The host's golang.org/x/sys/unix makes real system calls, and many of its
// functions act on the host process or its memory: Mmap, Munmap, Mprotect,
// Syscall*, RawSyscall*, PtraceAttach, Kill, Exec, Open, Write, Close,
// Setsockopt* of any option, Ioctl* of any request... A restricted plugin
// gets a read-only subset only, as the subset of syscall
// (unsafesyscall.go): functions that only read information about the
// process, the system or the time and write only to the value the plugin
// gives, pure conversions, and a few functions that take a file descriptor,
// checked as the ones of syscall:
//
//   - Getpagesize, Getpid, Getppid, Getuid, Geteuid, Getgid, Getegid,
//     Gettid: what os.Getpagesize, os.Getpid... give already.
//   - Gettimeofday: the current time, as time.Now.
//   - ClockGettime, ClockGetres: the time of a clock, only for the clocks of
//     the system (0 to 31: CLOCK_REALTIME, CLOCK_MONOTONIC,
//     CLOCK_PROCESS_CPUTIME_ID...). A negative clock id is the CPU clock of
//     another process or thread, or a clock of a file descriptor (a PTP
//     device): refused. gRPC reads CLOCK_PROCESS_CPUTIME_ID on linux.
//   - Uname: the name and version of the system (the OpenTelemetry SDK
//     describes the host with it); os.Hostname gives the node name already.
//   - Getrusage, Getrlimit: the resource usage and limits of the process.
//   - Getenv, Environ: the plugin's own environment copy, never the one of
//     the host process (as syscall.Getenv in the subset of syscall).
//   - pure functions: NsecToTimespec, NsecToTimeval, TimespecToNsec,
//     TimevalToNsec, TimeToTimespec, ByteSliceToString,
//     ByteSliceFromString, ErrnoName, SignalName, SignalNum, Major, Minor,
//     Mkdev.
//   - SetsockoptInt, GetsockoptInt: only on the socket that a RawConn of the
//     net package lends to its callback, and only for the socket options of
//     the syscall subset (rawconn.go, rawconn_unix.go). gRPC sets
//     SO_KEEPALIVE so in the Control function of its dialer.
//   - IoctlGetTermios, IoctlGetWinsize: only with the request that reads the
//     terminal attributes (TCGETS on linux, TIOCGETA on darwin and the BSDs)
//     or the window size (TIOCGWINSZ), into a value of the right size, and
//     only on a descriptor that a RawConn lends to its callback or on the
//     plugin's own os.Stdin, os.Stdout and os.Stderr (its duplicates of the
//     host files, stdio.go). go-isatty (zap, zerolog) asks if os.Stdout is a
//     terminal this way. The request is fixed because ioctl writes as many
//     bytes as the request says, wherever the pointer points: another
//     request would write past the value.
//
// The types and the variables of the package are given: types are plain
// data, their methods only read and write the value (Timespec.Unix,
// FdSet.Set, CPUSet.Count...), and the load-time and run-time checks of
// writes to host variables apply to unix.Stdout and friends (restrict.go,
// hostmem.go), as to the variables of the standard library. Everything else
// is refused at load time, with the position of each use; with
// Options.StubAssembly it loads as a stub (xsysStub). Constants come from
// the plugin's source.

import (
	"fmt"
	"os"
	"reflect"
	"syscall"

	"golang.org/x/tools/go/ssa"
)

// xsysPlainNames are the functions of golang.org/x/sys/unix that a
// restricted plugin with UseUnsafe gets as they are (see the top of this
// file).
var xsysPlainNames = []string{
	"Getpagesize", "Getpid", "Getppid", "Getuid", "Geteuid", "Getgid", "Getegid", "Gettid",
	"Gettimeofday", "Uname", "Getrusage", "Getrlimit",
	"NsecToTimespec", "NsecToTimeval", "TimespecToNsec", "TimevalToNsec", "TimeToTimespec",
	"ByteSliceToString", "ByteSliceFromString", "ErrnoName", "SignalName", "SignalNum",
	"Major", "Minor", "Mkdev",
}

// xsysSubset returns the golang.org/x/sys/unix table of a restricted machine
// with UseUnsafe (nil for another restricted machine). It is built at the
// first use.
func (m *Machine) xsysSubset() *Package {
	if !m.unsafeSyscall {
		return nil
	}
	m.xsysOnce.Do(func() {
		all := natPkg(xsysUnixPath)
		if all == nil {
			return
		}
		funcs := map[string]reflect.Value{}
		for _, name := range xsysPlainNames {
			if v, ok := all.Funcs[name]; ok {
				funcs[name] = v
			}
		}
		for _, name := range []string{"ClockGettime", "ClockGetres"} {
			if v, ok := all.Funcs[name]; ok {
				funcs[name] = systemClock(name, v)
			}
		}
		if v, ok := funcOf[func(fd, level, opt, value int) error](all, "SetsockoptInt"); ok {
			funcs["SetsockoptInt"] = reflect.ValueOf(func(fd, level, opt, value int) error {
				m.checkSockopt("unix.SetsockoptInt", fd, level, opt)
				return v(fd, level, opt, value)
			})
		}
		if v, ok := funcOf[func(fd, level, opt int) (int, error)](all, "GetsockoptInt"); ok {
			funcs["GetsockoptInt"] = reflect.ValueOf(func(fd, level, opt int) (int, error) {
				m.checkSockopt("unix.GetsockoptInt", fd, level, opt)
				return v(fd, level, opt)
			})
		}
		// Recvfrom(fd int, p []byte, flags int) (int, Sockaddr, error)
		if v, ok := all.Funcs["Recvfrom"]; ok && v.Type().NumIn() == 3 && v.Type().In(0).Kind() == reflect.Int &&
			v.Type().In(1) == reflect.TypeFor[[]byte]() && v.Type().In(2).Kind() == reflect.Int {
			funcs["Recvfrom"] = reflect.MakeFunc(v.Type(), func(in []reflect.Value) []reflect.Value {
				m.checkRecvfrom("unix.Recvfrom", int(in[0].Int()), int(in[2].Int()))
				return v.Call(in)
			})
		}
		for name, req := range xsysIoctls {
			if v, ok := all.Funcs[name]; ok {
				funcs[name] = m.readIoctl(name, req, v)
			}
		}
		if e := m.env; e != nil {
			if _, ok := all.Funcs["Getenv"]; ok {
				funcs["Getenv"] = reflect.ValueOf(func(key string) (string, bool) {
					e.mu.RLock()
					defer e.mu.RUnlock()
					v, ok := e.vars[key]
					return v, ok
				})
			}
			if _, ok := all.Funcs["Environ"]; ok {
				funcs["Environ"] = reflect.ValueOf(e.environ)
			}
		}
		m.xsysSyms = &Package{Path: xsysUnixPath, Funcs: funcs, Types: all.Types, Vars: all.Vars, Module: all.Module, Version: all.Version}
	})
	return m.xsysSyms
}

// funcOf returns the function name of p as a value of type F.
func funcOf[F any](p *Package, name string) (F, bool) {
	var zero F
	v, ok := p.Funcs[name]
	if !ok || v.Type() != reflect.TypeFor[F]() {
		return zero, false
	}
	return v.Interface().(F), true
}

// systemClock wraps ClockGettime or ClockGetres (fn, whose first parameter
// is the clock id): only the clocks of the system, 0 to 31.
func systemClock(name string, fn reflect.Value) reflect.Value {
	return reflect.MakeFunc(fn.Type(), func(in []reflect.Value) []reflect.Value {
		if id := in[0].Int(); id < 0 || id > 31 {
			panic(&RefusedError{What: fmt.Sprintf("unix.%s of the clock %d (only the clocks of the system, 0 to 31)", name, id)})
		}
		return fn.Call(in)
	})
}

// readIoctl wraps IoctlGetTermios or IoctlGetWinsize (fn: fd, request) for
// the request req only, on a descriptor that a RawConn lends to the
// goroutine or on the plugin's own standard files.
func (m *Machine) readIoctl(name string, req uint64, fn reflect.Value) reflect.Value {
	return reflect.MakeFunc(fn.Type(), func(in []reflect.Value) []reflect.Value {
		var r uint64
		switch in[1].Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			r = uint64(in[1].Int())
		default:
			r = in[1].Uint()
		}
		if r != req {
			panic(&RefusedError{What: fmt.Sprintf("unix.%s with the request %#x (only %#x)", name, r, req)})
		}
		fd := in[0].Int()
		if fd < 0 || !m.fdLent(uintptr(fd)) && !m.ownStdioFD(int(fd)) {
			panic(&RefusedError{What: fmt.Sprintf("unix.%s of a file descriptor that is not the plugin's os.Stdin, os.Stdout or os.Stderr and that no RawConn callback lends to the plugin", name)})
		}
		return fn.Call(in)
	})
}

// ownStdioFD reports whether fd is the descriptor of the os.Stdin, os.Stdout
// or os.Stderr of the plugin (its own duplicates of the host files, or the
// files of Options.Stdout and Options.Stderr).
func (m *Machine) ownStdioFD(fd int) bool {
	for _, key := range []string{"os.Stdin", "os.Stdout", "os.Stderr"} {
		h := m.varHooks[key]
		if h == nil {
			continue
		}
		p, ok := h().Interface().(**os.File)
		if !ok || p == nil || *p == nil {
			continue
		}
		sc, err := (*p).SyscallConn()
		if err != nil {
			continue
		}
		got := -1
		if sc.Control(func(x uintptr) { got = int(x) }) == nil && got == fd {
			return true
		}
	}
	return false
}

// xsysStub returns, for a restricted machine with the golang.org/x/sys/unix
// subset and Options.StubAssembly, a stub of the function f of the package
// that is outside the subset: the plugin loads, and a call does not reach
// the system. A function whose last result is an error returns ENOSYS (as
// the system calls of the package run from source with StubAssembly: gRPC,
// wazero and zap ignore or handle the error); another one panics with a
// *RefusedError.
func (m *Machine) xsysStub(f *ssa.Function) (reflect.Value, bool) {
	if !m.stubAsm || !m.unsafeSyscall || m.realStd || f.Pkg.Pkg.Path() != xsysUnixPath || f.Signature.Recv() != nil ||
		m.interpPkg(f.Pkg.Pkg) || m.modeOf(xsysUnixPath) != ModeNative {
		return reflect.Value{}, false
	}
	all := natPkg(xsysUnixPath)
	if all == nil {
		return reflect.Value{}, false
	}
	real, ok := all.Funcs[f.Name()]
	if !ok {
		return reflect.Value{}, false
	}
	t := real.Type()
	what := "unix." + f.Name()
	if n := t.NumOut(); n > 0 && t.Out(n-1) == errorType {
		return reflect.MakeFunc(t, func([]reflect.Value) []reflect.Value {
			out := make([]reflect.Value, n)
			for i := range out {
				out[i] = reflect.Zero(t.Out(i))
			}
			out[n-1] = reflect.ValueOf(&enosys).Elem()
			return out
		}), true
	}
	return reflect.MakeFunc(t, func([]reflect.Value) []reflect.Value {
		panic(&RefusedError{What: what + " (outside the read-only golang.org/x/sys/unix subset; Options.StubAssembly loaded the plugin)"})
	}), true
}

// enosys is the error of the stubs of xsysStub.
var enosys error = syscall.ENOSYS
