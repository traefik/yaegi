//go:build linux || darwin

package vm

import (
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// xsysSources returns the .go files of the host's golang.org/x/sys/unix
// (the module cache of yaegi v2's go.mod), by name.
func xsysSources(t *testing.T) map[string]string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "golang.org/x/sys")
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("no module directory of golang.org/x/sys: %v", err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "unix")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Skip(err)
	}
	files := map[string]string{}
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(b)
	}
	return files
}

// writeXSysPlugin writes a plugin module whose vendor directory has
// golang.org/x/sys/unix with the files unixFiles, at the version of
// modules.txt (version, and a replace directive when replace is not empty).
func writeXSysPlugin(t *testing.T, version, replace string, unixFiles map[string]string, plug map[string]string) string {
	t.Helper()
	mods := "# golang.org/x/sys " + version
	gomod := "module example.com/plug\n\ngo 1.24\n\nrequire golang.org/x/sys " + version + "\n"
	if replace != "" {
		mods += " => " + replace
		gomod += "\nreplace golang.org/x/sys => " + replace + "\n"
	}
	mods += "\n## explicit; go 1.24\ngolang.org/x/sys/unix\n"
	if replace != "" {
		mods += "# golang.org/x/sys => " + replace + "\n## explicit\n"
	}
	files := map[string]string{
		"go.mod":             gomod,
		"vendor/modules.txt": mods,
	}
	for name, src := range unixFiles {
		files["vendor/golang.org/x/sys/unix/"+name] = src
	}
	for name, src := range plug {
		files[name] = src
	}
	return writeModule(t, files)
}

// xsysPlug is a plugin that uses golang.org/x/sys/unix: the read-only
// subset (/info, /isatty, /winsize, /keepalive), and functions outside it.
// Each request answers the result, or the panic.
var xsysPlug = map[string]string{
	"req_linux.go":  "package plug\n\nimport \"golang.org/x/sys/unix\"\n\nconst termiosReq, termiosSet = unix.TCGETS, unix.TCSETS\n",
	"req_darwin.go": "package plug\n\nimport \"golang.org/x/sys/unix\"\n\nconst termiosReq, termiosSet = unix.TIOCGETA, unix.TIOCSETA\n",
	"plug.go": `package plug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func try(f func() string) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	return f()
}

// dial connects to a listener of the plugin, and calls f in the Control
// function of the dialer with the socket.
func dial(f func(fd int)) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			c.Close()
		}
	}()
	d := net.Dialer{Control: func(_, _ string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) { f(int(fd)) })
	}}
	if c, err := d.Dial("tcp", ln.Addr().String()); err == nil {
		c.Close()
	}
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, try(func() string {
			switch r.URL.Path {
			case "/info":
				var u unix.Utsname
				uerr := unix.Uname(&u)
				var ts unix.Timespec
				cerr := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
				var ru unix.Rusage
				rerr := unix.Getrusage(unix.RUSAGE_SELF, &ru)
				env, ok := unix.Getenv("XSYS")
				_, home := unix.Getenv("HOME")
				return fmt.Sprintf("pagesize=%v pid=%v uname=%v/%s clock=%v/%v rusage=%v env=%s/%v home=%v",
					unix.Getpagesize() == os.Getpagesize(), unix.Getpid() == os.Getpid(),
					uerr, unix.ByteSliceToString(u.Sysname[:]), cerr, ts.Nano() > 0, rerr, env, ok, home)
			case "/isatty":
				_, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), termiosReq)
				return fmt.Sprint(err)
			case "/winsize":
				_, err := unix.IoctlGetWinsize(int(os.Stderr.Fd()), unix.TIOCGWINSZ)
				return fmt.Sprint(err)
			case "/keepalive":
				var set, get error
				var v int
				dial(func(fd int) {
					set = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_KEEPALIVE, 1)
					v, get = unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_KEEPALIVE)
				})
				return fmt.Sprintf("set=%v get=%v/%v", set, v != 0, get)
			case "/sockopt-outside":
				var sock int
				dial(func(fd int) { sock = fd })
				return fmt.Sprint(unix.SetsockoptInt(sock, unix.SOL_SOCKET, unix.SO_KEEPALIVE, 1))
			case "/sockopt-option":
				var err error
				dial(func(fd int) { err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1) })
				return fmt.Sprint(err)
			case "/recvfrom":
				// in the Control function the socket is not connected yet:
				// the call reaches the system and gives an error
				var err error
				dial(func(fd int) { _, _, err = unix.Recvfrom(fd, make([]byte, 1), unix.MSG_PEEK|unix.MSG_DONTWAIT) })
				return fmt.Sprint(err != nil)
			case "/recvfrom-outside":
				var sock int
				dial(func(fd int) { sock = fd })
				_, _, err := unix.Recvfrom(sock, make([]byte, 1), unix.MSG_PEEK)
				return fmt.Sprint(err)
			case "/recvfrom-flags":
				var err error
				dial(func(fd int) { _, _, err = unix.Recvfrom(fd, make([]byte, 1), unix.MSG_DONTWAIT) })
				return fmt.Sprint(err)
			case "/ioctl-request":
				return fmt.Sprint(unix.IoctlSetTermios(int(os.Stdout.Fd()), termiosSet, &unix.Termios{}))
			case "/ioctl-get-request":
				_, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), termiosSet)
				return fmt.Sprint(err)
			case "/ioctl-fd":
				pr, pw, err := os.Pipe()
				if err != nil {
					return err.Error()
				}
				defer pr.Close()
				defer pw.Close()
				_, err = unix.IoctlGetWinsize(int(pr.Fd()), unix.TIOCGWINSZ)
				return fmt.Sprint(err)
			case "/clock":
				var ts unix.Timespec
				return fmt.Sprint(unix.ClockGettime(-2, &ts))
			case "/mmap":
				_, err := unix.Mmap(-1, 0, unix.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE)
				return fmt.Sprint(err)
			case "/kill":
				return fmt.Sprint(unix.Kill(unix.Getpid(), 0))
			case "/syscall":
				r1, _, _ := unix.Syscall(unix.SYS_GETPID, 0, 0, 0)
				return fmt.Sprint(int(r1) == os.Getpid())
			}
			return "?"
		}))
	}), nil
}
`,
}

