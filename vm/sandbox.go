package vm

// Capability checks for a plugin (docs/SANDBOX.md, layer 2). On top of the
// load-time restrictions (restrict.go) and the own environment copy
// (env.go), a Sandbox denies some filesystem and network functions by
// default and allows only what the caller lists, in the spirit of the
// capabilities of Wasm runtimes (mounts, environment variables). It works by replacing the
// os and net functions listed in this file (Machine.overrides); every other
// function stays native.
//
// This is a best effort against accidents, not a security boundary: an
// in-process Go interpreter shares the host's address space, and the
// replacement table covers only some entry points. The checks cover a
// subset of the os and net functions, the path check is lexical, and native
// code that the plugin reaches by other functions is not checked. A host
// that runs untrusted code needs process isolation (docs/SANDBOX.md).
//
// (Process-wide registries such as http.DefaultServeMux are per plugin, see
// hostregs.go.)
// TestSandbox covers only the checks that hold.

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sandbox lists the capabilities a plugin is granted. With the zero value,
// the plugin sees an empty environment, and the os and net functions
// replaced in this file refuse every path and address. This is not a
// boundary: many other functions still reach the filesystem and the
// network (see the file comment). It always implies the load-time
// restrictions (as if Unrestricted is false) and sends the panics of the
// plugin's goroutines to GoPanic.
type Sandbox struct {
	// Env is the environment the plugin sees ("key=value" entries); nil is an
	// empty environment, not the host's.
	Env []string
	// ReadPaths and WritePaths are the directories (and the files directly in
	// them, recursively) the plugin may read, respectively read and write.
	// A WritePaths entry also grants reading. Paths are made absolute.
	ReadPaths  []string
	WritePaths []string
	// DialHosts are the "host:port" the plugin may reach with net.Dial and
	// net.DialTimeout; "host" without a port allows every port of that host.
	// Only these two functions are checked: net.Dialer, net/http and the
	// other dial functions are not (see the file comment).
	DialHosts []string
	// Budget bounds every call from the host into the plugin (Options.Budget).
	// 0 keeps it unlimited.
	Budget int64
	// MaxGoroutines bounds the live goroutines of the plugin
	// (Options.MaxGoroutines). 0 means no cap.
	MaxGoroutines int
	// GoPanic receives the panics of the plugin's goroutines, and of the
	// callbacks that native code runs on goroutines of its own; nil prints
	// them on stderr. The host keeps running either way.
	GoPanic func(r any)
}

// apply installs the sandbox on the machine (called by Options.newMachine).
func (s *Sandbox) apply(m *Machine) {
	m.SetEnv(s.Env)
	fs := &fsCaps{read: absPaths(s.ReadPaths, s.WritePaths), write: absPaths(s.WritePaths)}
	fs.install(m)
	installDialCaps(m, s.DialHosts)
}

// absPaths returns the cleaned, absolute forms of the given path groups.
func absPaths(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		for _, p := range g {
			if abs, err := filepath.Abs(p); err == nil {
				out = append(out, filepath.Clean(abs))
			}
		}
	}
	return out
}

// fsCaps enforces the filesystem capabilities.
type fsCaps struct {
	read  []string // directories readable (write paths included)
	write []string // directories writable
}

