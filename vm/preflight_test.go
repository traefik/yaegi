package vm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// writeModule writes the files of a plugin module (path -> content) in a
// new directory and returns it.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// serveOnce loads the plugin, builds its handler and serves one request.
func serveOnce(t *testing.T, opts Options, pkg, path string) string {
	t.Helper()
	p, err := Load(opts, pkg)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Body.String()
}

const preflightGoMod = "module example.com/plug\n\ngo 1.22\n\nrequire example.com/fast v1.0.0\n"

// TestPurego checks that the loaders (toolchain-free and go list) select the pure Go
// version of a vendored package that also has one in assembly (the purego
// build tag, as golang.org/x/crypto, x/sys/cpu, cespare/xxhash and
// klauspost/compress use it). Without the tag, the loader took the file
// with the bodyless declaration, and ServeHTTP panicked with "native func
// example.com/fast.Sum not in registry".
func TestPurego(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":             preflightGoMod,
		"vendor/modules.txt": "# example.com/fast v1.0.0\n## explicit; go 1.20\nexample.com/fast\n",
		"vendor/example.com/fast/sum_asm.go": `//go:build (amd64 || arm64) && gc && !purego

package fast

// Sum is written in assembly.
func Sum(b []byte) int
`,
		"vendor/example.com/fast/sum_amd64.s": "// assembly\n",
		"vendor/example.com/fast/sum_arm64.s": "// assembly\n",
		"vendor/example.com/fast/sum_other.go": `//go:build !(amd64 || arm64) || !gc || purego

package fast

func Sum(b []byte) int {
	n := 0
	for _, c := range b {
		n += int(c)
	}
	return n
}
`,
		"plug.go": `package plug

import (
	"context"
	"fmt"
	"net/http"

	"example.com/fast"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, fast.Sum([]byte("abc")))
	}), nil
}
`,
	})
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	for _, goList := range []bool{false, true} {
		if got := serveOnce(t, Options{Dir: dir, GoList: goList}, "example.com/plug", "/"); got != "294" {
			t.Errorf("GoList %v: body %q, want 294", goList, got)
		}
	}
}

// TestNoasm checks that the loaders also set the noasm tag
// for a package with assembly. klauspost/compress (s2, zstd, huff0) selects
// its pure Go version with noasm, not purego: before, s2 failed with
// "s2Decode has no Go body".
func TestNoasm(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":             preflightGoMod,
		"vendor/modules.txt": "# example.com/fast v1.0.0\n## explicit; go 1.20\nexample.com/fast\n",
		"vendor/example.com/fast/decode_asm.go": `//go:build (amd64 || arm64) && !appengine && !noasm && gc

package fast

// Sum is written in assembly.
func Sum(b []byte) int
`,
		"vendor/example.com/fast/decode_amd64.s": "// assembly\n",
		"vendor/example.com/fast/decode_arm64.s": "// assembly\n",
		"vendor/example.com/fast/decode_other.go": `//go:build (!amd64 && !arm64) || appengine || !gc || noasm

package fast

func Sum(b []byte) int {
	n := 0
	for _, c := range b {
		n += int(c)
	}
	return n
}
`,
		"plug.go": `package plug

import (
	"context"
	"fmt"
	"net/http"

	"example.com/fast"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, fast.Sum([]byte("abc")))
	}), nil
}
`,
	})
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	for _, goList := range []bool{false, true} {
		if got := serveOnce(t, Options{Dir: dir, GoList: goList}, "example.com/plug", "/"); got != "294" {
			t.Errorf("GoList %v: body %q, want 294", goList, got)
		}
	}
}