// TestXSysSubset checks golang.org/x/sys/unix in a restricted plugin with
// UseUnsafe: the package runs from the host (xsys.go), and its read-only
// subset makes real system calls (xsysunix.go): before, they returned ENOSYS
// with StubAssembly. SO_KEEPALIVE is set and read back on the lent socket,
// as gRPC sets it in its dialer; go-isatty's IoctlGetTermios on os.Stdout
// gives what Go gives. The functions outside the subset are refused at load
// time, and with StubAssembly they return ENOSYS without calling the system
// (or panic, without an error result); the descriptors and requests outside
// the rules panic with a *RefusedError.
func TestXSysSubset(t *testing.T) {
	if !stdtypes.Available() || natPkg(xsysUnixPath) == nil {
		t.Skip("no embedded standard library types or no golang.org/x/sys/unix table")
	}
	dir := writeXSysPlugin(t, "v0.48.0", "", xsysSources(t), xsysPlug)

	_, err := Load(Options{Dir: dir, UseUnsafe: true}, "example.com/plug")
	var pe *PreflightError
	if !errors.As(err, &pe) {
		t.Fatalf("without StubAssembly: %v, want a *PreflightError", err)
	}
	for _, name := range []string{"Mmap", "Kill", "Syscall", "IoctlSetTermios"} {
		want := "golang.org/x/sys/unix." + name + " not in registry: it is not in the read-only golang.org/x/sys/unix subset"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("without StubAssembly: %v\nwant %q", err, want)
		}
	}
	if len(pe.Problems) != 4 {
		t.Errorf("without StubAssembly: problems %q, want 4", pe.Problems)
	}

	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		t.Fatal(err)
	}
	_, ttyErr := unix.IoctlGetTermios(1, termiosReqHost)
	_, wsErr := unix.IoctlGetWinsize(2, unix.TIOCGWINSZ)
	refused := func(s string) string { return s + " is not allowed in a plugin" }
	for _, opts := range []Options{
		{Dir: dir, UseUnsafe: true, StubAssembly: true, Env: []string{"XSYS=1"}},
		{Dir: dir, UseUnsafe: true, StubAssembly: true, Env: []string{"XSYS=1"}, AOT: true},
	} {
		p, err := Load(opts, "example.com/plug")
		if err != nil {
			t.Fatal(err)
		}
		if md := p.M.modeOf(xsysUnixPath); md != ModeNative || len(p.M.NativeFallback) > 0 {
			t.Fatalf("mode %v, fallback %v: want native", md, p.M.NativeFallback)
		}
		for _, c := range []struct{ path, want string }{
			{"/info", fmt.Sprintf("pagesize=true pid=true uname=<nil>/%s clock=<nil>/true rusage=<nil> env=1/true home=false", unix.ByteSliceToString(u.Sysname[:]))},
			{"/isatty", fmt.Sprint(ttyErr)},
			{"/winsize", fmt.Sprint(wsErr)},
			{"/keepalive", "set=<nil> get=true/<nil>"},
			{"/sockopt-outside", refused("unix.SetsockoptInt of a file descriptor that no RawConn callback of a network connection lends to the plugin")},
			{"/sockopt-option", refused(fmt.Sprintf("unix.SetsockoptInt of the socket option %d of level %d (not in the allowed socket options)", unix.SO_REUSEADDR, unix.SOL_SOCKET))},
			{"/recvfrom", "true"},
			{"/recvfrom-outside", refused("unix.Recvfrom of a file descriptor that no RawConn callback of a network connection lends to the plugin")},
			{"/recvfrom-flags", refused(fmt.Sprintf("unix.Recvfrom with the flags %#x (MSG_PEEK, with MSG_DONTWAIT at most)", unix.MSG_DONTWAIT))},
			{"/ioctl-request", "function not implemented"},
			{"/ioctl-get-request", refused(fmt.Sprintf("unix.IoctlGetTermios with the request %#x (only %#x)", uint64(termiosSetHost), uint64(termiosReqHost)))},
			{"/ioctl-fd", refused("unix.IoctlGetWinsize of a file descriptor that is not the plugin's os.Stdin, os.Stdout or os.Stderr and that no RawConn callback lends to the plugin")},
			{"/clock", refused("unix.ClockGettime of the clock -2 (only the clocks of the system, 0 to 31)")},
			{"/mmap", "function not implemented"},
			{"/kill", "function not implemented"},
			{"/syscall", refused("unix.Syscall (outside the read-only golang.org/x/sys/unix subset; Options.StubAssembly loaded the plugin)")},
		} {
			if got := serveOnce(t, opts, "example.com/plug", c.path); got != c.want {
				t.Errorf("AOT %v, %s: %q\nwant %q", opts.AOT, c.path, got, c.want)
			}
		}
	}

	// With the standard library as in Go, the whole package.
	real := Options{Dir: dir, Unrestricted: true, RealProcess: true}
	for path, want := range map[string]string{"/kill": "<nil>", "/syscall": "true", "/keepalive": "set=<nil> get=true/<nil>"} {
		if got := serveOnce(t, real, "example.com/plug", path); got != want {
			t.Errorf("RealProcess, %s: %q, want %q", path, got, want)
		}
	}
}

