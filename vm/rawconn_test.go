package vm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// TestRawConnSubset checks what a restricted plugin with UseUnsafe can do
// with the file descriptor of a syscall.RawConn (rawconn.go): the RawConn
// type works (testdata/rawconnread.go reads its own socket as gRPC does,
// compared with go run), but the descriptor is only good for syscall.Read
// inside the callback, on its goroutine. Read outside the callback, on
// another goroutine, in the callback of another connection, after a panic
// of the callback, of a host descriptor, or of a descriptor that a RawConn
// of the plugin gives panics; a method value and the reflect of the plugin
// lend as a direct call; a close by another goroutine during the callback
// does not free the number for another connection; a syscall.Conn of the
// plugin given to native code is never called. Close, Write and
// SetsockoptInt are refused at load time, and with StubAssembly they load
// and panic. SetsockoptInt and GetsockoptInt work on the lent socket, in
// the callback, for the allowed options, and panic for another descriptor
// (outside the callback, another goroutine, the host, a file), another
// option or another level. Recvfrom works on the lent socket, in the
// callback, with MSG_PEEK and MSG_DONTWAIT (go-redis), and panics with
// another flag, without MSG_PEEK (the call would consume data), or on
// another descriptor. Fstatfs works on a lent descriptor and panics on
// another one. os.NewFile panics, called by name or through a
// function value. In every case the connection keeps working: nothing was
// closed or written.
func TestRawConnSubset(t *testing.T) {
	if !hasStdTable("syscall") || runtime.GOOS == "windows" {
		t.Skip("no syscall table, or no unix syscall.Read, on this platform")
	}
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/rawconnplug"
	_, err := Load(Options{Dir: "..", UseUnsafe: true}, pkg)
	var pe *PreflightError
	if !errors.As(err, &pe) {
		t.Fatalf("without StubAssembly: %v, want a *PreflightError", err)
	}
	for _, name := range []string{"Close", "Write", "SetsockoptString"} {
		want := "syscall." + name + " not in registry: it is not in the read-only syscall subset"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("without StubAssembly: %v\nwant %q", err, want)
		}
	}
	if len(pe.Problems) != 3 {
		t.Errorf("without StubAssembly: problems %q, want 3", pe.Problems)
	}

	const lent = "syscall.Read of a file descriptor that no RawConn callback lends to the plugin is not allowed in a plugin"
	stub := func(name string) string {
		return "syscall." + name + " (outside the read-only syscall subset; Options.StubAssembly loaded the plugin) is not allowed in a plugin"
	}
	notSock := func(name string) string {
		return "syscall." + name + " of a file descriptor that no RawConn callback of a network connection lends to the plugin is not allowed in a plugin"
	}
	badOpt := func(name string, level, opt int) string {
		return fmt.Sprintf("syscall.%s of the socket option %d of level %d (not in the allowed socket options) is not allowed in a plugin", name, opt, level)
	}
	badFlags := func(flags int) string {
		return fmt.Sprintf("syscall.Recvfrom with the flags %#x (MSG_PEEK, with MSG_DONTWAIT at most) is not allowed in a plugin", flags)
	}
	const newFile = "os.NewFile (a file of any descriptor number) is not allowed in a plugin"
	for _, opts := range []Options{
		{Dir: "..", UseUnsafe: true, StubAssembly: true},
		{Dir: "..", UseUnsafe: true, StubAssembly: true, AOT: true},
	} {
		for _, c := range []struct{ path, want string }{
			{"/outside", lent},
			{"/host", lent},
			{"/goroutine", lent},
			{"/other", lent},
			{"/panic", lent},
			{"/own", lent},
			{"/methodvalue", "ok"},
			{"/reflect", "ok"},
			{"/closed", "reuse=false new=" + lent},
			{"/ownconn", "n=5 err=<nil> called=0 echo=hello"},
			{"/close", stub("Close")},
			{"/write", stub("Write")},
			{"/setsockopt", stub("SetsockoptString")},
			{"/sockopt", "ok[<nil>/false/<nil> <nil>/true/<nil> <nil>/true/<nil>]"},
			{"/sockopt-outside", notSock("SetsockoptInt")},
			{"/getsockopt-outside", notSock("GetsockoptInt")},
			{"/sockopt-goroutine", notSock("SetsockoptInt")},
			{"/sockopt-host", notSock("SetsockoptInt")},
			{"/sockopt-other", notSock("SetsockoptInt")},
			{"/sockopt-file", notSock("SetsockoptInt")},
			{"/sockopt-option", badOpt("SetsockoptInt", syscall.SOL_SOCKET, syscall.SO_REUSEADDR)},
			{"/getsockopt-option", badOpt("GetsockoptInt", syscall.SOL_SOCKET, syscall.SO_BROADCAST)},
			{"/sockopt-level", badOpt("SetsockoptInt", syscall.IPPROTO_IP, syscall.TCP_NODELAY)},
			{"/recvfrom", "ok eagain=true"},
			{"/recvfrom-flags", badFlags(peekWaitall)},
			{"/recvfrom-nopeek", badFlags(0)},
			{"/recvfrom-outside", notSock("Recvfrom")},
			{"/recvfrom-goroutine", notSock("Recvfrom")},
			{"/recvfrom-host", notSock("Recvfrom")},
			{"/recvfrom-file", notSock("Recvfrom")},
			{"/fstatfs", "ok err=<nil> filled=true"},
			{"/fstatfs-host", "syscall.Fstatfs of a file descriptor that no RawConn callback lends to the plugin is not allowed in a plugin"},
			{"/fstatfs-outside", "syscall.Fstatfs of a file descriptor that no RawConn callback lends to the plugin is not allowed in a plugin"},
			{"/newfile", newFile},
			{"/newfile-value", newFile},
		} {
			want := c.want + ` | conn="pongping" <nil>`
			if got := serveOnce(t, opts, pkg, c.path); got != want {
				t.Errorf("AOT %v, %s: %q\nwant %q", opts.AOT, c.path, got, want)
			}
		}
	}
}