// TestPuregoOnlyForAssembly checks that the purego tag is added only to a
// package that needs it (assembly or bodyless functions). A vendored package
// with only Go files and !purego / purego variants keeps the files of go run
// in both loaders (and in the AOT copies): "fast". Before, the
// toolchain-free loader set purego for every package and ran "generic".
func TestPuregoOnlyForAssembly(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":             preflightGoMod,
		"vendor/modules.txt": "# example.com/fast v1.0.0\n## explicit; go 1.20\nexample.com/fast\n",
		"vendor/example.com/fast/a_fast.go": `//go:build !purego

package fast

func Kind() string { return "fast" }
`,
		"vendor/example.com/fast/a_generic.go": `//go:build purego

package fast

func Kind() string { return "generic" }
`,
		"plug.go": `package plug

import (
	"context"
	"fmt"
	"net/http"

	"example.com/fast"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, fast.Kind())
	}), nil
}
`,
	})
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	for _, goList := range []bool{false, true} {
		if got := serveOnce(t, Options{Dir: dir, GoList: goList}, "example.com/plug", "/"); got != "fast" {
			t.Errorf("GoList %v: body %q, want fast (the files of go run)", goList, got)
		}
	}
}

// preflightPlugin calls syscall (Chdir is not in the read-only subset of a
// plugin with useUnsafe, unsafesyscall.go) in a method that
// is only reached through an interface, in a branch that no request of the
// test takes. Code under a constant false condition never runs, and is not
// reported.
const preflightPlugin = `package plug

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"syscall"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type pid struct{}

func (pid) String() string { return fmt.Sprint(syscall.Getpid(), syscall.Chdir("/")) }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	var s fmt.Stringer = pid{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pid" {
			fmt.Fprint(w, s.String())
		}
		if runtime.GOOS == "plan9" {
			syscall.Exit(1)
		}
		fmt.Fprint(w, "ok")
	}), nil
}
`

// preflightExec calls os/exec (no symbols without RealProcess) and syscall
// in a branch that runs only for some requests.
const preflightExec = `package execp

import (
	"context"
	"net/http"
	"os/exec"
	"syscall"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/exec" {
			exec.Command("true").Run()
			syscall.Getppid() // in syscall on every GOOS, Windows too
		}
	}), nil
}
`

// TestPreflight checks the load-time check: code the VM cannot run fails
// the load with one error that lists every problem with its position, also
// in branches that never run at load time. Before, the load and the first
// requests worked, and the request that took the branch panicked with "not
// in registry".
func TestPreflight(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":         "module example.com/plug\n\ngo 1.22\n",
		"plug.go":        preflightPlugin,
		"execp/execp.go": preflightExec,
	})
	const noSyscall = "unsupported: native func syscall.%s not in registry: yaegi v2 gives the syscall symbols only with Options.Unrestricted and RealProcess"
	const notInSubset = "unsupported: native func syscall.%s not in registry: it is not in the read-only syscall subset of a plugin with Options.UseUnsafe"
	for _, c := range []struct {
		opts Options
		pkg  string
		want []string
	}{
		{Options{Dir: dir, UseUnsafe: true}, "example.com/plug", []string{"plug.go:17:79: " + fmt.Sprintf(notInSubset, "Chdir")}},
		{Options{Dir: dir, UseUnsafe: true, GoList: true}, "example.com/plug", []string{"plug.go:17:79: " + fmt.Sprintf(notInSubset, "Chdir")}},
		{Options{Dir: dir, Unrestricted: true}, "example.com/plug/execp", []string{
			"execp.go:17:16: unsupported: native func os/exec.Command not in registry: yaegi v2 gives the os/exec symbols only with Options.Exec, or Options.Unrestricted and RealProcess (in example.com/plug/execp.New$1)",
			"execp.go:18:19: " + fmt.Sprintf(noSyscall, "Getppid"),
		}},
	} {
		name := fmt.Sprintf("%s (Unrestricted %v, GoList %v)", c.pkg, c.opts.Unrestricted, c.opts.GoList)
		_, err := Load(c.opts, c.pkg)
		var pe *PreflightError
		if !errors.As(err, &pe) {
			t.Errorf("%s: error %v, want a *PreflightError", name, err)
			continue
		}
		msg := err.Error()
		for _, want := range c.want {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: error\n%s\nwant %q", name, msg, want)
			}
		}
		if len(pe.Problems) != len(c.want) || strings.Contains(msg, "syscall.Exit") {
			t.Errorf("%s: problems %q, want %d (not the code under a constant false condition)", name, pe.Problems, len(c.want))
		}
	}

	// With NoPreflight, the problems only show when the code runs.
	opts := Options{Dir: dir, UseUnsafe: true, NoPreflight: true}
	if got := serveOnce(t, opts, "example.com/plug", "/"); got != "ok" {
		t.Errorf("NoPreflight: body %q, want ok", got)
	}
	func() {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(errString(r), "syscall.Chdir not in registry") {
				t.Errorf("NoPreflight, /pid: panic %v, want syscall.Chdir not in registry", r)
			}
		}()
		serveOnce(t, opts, "example.com/plug", "/pid")
	}()

	// The real standard library has the symbols: no problem.
	if _, err := Load(Options{Dir: dir, Unrestricted: true, RealProcess: true}, "example.com/plug"); err != nil && hasStdTable("syscall") {
		t.Errorf("RealProcess: %v", err)
	}
}

