package vm

// os.NewFile in a restricted plugin.
//
// os.NewFile(fd, name) makes an *os.File of a number. The number has no
// path to check: in a restricted plugin it gave any open descriptor of the
// host (a file, a socket, a listener that systemd gives to Traefik), to
// read, write, close (also by the cleanup of the *os.File when the plugin
// drops it), or to tune through its SyscallConn. Even a descriptor that the
// plugin opened itself is not safe: the new *os.File is a second owner of
// the number, and when one of the two closes it, the number can go to a
// file of the host, which the other one then reads, writes or closes.
//
// So a restricted machine refuses os.NewFile at run time: the call panics
// with a *RefusedError and makes no file. The check is not at load time
// because libraries of plugins of the catalog name os.NewFile in code that
// the plugin does not call (gin.(*Engine).RunFd, the listeners of caddy),
// and refusing the name would refuse the whole plugin. A plugin gets a
// function value only by naming the function, so the override covers the
// calls through a function value too. Unrestricted machines keep the
// os.NewFile of Go.
//
// The other ways to a descriptor of the host (/dev/fd/N, /proc/self/fd/N,
// net.FileConn, the host os.Stdout) are closed in hostfd.go, which calls
// refuseNewFile.

import "os"

// refuseNewFile replaces os.NewFile with a function that panics, for a
// restricted machine (guardHostFiles).
func (m *Machine) refuseNewFile() {
	if !m.restricted {
		return
	}
	m.addOverride("os.NewFile", func(fd uintptr, name string) *os.File {
		panic(&RefusedError{What: "os.NewFile (a file of any descriptor number)"})
	})
}
