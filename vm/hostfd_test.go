//go:build linux || darwin || freebsd || netbsd || openbsd

package vm

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestHostFD checks that a restricted plugin cannot open a descriptor of
// the host process again through the file system (hostfd.go): /dev/fd/N
// on darwin and the BSDs, /proc/self/fd/N and /proc/self/mem on linux,
// directly, through a symbolic link, os.DirFS, os.Root and http.Dir. A
// regular file still opens as in Go, and the descriptor of the host keeps
// working after each try.
func TestHostFD(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostfdplug"
	tmp := t.TempDir()
	regular := filepath.Join(tmp, "regular")
	if err := os.WriteFile(regular, []byte("host data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(regular, filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	// The host descriptor is the read end of a pipe: it has no path that
	// could be opened again by name.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()
	fd := strconv.Itoa(int(pr.Fd()))
	// A second host descriptor is a regular file with a secret, removed:
	// no path leads to it, and a handler that serves it has read it.
	secret, err := os.CreateTemp(tmp, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer secret.Close()
	if _, err := secret.WriteString("secret data"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(secret.Name()); err != nil {
		t.Fatal(err)
	}
	sfd := strconv.Itoa(int(secret.Fd()))

	type tc struct {
		name string
		q    url.Values
		want string // "ok ..." exact, "refused: <text>" contained, or "error"
	}
	q := func(kv ...string) url.Values {
		v := url.Values{}
		for i := 0; i < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}
	cases := []tc{
		{"regular", q("op", "open", "path", regular), `ok "host data"`},
		{"link to regular", q("op", "open", "path", filepath.Join(tmp, "link")), `ok "host data"`},
		{"dirfs regular", q("op", "dirfs", "dir", tmp, "name", "regular"), `ok "host data"`},
		{"root regular", q("op", "root", "dir", tmp, "name", "regular"), `ok "host data"`},
		{"httpdir regular", q("op", "httpdir", "dir", tmp, "name", "/regular"), `ok "host data"`},
		{"ifacedir regular", q("op", "ifacedir", "dir", tmp, "name", "/regular"), `ok "host data"`},
		{"fileserver regular", q("op", "fileserver", "dir", tmp, "name", "/regular"), `ok "host data"`},
		{"embeddir regular", q("op", "embeddir", "dir", tmp, "name", "/regular"), `ok "host data"`},
		{"servefile regular", q("op", "servefile", "path", regular), `ok "host data"`},
		{"filetransport regular", q("op", "filetransport", "dir", tmp, "name", "/regular"), `ok "host data"`},
		{"openinroot regular", q("op", "openinroot", "dir", tmp, "name", "regular"), `ok "host data"`},
		{"filelistener", q("op", "filelistener", "path", regular), "refused: " + errNetFile.Error()},
		{"fileconn", q("op", "fileconn"), "refused: " + errNetFile.Error()},
	}
	var fdLink, fdDir string
	switch runtime.GOOS {
	case "linux":
		fdLink = "/proc/self/fd/" + fd
		fdDir = "/proc/self/fd"
		pid := strconv.Itoa(os.Getpid())
		cases = append(cases,
			// The magic link is read as text: nothing is opened.
			tc{"proc self fd", q("op", "open", "path", fdLink), "error"},
			tc{"proc pid fd", q("op", "open", "path", "/proc/"+pid+"/fd/"+fd), "error"},
			tc{"proc thread-self fd", q("op", "open", "path", "/proc/thread-self/fd/"+fd), "error"},
			tc{"proc self mem", q("op", "openfile", "path", "/proc/self/mem"), "refused: " + errProcMem.Error()},
			tc{"proc pid mem", q("op", "openfile", "path", "/proc/"+pid+"/mem"), "refused: " + errProcMem.Error()},
			tc{"root proc", q("op", "root", "dir", "/proc", "name", "self/mem"), "refused: " + errFDRoot.Error()},
			// A directory of the procfs that is not /proc: found by the
			// type of the file system of the root's own descriptor.
			tc{"root proc self", q("op", "root", "dir", "/proc/self", "name", "mem"), "refused: " + errFDRoot.Error()},
			tc{"root proc self fd", q("op", "root", "dir", "/proc/self/fd", "name", fd), "refused: " + errFDRoot.Error()},
			tc{"root slash", q("op", "root", "dir", "/", "name", "proc/self/mem"), "refused: " + errFDRoot.Error()},
		)
		if err := os.Symlink("/proc/self/mem", filepath.Join(tmp, "memlink")); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, tc{"link to proc mem", q("op", "openfile", "path", filepath.Join(tmp, "memlink")), "refused: " + errProcMem.Error()})
	default:
		fdLink = "/dev/fd/" + fd
		fdDir = "/dev/fd"
		cases = append(cases,
			tc{"dev fd", q("op", "open", "path", fdLink), "refused: " + errFDEntry.Error()},
			tc{"dev stdin", q("op", "open", "path", "/dev/stdin"), "refused: " + errFDEntry.Error()},
			tc{"dirfs dev fd", q("op", "dirfs", "dir", "/dev/fd", "name", fd), "refused: " + errFDEntry.Error()},
			// http.Dir maps the refusal to fs.ErrNotExist, as it does for
			// any error of os.Open: nothing is opened.
			tc{"httpdir dev fd", q("op", "httpdir", "dir", "/dev/fd", "name", "/"+fd), "error"},
			tc{"root dev", q("op", "root", "dir", "/dev", "name", "fd/"+fd), "refused: " + errFDRoot.Error()},
			tc{"root dev fd", q("op", "root", "dir", "/dev/fd", "name", fd), "refused: " + errFDRoot.Error()},
		)
	}
	if err := os.Symlink(fdLink, filepath.Join(tmp, "fdlink")); err != nil {
		t.Fatal(err)
	}
	linkWant := "error"
	if runtime.GOOS != "linux" {
		linkWant = "refused: " + errFDEntry.Error()
	}
	cases = append(cases, tc{"link to fd", q("op", "open", "path", filepath.Join(tmp, "fdlink")), linkWant})
	// The secret file: none of the ways to a file may serve it.
	secretLink := filepath.Join(tmp, "secretlink")
	if err := os.Symlink(fdDir+"/"+sfd, secretLink); err != nil {
		t.Fatal(err)
	}
	cases = append(cases,
		tc{"secret open", q("op", "open", "path", fdDir+"/"+sfd), "error"},
		tc{"secret readfile", q("op", "readfile", "path", secretLink), "error"},
		tc{"secret dirfs", q("op", "dirfs", "dir", tmp, "name", "secretlink"), "error"},
		tc{"secret ifacedir", q("op", "ifacedir", "dir", fdDir, "name", "/"+sfd), "error"},
		tc{"secret fileserver", q("op", "fileserver", "dir", fdDir, "name", "/"+sfd), "error"},
		tc{"secret fileserver link", q("op", "fileserver", "dir", tmp, "name", "/secretlink"), "error"},
		tc{"secret embeddir", q("op", "embeddir", "dir", fdDir, "name", "/"+sfd), "error"},
		tc{"secret servefile", q("op", "servefile", "path", secretLink), "error"},
		tc{"secret filetransport", q("op", "filetransport", "dir", fdDir, "name", "/"+sfd), "error"},
		tc{"secret openinroot", q("op", "openinroot", "dir", tmp, "name", "secretlink"), "error"},
	)

	check := func(t *testing.T, got, want string) {
		t.Helper()
		switch {
		case want == "error":
			if strings.HasPrefix(got, "ok") || strings.Contains(got, "secret data") {
				t.Errorf("got %q, want an error", got)
			}
		case strings.HasPrefix(want, "refused: "):
			if !strings.Contains(got, strings.TrimPrefix(want, "refused: ")) {
				t.Errorf("got %q, want %q", got, want)
			}
		default:
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		}
	}
	for _, opts := range []Options{{Dir: ".."}, {Dir: "..", AOT: true}} {
		for _, c := range cases {
			t.Run(fmt.Sprintf("aot=%v/%s", opts.AOT, c.name), func(t *testing.T) {
				check(t, serveOnce(t, opts, pkg, "/?"+c.q.Encode()), c.want)
				// The host descriptor still works.
				if _, err := pw.Write([]byte("x")); err != nil {
					t.Fatal(err)
				}
				b := make([]byte, 1)
				if _, err := pr.Read(b); err != nil || b[0] != 'x' {
					t.Fatalf("host pipe after the try: %q, %v", b, err)
				}
			})
		}
	}
	if b, err := os.ReadFile(regular); err != nil || string(b) != "host data" {
		t.Fatalf("host file: %q, %v", b, err)
	}

	// A Sandbox refuses the same paths.
	sb := Options{Dir: "..", Sandbox: &Sandbox{}}
	for _, c := range cases {
		if strings.HasPrefix(c.want, "ok") {
			continue
		}
		if got := serveOnce(t, sb, pkg, "/?"+c.q.Encode()); strings.HasPrefix(got, "ok") {
			t.Errorf("sandbox, %s: %q, want an error", c.name, got)
		}
	}

	// An unrestricted plugin keeps the functions of Go.
	if got := serveOnce(t, Options{Dir: "..", Unrestricted: true}, pkg, "/?"+q("op", "open", "path", regular).Encode()); got != `ok "host data"` {
		t.Errorf("unrestricted regular: %q", got)
	}
	if runtime.GOOS != "linux" {
		// So /dev/fd/N serves the secret there: the cases above would see
		// it.
		if got := serveOnce(t, Options{Dir: "..", Unrestricted: true}, pkg, "/?"+q("op", "fileserver", "dir", fdDir, "name", "/"+sfd).Encode()); got != `ok "secret data"` {
			t.Errorf("unrestricted secret: %q", got)
		}
	}
}

// TestHostFDStdPaths checks that a restricted plugin opens /dev/stdout,
// /dev/stderr and the entries 1 and 2 of the descriptor directory, as in Go:
// a logging plugin with "file: /dev/stdout". It gets
// its own duplicate of the descriptor, so closing it leaves the standard
// output and error of the host working. The descriptors 1 and 2 of the
// test are pipes here, to read what the plugin wrote.
func TestHostFDStdPaths(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostfdplug"
	paths := map[int][]string{1: {"/dev/stdout"}, 2: {"/dev/stderr"}}
	switch runtime.GOOS {
	case "linux":
		pid := strconv.Itoa(os.Getpid())
		paths[1] = append(paths[1], "/proc/self/fd/1", "/proc/"+pid+"/fd/1", "/dev/fd/1")
		paths[2] = append(paths[2], "/proc/self/fd/2", "/proc/"+pid+"/fd/2", "/dev/fd/2")
	default:
		paths[1] = append(paths[1], "/dev/fd/1")
		paths[2] = append(paths[2], "/dev/fd/2")
	}
	type plug struct {
		name string
		opts Options
	}
	plugs := []plug{{"vm", Options{Dir: ".."}}, {"aot", Options{Dir: "..", AOT: true}}}

	var readers [3]*os.File
	var saved [3]int
	for _, n := range []int{1, 2} {
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer pr.Close()
		readers[n] = pr
		if saved[n], err = unix.Dup(n); err != nil {
			t.Fatal(err)
		}
		if err := dupTo(int(pw.Fd()), n); err != nil {
			t.Fatal(err)
		}
		pw.Close()
	}
	restore := func() {
		for _, n := range []int{1, 2} {
			if saved[n] > 0 {
				dupTo(saved[n], n)
				unix.Close(saved[n])
				saved[n] = 0
			}
		}
	}
	defer restore()

	var got []string
	for _, p := range plugs {
		for _, n := range []int{1, 2} {
			for _, path := range paths[n] {
				got = append(got, fmt.Sprintf("%s %s: %s", p.name, path, serveOnce(t, p.opts, pkg, "/?"+url.Values{"op": {"writestd"}, "path": {path}}.Encode())))
			}
		}
	}
	// The host descriptors still work after the plugin closed its files.
	for _, n := range []int{1, 2} {
		if _, err := unix.Write(n, []byte("host\n")); err != nil {
			got = append(got, fmt.Sprintf("host write to %d: %v", n, err))
		}
	}
	restore()
	for _, s := range got {
		if !strings.HasSuffix(s, ": ok <nil> <nil>") {
			t.Errorf("%s, want ok <nil> <nil>", s)
		}
	}
	for _, n := range []int{1, 2} {
		var want strings.Builder
		for range plugs {
			for _, path := range paths[n] {
				fmt.Fprintf(&want, "line of %s\n", path)
			}
		}
		want.WriteString("host\n")
		// The machines may still hold their own copy of the descriptor (their
		// os.Stdout): read what is expected, or until the deadline.
		readers[n].SetReadDeadline(time.Now().Add(5 * time.Second))
		b := make([]byte, 0, want.Len())
		for len(b) < want.Len() {
			k, err := readers[n].Read(b[len(b):cap(b)])
			b = b[:len(b)+k]
			if err != nil {
				break
			}
		}
		if string(b) != want.String() {
			t.Errorf("descriptor %d got:\n%s\nwant:\n%s", n, b, want.String())
		}
	}
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		if _, err := f.Stat(); err != nil {
			t.Fatalf("host %s after the plugin closed its copy: %v", f.Name(), err)
		}
	}
}

// TestHostFDStdPathsWriters checks that with Options.Stdout and
// Options.Stderr set, /dev/stdout and /dev/stderr (and the entries 1 and 2
// of the descriptor directory) of a restricted plugin lead to these
// writers, as its os.Stdout and os.Stderr do, and not to the descriptors 1
// and 2 of the host behind them: the host set the writers to get every
// line of the plugin. The descriptors 1 and 2 of the test are pipes here,
// to see that nothing reaches them.
func TestHostFDStdPathsWriters(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostfdplug"
	paths := map[int][]string{1: {"/dev/stdout", "/dev/fd/1"}, 2: {"/dev/stderr", "/dev/fd/2"}}
	if runtime.GOOS == "linux" {
		paths[1] = append(paths[1], "/proc/self/fd/1")
		paths[2] = append(paths[2], "/proc/self/fd/2")
	}

	var readers [3]*os.File
	var saved [3]int
	for _, n := range []int{1, 2} {
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer pr.Close()
		readers[n] = pr
		if saved[n], err = unix.Dup(n); err != nil {
			t.Fatal(err)
		}
		if err := dupTo(int(pw.Fd()), n); err != nil {
			t.Fatal(err)
		}
		pw.Close()
	}
	restore := func() {
		for _, n := range []int{1, 2} {
			if saved[n] > 0 {
				dupTo(saved[n], n)
				unix.Close(saved[n])
				saved[n] = 0
			}
		}
	}
	defer restore()

	var got []string
	outs := map[bool][2]*syncBuffer{}
	for _, aot := range []bool{false, true} {
		out, errOut := new(syncBuffer), new(syncBuffer)
		outs[aot] = [2]*syncBuffer{out, errOut}
		opts := Options{Dir: "..", AOT: aot, Stdout: out, Stderr: errOut}
		for _, n := range []int{1, 2} {
			for _, path := range paths[n] {
				got = append(got, fmt.Sprintf("aot %v %s: %s", aot, path, serveOnce(t, opts, pkg, "/?"+url.Values{"op": {"writestd"}, "path": {path}}.Encode())))
			}
		}
	}
	for _, n := range []int{1, 2} {
		unix.Write(n, []byte("host\n"))
	}
	restore()
	for _, s := range got {
		if !strings.HasSuffix(s, ": ok <nil> <nil>") {
			t.Errorf("%s, want ok <nil> <nil>", s)
		}
	}
	for aot, w := range outs {
		for _, n := range []int{1, 2} {
			var want []string
			for _, path := range paths[n] {
				want = append(want, fmt.Sprintf("line of %s\n", path))
			}
			slices.Sort(want)
			if lines := w[n-1].lines(len(want)); !slices.Equal(lines, want) {
				t.Errorf("aot %v: writer %d got %q, want %q", aot, n, lines, want)
			}
		}
	}
	// The host descriptors got nothing but the line of the test.
	for _, n := range []int{1, 2} {
		readers[n].SetReadDeadline(time.Now().Add(5 * time.Second))
		b, _ := io.ReadAll(readers[n])
		if string(b) != "host\n" {
			t.Errorf("descriptor %d of the host got %q, want only the line of the test", n, b)
		}
	}
}

// TestHostFDStdio checks that a restricted plugin that closes os.Stdin,
// os.Stdout and os.Stderr closes its own copies: the files of the host
// still work.
func TestHostFDStdio(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostfdplug"
	for _, aot := range []bool{false, true} {
		got := serveOnce(t, Options{Dir: "..", AOT: aot}, pkg, "/?op=closestdio")
		if got != "<nil> <nil> <nil>" {
			t.Errorf("aot %v: close: %q", aot, got)
		}
		for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
			if _, err := f.Stat(); err != nil {
				t.Fatalf("aot %v: host %s after the plugin closed its copy: %v", aot, f.Name(), err)
			}
		}
	}
}