// TestPreflightFieldMethods checks that the check walks the methods of the
// types inside a type converted to an interface: reflect reaches
// fast.Inner.MarshalJSON from outer{} with Field(0).Interface(). Before, only
// the methods of outer were walked: the load passed, and the request
// panicked with "native func syscall.Getpid not in registry" (Getpid is now
// in the syscall subset of useUnsafe: the test uses Chdir).
func TestPreflightFieldMethods(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":             preflightGoMod,
		"vendor/modules.txt": "# example.com/fast v1.0.0\n## explicit; go 1.20\nexample.com/fast\n",
		"vendor/example.com/fast/inner.go": `package fast

import (
	"fmt"
	"syscall"
)

type Inner struct{}

func (Inner) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprint(syscall.Chdir("/"))), nil }
`,
		"plug.go": `package plug

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"

	"example.com/fast"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type outer struct{ X fast.Inner }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := reflect.ValueOf(outer{}).Field(0).Interface().(json.Marshaler).MarshalJSON()
		w.Write(b)
	}), nil
}
`,
	})
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	for _, goList := range []bool{false, true} {
		_, err := Load(Options{Dir: dir, UseUnsafe: true, GoList: goList}, "example.com/plug")
		var pe *PreflightError
		if !errors.As(err, &pe) || !strings.Contains(err.Error(), "inner.go:10:84: unsupported: native func syscall.Chdir not in registry") {
			t.Errorf("GoList %v: error %v, want a *PreflightError on syscall.Chdir in fast.Inner.MarshalJSON", goList, err)
		}
	}
}

// TestPreflightMethodSignatures checks that the check walks the parameters
// and results of the methods it adds: reflect reaches fast.Inner.MarshalJSON
// from outer{} with Method(0).Call(nil)[0].Interface(). Before, the types in
// method signatures were not walked: the load passed, and the request
// panicked with "native func syscall.Getpid not in registry" (now Chdir, as
// in TestPreflightFieldMethods).
func TestPreflightMethodSignatures(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":             preflightGoMod,
		"vendor/modules.txt": "# example.com/fast v1.0.0\n## explicit; go 1.20\nexample.com/fast\n",
		"vendor/example.com/fast/inner.go": `package fast

import (
	"fmt"
	"syscall"
)

type Inner struct{}

func (Inner) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprint(syscall.Chdir("/"))), nil }
`,
		"plug.go": `package plug

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"

	"example.com/fast"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type outer struct{}

func (outer) Get() fast.Inner { return fast.Inner{} }

// rec refers to itself through a method signature: the walk must stop.
type rec struct{}

func (rec) Next(rec) *rec { return nil }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = reflect.ValueOf(rec{})
		b, _ := reflect.ValueOf(outer{}).Method(0).Call(nil)[0].Interface().(json.Marshaler).MarshalJSON()
		w.Write(b)
	}), nil
}
`,
	})
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	for _, goList := range []bool{false, true} {
		_, err := Load(Options{Dir: dir, UseUnsafe: true, GoList: goList}, "example.com/plug")
		var pe *PreflightError
		if !errors.As(err, &pe) || !strings.Contains(err.Error(), "inner.go:10:84: unsupported: native func syscall.Chdir not in registry") {
			t.Errorf("GoList %v: error %v, want a *PreflightError on syscall.Chdir in fast.Inner.MarshalJSON", goList, err)
		}
	}
}

func errString(r any) string {
	if e, ok := r.(error); ok {
		return e.Error()
	}
	if s, ok := r.(string); ok {
		return s
	}
	return ""
}