// TestXSysRestrictions checks what golang.org/x/sys/unix from the host does
// not change: a restricted plugin without UseUnsafe still may not import it,
// and a plugin may not write the variables of the host's copy.
func TestXSysRestrictions(t *testing.T) {
	if !stdtypes.Available() || natPkg(xsysUnixPath) == nil {
		t.Skip("no embedded standard library types or no golang.org/x/sys/unix table")
	}
	src := xsysSources(t)
	dir := writeXSysPlugin(t, "v0.48.0", "", src, map[string]string{"plug.go": `package plug

import "golang.org/x/sys/unix"

func Page() int { return unix.Getpagesize() }
`})
	var ie *ImportError
	if _, err := Load(Options{Dir: dir}, "example.com/plug"); !errors.As(err, &ie) || !ie.NeedsUnsafe {
		t.Errorf("without UseUnsafe: %v, want an import that needs UseUnsafe", err)
	}
	dir = writeXSysPlugin(t, "v0.48.0", "", src, map[string]string{"plug.go": `package plug

import "golang.org/x/sys/unix"

func Set() { unix.Stdout = 7 }
`})
	_, err := Load(Options{Dir: dir, UseUnsafe: true, StubAssembly: true}, "example.com/plug")
	if err == nil || !strings.Contains(err.Error(), "assigning the host variable golang.org/x/sys/unix.Stdout") {
		t.Errorf("write to unix.Stdout: %v, want refused", err)
	}
}