// TestStubRawSyscall checks the functions of golang.org/x/sys/unix written
// in assembly that make a system call directly (linux and the other unix
// systems but darwin): with StubAssembly, Syscall6 returns ENOSYS without
// calling the system, as the calls through the C library on darwin. gRPC
// sets SO_KEEPALIVE with unix.SetsockoptInt in the Control function of its
// dialer and ignores the error: on linux the dial failed with a panic. A
// function without an error result (SyscallNoError) stays a stub that
// panics. The vendored package here has the declarations of x/sys/unix, at
// a version newer than the host's: the VM runs it from source instead of
// using the host's copy (xsys.go, TestXSysVersions).
func TestStubRawSyscall(t *testing.T) {
	if !hasStdTable("syscall") || !stdtypes.Available() {
		t.Skip("no syscall table or no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":             "module example.com/plug\n\ngo 1.22\n\nrequire golang.org/x/sys v0.99.0\n",
		"vendor/modules.txt": "# golang.org/x/sys v0.99.0\n## explicit; go 1.20\ngolang.org/x/sys/unix\n",
		"vendor/golang.org/x/sys/unix/syscall.go": `package unix

import "syscall"

func Syscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)

func SyscallNoError(trap, a1, a2, a3 uintptr) (r1, r2 uintptr)

func SetsockoptInt(fd, level, opt, value int) error {
	if _, _, e := Syscall6(54, uintptr(fd), uintptr(level), uintptr(opt), 0, 4, 0); e != 0 {
		return e
	}
	return nil
}

func Getpid() int {
	r, _ := SyscallNoError(172, 0, 0, 0)
	return int(r)
}
`,
		"plug.go": `package plug

import (
	"context"
	"fmt"
	"net/http"
	"syscall"

	"golang.org/x/sys/unix"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/noerror" {
			defer func() { fmt.Fprint(w, recover() != nil) }()
			unix.Getpid()
			return
		}
		err := unix.SetsockoptInt(3, 1, 9, 1)
		fmt.Fprint(w, err == syscall.ENOSYS)
	}), nil
}
`,
	})
	_, err := Load(Options{Dir: dir, UseUnsafe: true}, "example.com/plug")
	if err == nil || !strings.Contains(err.Error(), "unix.Syscall6 not in registry") {
		t.Fatalf("without StubAssembly: %v, want unix.Syscall6 refused", err)
	}
	for _, aot := range []bool{false, true} {
		opts := Options{Dir: dir, UseUnsafe: true, StubAssembly: true, AOT: aot}
		if got := serveOnce(t, opts, "example.com/plug", "/"); got != "true" {
			t.Errorf("AOT %v: SetsockoptInt gives ENOSYS: %s, want true", aot, got)
		}
		if got := serveOnce(t, opts, "example.com/plug", "/noerror"); got != "true" {
			t.Errorf("AOT %v: SyscallNoError panics: %s, want true", aot, got)
		}
	}
}

// TestSandboxStatfs checks syscall.Statfs of the subset of a restricted
// plugin with UseUnsafe (unsafesyscall.go) under Options.Sandbox: a path
// in ReadPaths gives the statistics of its file system, another path is
// denied, as os.Stat (sandbox.go, statfs_unix.go).
func TestSandboxStatfs(t *testing.T) {
	if !hasStdTable("syscall") || !stdtypes.Available() {
		t.Skip("no syscall table or no embedded standard library types")
	}
	switch runtime.GOOS {
	case "windows", "solaris", "illumos", "netbsd":
		t.Skip("no syscall.Statfs on " + runtime.GOOS)
	}
	in := t.TempDir()
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/plug\n\ngo 1.22\n",
		"plug.go": `package plug

import (
	"context"
	"fmt"
	"net/http"
	"syscall"
)

type Config struct{ In, Out string }

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, c *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var st syscall.Statfs_t
		in := syscall.Statfs(c.In, &st)
		filled := st != syscall.Statfs_t{}
		out := syscall.Statfs(c.Out, &st)
		fmt.Fprintf(w, "in=%v %v out=%v", in, filled, out)
	}), nil
}
`,
	})
	opts := Options{Dir: dir, UseUnsafe: true, Sandbox: &Sandbox{ReadPaths: []string{in}}}
	p, err := Load(opts, "example.com/plug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Dir(in)
	if err := cfg.SetFields(map[string]any{"In": in, "Out": out}); err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "statfs")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	want := "in=<nil> true out=statfs " + out + ": " + errDenied.Error()
	if got := w.Body.String(); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
