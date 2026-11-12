package vm

// The file descriptors that a restricted plugin with Options.UseUnsafe may
// read with syscall.Read, and whose socket options it may set with
// syscall.SetsockoptInt.
//
// A plugin gets a syscall.RawConn from the SyscallConn method of a network
// connection (or of an *os.File) that it holds, and native code gives one to
// the Control function of a net.Dialer or net.ListenConfig of the plugin.
// The methods Control, Read and Write of a RawConn call a function of the
// plugin with the file descriptor of the connection. gRPC on linux reads
// the socket there with syscall.Read (internal/transport/readyreader).
//
// The file descriptor is a plain number, and the plugin can write any
// number. So syscall.Read is not in the syscall subset as it is: it reads
// only a descriptor that a RawConn of the net or os package lends to the
// plugin, on the goroutine of the call and during the call. These RawConn
// methods keep the descriptor open during the call (the connection holds a
// reference), so the number cannot be closed and given to another file of
// the host in the middle. A plugin can read with it only what the
// connection itself gives (conn.Read).
//
// The lending works when the plugin calls the method itself (a static call,
// a call through an interface, a method value: Machine.lookupMethod; a
// call through its reflect: restrictedMethodOf). By another path (a
// RawConn of a type outside net and os, a RawConn that the plugin
// implements), the descriptor is not lent, and syscall.Read refuses it: the
// plugin fails, the host keeps its descriptors. A RawConn that the plugin
// implements is never given to native code (wrappers.go), so native code
// never acts on a descriptor that the plugin chose.
//
// A descriptor that a RawConn of the net package lends is a socket of a
// network connection that the plugin holds. On it, and only during the
// callback as for Read, the plugin may also call syscall.SetsockoptInt and
// syscall.GetsockoptInt, for a short list of options that tune the
// connection (sockopt*.go): keepalive, Nagle, buffer sizes, the TCP user
// timeout. gRPC sets TCP_USER_TIMEOUT on each connection of its server
// (internal/syscall on linux). Any other option (SO_MARK, SO_BINDTODEVICE,
// SO_REUSEPORT, IP_TRANSPARENT, the *FORCE options...) acts on the host
// network or needs privileges: it panics with a *RefusedError, as a
// descriptor that is not lent. A descriptor lent by a RawConn of the os
// package (an *os.File) is not a socket of the plugin: only Read takes it.
// A RawConn of a connection that the host gave the plugin (the net.Conn of
// a hijacked http.ResponseWriter) lends its socket too: the plugin owns
// that connection then (Close, Write), and the options of the list are
// those of its SetNoDelay, SetKeepAlive, SetKeepAliveConfig, SetReadBuffer
// and SetWriteBuffer, plus TCP_USER_TIMEOUT and TCP_QUICKACK on linux,
// which tune that connection only.
//
// golang.org/x/sys/unix.SetsockoptInt and GetsockoptInt have the same rules
// when the plugin gets the host's golang.org/x/sys/unix (xsys.go,
// xsysunix.go): gRPC sets SO_KEEPALIVE with it in the Control function of
// its dialer. Run from source (a plugin whose x/sys does not fit the host's
// copy), they end in the assembly Syscall6 (linux) or in a call through the
// C library (darwin), which return ENOSYS with Options.StubAssembly.
//
// A lent descriptor can be one of the host when the file is: the plugin
// gets its os.Stdin as the host file (os.Stdin.SyscallConn lends fd 0).
// Read of such a lent descriptor gives what the Read method of that file
// gives already: the lending adds no new descriptor. os.NewFile, which made
// an *os.File of any number, is refused in a restricted plugin
// (newfile.go).
//
// Nothing else takes a descriptor in the subset: syscall.Write, Close,
// Setsockopt* other than SetsockoptInt... stay refused (with
// Options.StubAssembly, they load and a call panics, see
// Machine.nativeFunc).

import (
	"reflect"
	"syscall"
)

// lentFD is a file descriptor lent to the goroutine g. sock is true when
// a RawConn of the net package lends it (a socket).
type lentFD struct {
	g, fd uintptr
	sock  bool
}

var rawConnType = reflect.TypeFor[syscall.RawConn]()

// lendFD records that the goroutine that runs it may read fd (and set the
// allowed options of the socket when sock is true), until the returned
// function is called.
func (m *Machine) lendFD(fd uintptr, sock bool) func() {
	k := lentFD{getg(), fd, sock}
	m.fdMu.Lock()
	if m.lentFDs == nil {
		m.lentFDs = map[lentFD]int{}
	}
	m.lentFDs[k]++
	m.fdMu.Unlock()
	return func() {
		m.fdMu.Lock()
		if m.lentFDs[k]--; m.lentFDs[k] <= 0 {
			delete(m.lentFDs, k)
		}
		m.fdMu.Unlock()
	}
}

// fdLent reports whether a RawConn lends fd to the current goroutine.
func (m *Machine) fdLent(fd uintptr) bool {
	g := getg()
	m.fdMu.Lock()
	defer m.fdMu.Unlock()
	return m.lentFDs[lentFD{g, fd, false}] > 0 || m.lentFDs[lentFD{g, fd, true}] > 0
}

// rawConnMethodOf returns, for a restricted machine with the syscall
// subset, the replacement of the method Control, Read or Write of a RawConn
// of the net or os package: the same call, with the descriptor lent to the
// function of the plugin while it runs.
func rawConnMethodOf(m *Machine, t reflect.Type, name string) (reflect.Value, bool) {
	if name != "Control" && name != "Read" && name != "Write" ||
		m == nil || !m.unsafeSyscall || m.realStd || !t.Implements(rawConnType) {
		return reflect.Value{}, false
	}
	pt := t
	if pt.Kind() == reflect.Pointer {
		pt = pt.Elem()
	}
	p := pt.PkgPath()
	if p != "net" && p != "os" {
		return reflect.Value{}, false
	}
	sock := p == "net"
	switch name {
	case "Control":
		return reflect.ValueOf(func(c syscall.RawConn, f func(fd uintptr)) error {
			return c.Control(func(fd uintptr) {
				defer m.lendFD(fd, sock)()
				f(fd)
			})
		}), true
	case "Read":
		return reflect.ValueOf(func(c syscall.RawConn, f func(fd uintptr) bool) error {
			return c.Read(func(fd uintptr) bool {
				defer m.lendFD(fd, sock)()
				return f(fd)
			})
		}), true
	case "Write":
		return reflect.ValueOf(func(c syscall.RawConn, f func(fd uintptr) bool) error {
			return c.Write(func(fd uintptr) bool {
				defer m.lendFD(fd, sock)()
				return f(fd)
			})
		}), true
	}
	return reflect.Value{}, false
}