// TestXSysVersions checks when the plugin's golang.org/x/sys/unix runs from
// the host: an older version with the same API does; a newer version, a
// replaced module, or a version whose API the plugin uses differs from the
// host's runs from source as before, and NativeFallback says why.
func TestXSysVersions(t *testing.T) {
	if !stdtypes.Available() || natPkg(xsysUnixPath) == nil {
		t.Skip("no embedded standard library types or no golang.org/x/sys/unix table")
	}
	plug := map[string]string{"plug.go": `package plug

import "golang.org/x/sys/unix"

func Page() int64 { return int64(unix.Getpagesize()) }
`}
	fake := map[string]string{"unix.go": "package unix\n\nfunc Getpagesize() int64 { return 42 }\n"}
	fakeInt := map[string]string{"unix.go": "package unix\n\nfunc Getpagesize() int { return 42 }\n"}
	page := int64(os.Getpagesize())
	for _, c := range []struct {
		name, version, replace string
		files                  map[string]string
		why                    string // "" for native
		want                   int64
	}{
		{"older", "v0.40.0", "", fakeInt, "", page},
		{"pseudo", "v0.0.0-20220715151400-c0bba94af5f8", "", fakeInt, "", page},
		{"newer", "v0.99.0", "", fakeInt, "newer than the host's", 42},
		{"replaced", "v0.48.0", "../fork", fakeInt, "the plugin replaces module golang.org/x/sys", 42},
		{"api", "v0.40.0", "", fake, "func Getpagesize differs", 42},
	} {
		dir := writeXSysPlugin(t, c.version, c.replace, c.files, plug)
		p, err := Load(Options{Dir: dir, UseUnsafe: true}, "example.com/plug")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		why := p.M.NativeFallback[xsysUnixPath]
		if c.why == "" && (why != "" || p.M.modeOf(xsysUnixPath) != ModeNative) || c.why != "" && !strings.Contains(why, c.why) {
			t.Errorf("%s: fallback %q, want %q", c.name, why, c.why)
		}
		res, err := p.Call(t.Context(), "Page")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if v, _ := res[0].Interface(); v != c.want {
			t.Errorf("%s: Page() = %v, want %v", c.name, v, c.want)
		}
	}
}

