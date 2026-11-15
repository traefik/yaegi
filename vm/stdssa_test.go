package vm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// stdSSAPlugin imports net/http and os, and uses the types of packages that
// it does not import (net/url, io/fs, crypto/tls) in all the ways that need
// their methods: a method call (URL.Query, Values.Get), a conversion to an
// interface (fmt.Stringer, error) and the call through it, a promoted
// method of an embedded field (fs.FileMode through the alias os.FileMode),
// a method value, a method expression, a generic instance that calls the
// method through its constraint, a type switch, and native fmt that calls
// String on a value boxed by the program.
const stdSSAPlugin = `package p

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

type mode struct {
	os.FileMode
	n int
}

func str[T fmt.Stringer](x T) string { return x.String() }

func Name() string {
	r, err := http.NewRequest("GET", "http://example.com/p?a=1&b=two", nil)
	if err != nil {
		return err.Error()
	}
	var out []string
	out = append(out, "a="+r.URL.Query().Get("a"), "b="+r.URL.Query().Get("b"))
	var s fmt.Stringer = r.URL
	out = append(out, s.String())
	m := mode{FileMode: 0o755, n: 1}
	out = append(out, m.String(), fmt.Sprint(m.IsDir()), fmt.Sprint(m.Perm()))
	f := r.URL.Hostname
	g := os.FileMode.String
	out = append(out, f(), g(os.ModeDir|0o700), str(os.FileMode(0o644)))
	var x any = m.FileMode
	switch v := x.(type) {
	case fmt.Stringer:
		out = append(out, "stringer:"+v.String())
	}
	out = append(out, fmt.Sprintf("%v|%s", m.FileMode, r.URL))
	out = append(out, fmt.Sprint(r.TLS == nil))
	_, err = http.NewRequest("GET", "http://[::1", nil)
	var e error = err
	out = append(out, e.Error())
	return strings.Join(out, " ")
}
`

const stdSSAWant = `a=1 b=two http://example.com/p?a=1&b=two -rwxr-xr-x false -rwxr-xr-x example.com drwx------ -rw-r--r-- stringer:-rwxr-xr-x -rwxr-xr-x|http://example.com/p?a=1&b=two true parse "http://[::1": missing ']' in host`

// TestStdSSAIndirect checks the standard library packages that a plugin
// load creates in its SSA program (stdSSA): only those that the source
// imports (and the ones the VM looks up by path), not their whole import
// closure. CreatePackage made an ssa.Function for every function and method
// of each package of the closure, in each plugin's program: 3.2 MB per
// plugin for a small plugin that imports net/http (cmd/loadmany). The
// methods of the types of the packages left out are created by ssa when the
// program needs them: the plugin runs as before, also with concurrent first
// calls (run with -race), in default and AOT mode.
func TestStdSSAIndirect(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, stdSSAPlugin)
	for _, aot := range []bool{false, true} {
		t.Run(fmt.Sprintf("aot=%v", aot), func(t *testing.T) {
			p, err := Load(Options{Dir: dir, AOT: aot}, pkgPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"net/http", "os", "fmt", "strings", "context"} {
				if p.M.prog.ImportedPackage(path) == nil {
					t.Errorf("no SSA package for %s", path)
				}
			}
			for _, path := range []string{"net/url", "io/fs", "crypto/tls", "crypto/x509", "mime/multipart"} {
				if p.M.prog.ImportedPackage(path) != nil {
					t.Errorf("SSA package for %s, which no source package imports", path)
				}
			}
			const n = 16
			got := make([]string, n)
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() {
					res, err := p.Call(context.Background(), "Name")
					if err != nil {
						got[i] = err.Error()
						return
					}
					v, err := res[0].Interface()
					if err != nil {
						got[i] = err.Error()
						return
					}
					got[i] = v.(string)
				})
			}
			wg.Wait()
			for i, g := range got {
				if g != stdSSAWant {
					t.Fatalf("call %d:\n got %s\nwant %s", i, g, stdSSAWant)
				}
			}
		})
	}
}

// TestStdSSALoadCached checks LoadCached on a program without the SSA
// packages of the indirect imports (stdSSA): the cached package may import
// such a package (net/url here, imported by net/http only) and call its
// functions; LoadCached creates the missing SSA package.
func TestStdSSALoadCached(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, "package p\n\nimport \"net/http\"\n\nfunc Name() string { return http.MethodGet }\n")
	p, err := Load(Options{Dir: dir}, pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	if p.M.prog.ImportedPackage("net/url") != nil {
		t.Fatal("SSA package for net/url, which the plugin does not import")
	}
	cdir := t.TempDir()
	src := "package p\n\nimport (\n\t\"net/http\"\n\t\"net/url\"\n)\n\nfunc Name() string { return http.MethodPost + \" \" + url.QueryEscape(\"a b\") }\n"
	if err := os.WriteFile(filepath.Join(cdir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	q, err := p.LoadCached(cdir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := callName(t, q); got != "POST a+b" {
		t.Errorf("LoadCached: Name() = %q, want %q", got, "POST a+b")
	}
	if got := callName(t, p); !strings.EqualFold(got, "GET") {
		t.Errorf("Load: Name() = %q, want GET", got)
	}
}

// stdSSAGrowPlugin gets a *bytes.Buffer (httptest.ResponseRecorder.Body)
// without importing bytes, and calls Grow on it: a static call, a method
// value and a deferred call.
const stdSSAGrowPlugin = `package p

import (
	"fmt"
	"net/http/httptest"
)

func try(f func()) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	f()
	return "ok"
}

func Static(n int) string {
	return try(func() { httptest.NewRecorder().Body.Grow(n) })
}

func Value(n int) string {
	return try(func() {
		grow := httptest.NewRecorder().Body.Grow
		grow(n)
	})
}

func Defer(n int) string {
	return try(func() {
		defer httptest.NewRecorder().Body.Grow(n)
	})
}
`

// TestStdSSANativeMemIndirect: the size check of (*bytes.Buffer).Grow
// (nativemem.go) also runs when the plugin reaches the type without
// importing bytes. stdSSA creates no SSA package for bytes then, and ssa
// creates the method on demand, with no Pkg: the check, keyed by package,
// was skipped, and a Grow of 1 TiB ended the host with a fatal "out of
// memory" (see TestNativeMem). 256 MiB is over the quota, but the host can
// still allocate it if the check fails.
func TestStdSSANativeMemIndirect(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, stdSSAGrowPlugin)
	for _, aot := range []bool{false, true} {
		t.Run(fmt.Sprintf("aot=%v", aot), func(t *testing.T) {
			p, err := Load(Options{Dir: dir, AOT: aot, MaxMemory: 64 << 20}, pkgPath)
			if err != nil {
				t.Fatal(err)
			}
			if p.M.prog.ImportedPackage("bytes") != nil {
				t.Error("SSA package for bytes, which the plugin does not import")
			}
			for _, fn := range []string{"Static", "Value", "Defer"} {
				for _, tt := range []struct {
					n    int
					want string
				}{{1000, "ok"}, {256 << 20, ErrMemory.Error()}} {
					res, err := p.Call(context.Background(), fn, tt.n)
					if err != nil {
						t.Fatalf("%s(%d): %v", fn, tt.n, err)
					}
					v, err := res[0].Interface()
					if err != nil {
						t.Fatalf("%s(%d): %v", fn, tt.n, err)
					}
					if got := v.(string); !strings.Contains(got, tt.want) {
						t.Errorf("%s(%d) with MaxMemory 64 MiB: got %q, want %q", fn, tt.n, got, tt.want)
					}
				}
			}
		})
	}
}
