package vm

// The syscall subset of a restricted plugin with Options.UseUnsafe.
//
// yaegi gives such a plugin the whole syscall package. yaegi v2 registers the
// syscall tables in stdsyms.Unrestricted only, so a restricted plugin got no
// syscall function at all, and a plugin that called syscall.Getpid did not
// load. A restricted plugin with UseUnsafe now gets a small read-only
// subset: functions that only read information about the process or the
// time, and the environment through the plugin's own copy (env.go). The
// rest of syscall stays refused: everything that opens, writes, signals,
// starts processes, maps memory, makes sockets or raw system calls, or
// changes the process (Kill, Exec, ForkExec, Mmap, Syscall*, Setenv,
// Chdir, Open...).
//
// Five functions take a file descriptor: syscall.Read and Fstatfs, only for
// the descriptor that a RawConn of the plugin lends to its callback, and
// syscall.Recvfrom, SetsockoptInt and GetsockoptInt, only for a socket lent
// that way, with MSG_PEEK and a short list of options (rawconn.go). With Options.StubAssembly, the
// functions outside the subset load, and a call panics
// (Machine.nativeFunc): nothing reaches the system.
//
// The subset takes its functions from the syscall table of this platform
// (stdsyms.Unrestricted): a function that the platform does not have, or a
// platform without a syscall table (stdtypes.Platforms), stays refused.

import (
	"reflect"
	"sort"

	"github.com/traefik/yaegi/v2/stdsyms"
)

// unsafeSyscallNames are the names of syscall that a restricted plugin with
// Options.UseUnsafe gets from the real syscall table. Each one only reads
// and returns plain values (no file descriptor, no handle, no pointer):
//
//   - Getpid, Getppid: the process id and its parent, as os.Getpid and
//     os.Getppid, which a restricted plugin already has.
//   - Getuid, Geteuid, Getgid, Getegid: the user and group ids, as
//     os.Getuid and friends (-1 on Windows).
//   - Getpagesize: the memory page size, as os.Getpagesize.
//   - Gettimeofday: the current time, as time.Now. It writes only to the
//     Timeval that the plugin gives.
//   - Errno, Timeval: types, so that the plugin can use the error values
//     that syscall returns (errors.Is(err, syscall.ECONNREFUSED), a type
//     switch on Errno) and the argument of Gettimeofday. Their methods only
//     read the value (Error, Is, Temporary, Timeout, Unix, Nano).
//   - Uname, Utsname (linux): the name and version of the system, as
//     unix.Uname in the golang.org/x/sys/unix subset (xsysunix.go). It
//     writes only to the Utsname the plugin gives. golang.org/x/sys/cpu
//     reads the kernel version with it on linux/arm64 (wazero).
//   - Signal: a type, the signal numbers that golang.org/x/sys/unix uses in
//     its API (unix.Signal is syscall.Signal: Kill, SignalName...); its
//     methods only read the value (Signal, String). No function of the
//     subset sends a signal.
//   - Timespec, and NsecToTimespec, TimespecToNsec, NsecToTimeval,
//     TimevalToNsec: a plain struct of two integers and pure conversions
//     between it and nanoseconds. wazero fills a Timespec from the
//     os.FileInfo of a file (sys/stat_bsd.go).
//   - Statfs, Statfs_t (unix): the statistics of the file system of a path
//     (its type, its sizes and free blocks; on darwin and the BSDs also the
//     mount point, the device and the owner of the file system), as
//     os.Stat gives the metadata of a file, which a restricted plugin
//     already has. It writes only to
//     the Statfs_t the plugin gives. prometheus/procfs checks with it that
//     /proc is a procfs (prometheus/client_golang imports procfs). With
//     Options.Sandbox, the path must be readable, as for os.Stat
//     (sandbox_unix.go). Fstatfs takes a descriptor: only a lent one
//     (rawconn.go).
//   - RawConn, Conn: interface types. (*net.TCPConn).SyscallConn and the
//     Control function of a net.Dialer give a RawConn; gRPC sets its dialer
//     Control and reads its own sockets through it. Its methods call a
//     function of the plugin with the file descriptor of the connection: a
//     number, which no function of the subset accepts, except syscall.Read,
//     SetsockoptInt and GetsockoptInt during the callback (rawconn.go). A
//     type of the plugin is never given
//     to native code as a Conn or a RawConn: the wrappers of these
//     interfaces are only for the standard library as in Go (wrappers.go),
//     so native code (the sendfile of net) never calls a RawConn of the
//     plugin and never acts on a descriptor that the plugin chose.
//
// The constants of syscall need no table: the compiler knows them.
var unsafeSyscallNames = []string{
	"Getpid", "Getppid",
	"Getuid", "Geteuid", "Getgid", "Getegid",
	"Getpagesize", "Gettimeofday", "Uname", "Utsname",
	"Errno", "Signal", "Timeval", "Timespec", "Statfs", "Statfs_t",
	"NsecToTimespec", "TimespecToNsec", "NsecToTimeval", "TimevalToNsec",
	"RawConn", "Conn",
}

// syscallSubset returns the syscall table of a restricted machine with
// Options.UseUnsafe: the entries of unsafeSyscallNames that the platform
// has, Read, Recvfrom, Fstatfs, SetsockoptInt and GetsockoptInt over the lent file
// descriptors (rawconn.go), and Getenv and Environ over the environment
// copy of the machine (SetEnv). It returns nil on a platform without a
// syscall table. It is built at the first use.
func (m *Machine) syscallSubset() map[string]reflect.Value {
	m.syscallOnce.Do(func() {
		all := stdsyms.Unrestricted["syscall/syscall"]
		if all == nil {
			return
		}
		syms := map[string]reflect.Value{}
		for _, name := range unsafeSyscallNames {
			if v, ok := all[name]; ok {
				syms[name] = v
			}
		}
		// syscall.Read reads only a descriptor that a RawConn lends to
		// the callback that runs (rawconn.go).
		if _, ok := all["Read"]; ok {
			if r := lentRead(m); r.IsValid() {
				syms["Read"] = r
			}
		}
		// SetsockoptInt and GetsockoptInt act only on a socket that a
		// RawConn of the net package lends, for the options of
		// allowedSockopts, and Recvfrom only peeks at it (rawconn.go).
		for name, f := range map[string]func(*Machine) reflect.Value{
			"SetsockoptInt": lentSetsockoptInt,
			"GetsockoptInt": lentGetsockoptInt,
			"Recvfrom":      lentRecvfrom,
			"Fstatfs":       lentFstatfs,
		} {
			if _, ok := all[name]; ok {
				if r := f(m); r.IsValid() {
					syms[name] = r
				}
			}
		}
		// The environment is the plugin's own copy, never the one of the
		// host process. Without a copy, Getenv and Environ stay refused.
		if e := m.env; e != nil {
			if _, ok := all["Getenv"]; ok {
				syms["Getenv"] = reflect.ValueOf(func(key string) (string, bool) {
					e.mu.RLock()
					defer e.mu.RUnlock()
					v, ok := e.vars[key]
					return v, ok
				})
			}
			if _, ok := all["Environ"]; ok {
				syms["Environ"] = reflect.ValueOf(e.environ)
			}
		}
		m.syscallSyms = syms
	})
	return m.syscallSyms
}

// environ returns the variables of e as "key=value" entries, sorted.
func (e *vmEnv) environ() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]string, 0, len(e.vars))
	for k, v := range e.vars {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