// TestXSysHostPointers checks that a restricted plugin cannot hand a
// pointer to host memory to a function of the golang.org/x/sys/unix subset
// that writes through its pointer (Getrusage, ClockGettime, Uname): the
// subset runs the host's functions, which write wherever the pointer
// points. A pointer to a host variable (a copy of crc32.IEEETable: the
// load-time check refuses &crc32.IEEETable) seen as a *unix.Rusage or a
// *unix.Timespec through unsafe, unsafe.Add, reflect.NewAt or
// reflect.Value.Pointer, and a view of the plugin's own bytes, are refused
// before the call, and the host variable is unchanged. The vendored copy of
// the package may also have a function that the host copy has not (one that
// calls syscall.Syscall): the package then runs from source as before, where
// syscall.Syscall is refused.
func TestXSysHostPointers(t *testing.T) {
	if !stdtypes.Available() || natPkg(xsysUnixPath) == nil {
		t.Skip("no embedded standard library types or no golang.org/x/sys/unix table")
	}
	plug := map[string]string{"plug.go": `package plug

import (
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"reflect"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func try(f func() string) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	return f()
}

// tbl is a pointer to a host variable, copied (the load-time check refuses
// the address of a host variable).
func tbl() *crc32.Table { t := crc32.IEEETable; return t }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, try(func() string {
			switch r.URL.Path {
			case "/newat":
				p := reflect.NewAt(reflect.TypeOf(unix.Rusage{}), unsafe.Pointer(tbl())).Interface().(*unix.Rusage)
				return fmt.Sprint(unix.Getrusage(unix.RUSAGE_SELF, p), p.Maxrss)
			case "/conv":
				p := (*unix.Rusage)(unsafe.Pointer(tbl()))
				return fmt.Sprint(unix.Getrusage(unix.RUSAGE_SELF, p), p.Maxrss)
			case "/elem":
				p := (*unix.Timespec)(unsafe.Pointer(&tbl()[0]))
				return fmt.Sprint(unix.ClockGettime(0, p), p.Sec)
			case "/reflect":
				p := (*unix.Timespec)(unsafe.Pointer(reflect.ValueOf(tbl()).Pointer()))
				return fmt.Sprint(unix.ClockGettime(0, p), p.Sec)
			case "/add":
				p := (*unix.Timespec)(unsafe.Add(unsafe.Pointer(tbl()), 8))
				return fmt.Sprint(unix.ClockGettime(0, p), p.Sec)
			case "/bytes":
				b := make([]byte, unsafe.Sizeof(unix.Timespec{})-1) // smaller than a Timespec (8 bytes on 32-bit)
				p := (*unix.Timespec)(unsafe.Pointer(&b[0]))
				return fmt.Sprint(unix.ClockGettime(0, p), b)
			case "/uname":
				b := make([]byte, 64) // smaller than a Utsname
				p := (*unix.Utsname)(unsafe.Pointer(&b[0]))
				return fmt.Sprint(unix.Uname(p), b[:8])
			}
			return "?"
		}))
	}), nil
}
`}
	dir := writeXSysPlugin(t, "v0.48.0", "", xsysSources(t), plug)
	opts := Options{Dir: dir, UseUnsafe: true, StubAssembly: true}
	before := *crc32.IEEETable
	for _, c := range []struct{ path, want string }{
		{"/newat", "interface conversion: interface {} is *crc32.Table, not *unix.Rusage"},
		// Both are native types of plain data (nativeview.go), but the
		// table is memory of the host: refused.
		{"/conv", "unsupported: unsafe: viewing a hash/crc32.Table as a golang.org/x/sys/unix.Rusage: the value is memory of the host"},
		{"/elem", "unsupported: unsafe: viewing a uint32 as a golang.org/x/sys/unix.Timespec: the value is memory of the host"},
		{"/reflect", "unsupported: unsafe: viewing a crc32.Table as a golang.org/x/sys/unix.Timespec: the value is memory of the host"},
		{"/add", "unsupported: unsafe.Add on a pointer the VM cannot follow"},
		{"/bytes", "unsupported: unsafe: viewing bytes as a golang.org/x/sys/unix.Timespec reaches outside the memory known to the VM"},
		{"/uname", "unsupported: unsafe: viewing bytes as a golang.org/x/sys/unix.Utsname reaches outside the memory known to the VM"},
	} {
		if got := serveOnce(t, opts, "example.com/plug", c.path); got != c.want {
			t.Errorf("%s: %q\nwant %q", c.path, got, c.want)
		}
		if *crc32.IEEETable != before {
			t.Fatalf("%s: the host's crc32.IEEETable changed", c.path)
		}
	}

	// A function that only the plugin's copy has.
	src := xsysSources(t)
	src["zz_extra.go"] = `package unix

import "syscall"

func Extra() int {
	r1, _, _ := syscall.Syscall(syscall.SYS_GETPID, 0, 0, 0)
	return int(r1)
}
`
	dir = writeXSysPlugin(t, "v0.48.0", "", src, map[string]string{"plug.go": `package plug

import "golang.org/x/sys/unix"

func Pid() int { return unix.Extra() }
`})
	_, err := Load(Options{Dir: dir, UseUnsafe: true}, "example.com/plug")
	if err == nil || !strings.Contains(err.Error(), "zz_extra.go:6:29: unsupported: native func syscall.Syscall not in registry: it is not in the read-only syscall subset") {
		t.Errorf("extra function, without StubAssembly: %v, want syscall.Syscall refused at load time", err)
	}
	p, err := Load(Options{Dir: dir, UseUnsafe: true, StubAssembly: true}, "example.com/plug")
	if err != nil {
		t.Fatal(err)
	}
	if why := p.M.NativeFallback[xsysUnixPath]; !strings.Contains(why, "func Extra is missing") || p.M.modeOf(xsysUnixPath) == ModeNative {
		t.Errorf("extra function: fallback %q, mode %v: want the package from source", why, p.M.modeOf(xsysUnixPath))
	}
	_, err = p.Call(t.Context(), "Pid")
	if err == nil || !strings.Contains(err.Error(), "syscall.Syscall (outside the read-only syscall subset; Options.StubAssembly loaded the plugin) is not allowed in a plugin") {
		t.Errorf("extra function, with StubAssembly: %v, want syscall.Syscall refused", err)
	}
}