// allows reports whether name is inside one of roots.
func allows(roots []string, name string) bool {
	abs, err := filepath.Abs(name)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	for _, r := range roots {
		if abs == r || strings.HasPrefix(abs, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (c *fsCaps) denied(op, name string) error {
	return &os.PathError{Op: op, Path: name, Err: errDenied}
}

func (c *fsCaps) install(m *Machine) {
	readOnly := func(op, name string) error {
		if !allows(c.read, name) {
			return c.denied(op, name)
		}
		return nil
	}
	writable := func(op, name string) error {
		if !allows(c.write, name) {
			return c.denied(op, name)
		}
		return nil
	}
	m.addOverride("os.Open", func(name string) (*os.File, error) {
		if err := readOnly("open", name); err != nil {
			return nil, err
		}
		return openFile(name, os.O_RDONLY, 0)
	})
	m.addOverride("os.ReadFile", func(name string) ([]byte, error) {
		if err := readOnly("open", name); err != nil {
			return nil, err
		}
		return readFile(name)
	})
	m.addOverride("os.ReadDir", func(name string) ([]os.DirEntry, error) {
		if err := readOnly("open", name); err != nil {
			return nil, err
		}
		return os.ReadDir(name)
	})
	m.addOverride("os.Stat", func(name string) (os.FileInfo, error) {
		if err := readOnly("stat", name); err != nil {
			return nil, err
		}
		return os.Stat(name)
	})
	c.installUnix(m, readOnly)
	m.addOverride("os.Lstat", func(name string) (os.FileInfo, error) {
		if err := readOnly("lstat", name); err != nil {
			return nil, err
		}
		return os.Lstat(name)
	})
	m.addOverride("os.OpenFile", func(name string, flag int, perm os.FileMode) (*os.File, error) {
		op, check := "open", readOnly
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
			op, check = "open", writable
		}
		if err := check(op, name); err != nil {
			return nil, err
		}
		return openFile(name, flag, perm)
	})
	m.addOverride("os.Create", func(name string) (*os.File, error) {
		if err := writable("open", name); err != nil {
			return nil, err
		}
		return createFile(name)
	})
	m.addOverride("os.WriteFile", func(name string, data []byte, perm os.FileMode) error {
		if err := writable("open", name); err != nil {
			return err
		}
		return writeFile(name, data, perm)
	})
	m.addOverride("os.Mkdir", func(name string, perm os.FileMode) error {
		if err := writable("mkdir", name); err != nil {
			return err
		}
		return os.Mkdir(name, perm)
	})
	m.addOverride("os.MkdirAll", func(name string, perm os.FileMode) error {
		if err := writable("mkdir", name); err != nil {
			return err
		}
		return os.MkdirAll(name, perm)
	})
	m.addOverride("os.Remove", func(name string) error {
		if err := writable("remove", name); err != nil {
			return err
		}
		return os.Remove(name)
	})
	m.addOverride("os.RemoveAll", func(name string) error {
		if err := writable("removeall", name); err != nil {
			return err
		}
		return os.RemoveAll(name)
	})
	m.addOverride("os.Rename", func(oldp, newp string) error {
		if err := writable("rename", oldp); err != nil {
			return err
		}
		if err := writable("rename", newp); err != nil {
			return err
		}
		return os.Rename(oldp, newp)
	})
}

// installDialCaps replaces net.Dial and net.DialTimeout with versions that
// only reach the allowed hosts.
func installDialCaps(m *Machine, hosts []string) {
	allowed := map[string]bool{} // "host:port"
	anyPort := map[string]bool{} // "host"
	for _, h := range hosts {
		if _, _, err := net.SplitHostPort(h); err == nil {
			allowed[h] = true
		} else {
			anyPort[h] = true
		}
	}
	ok := func(address string) bool {
		if allowed[address] {
			return true
		}
		host, _, err := net.SplitHostPort(address)
		return err == nil && anyPort[host]
	}
	m.addOverride("net.Dial", func(network, address string) (net.Conn, error) {
		if !ok(address) {
			return nil, &net.OpError{Op: "dial", Net: network, Err: errDenied}
		}
		return net.Dial(network, address)
	})
	m.addOverride("net.DialTimeout", func(network, address string, timeout time.Duration) (net.Conn, error) {
		if !ok(address) {
			return nil, &net.OpError{Op: "dial", Net: network, Err: errDenied}
		}
		return net.DialTimeout(network, address, timeout)
	})
}

type sandboxError string

func (e sandboxError) Error() string { return string(e) }

const errDenied = sandboxError("denied by the plugin sandbox")
