package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/types"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// TestDecodeConfig fills the interpreted config of a plugin with mapstructure,
// as Traefik does with testData: weakly typed input, a comma-separated string
// into a slice, a map, and keys whose case differs from the field names.
func TestDecodeConfig(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/plugins/w1headers")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{
		"prefix":         "X-",
		"addHeaders":     map[string]any{"X-Added": "yes"},
		"removeHeaders":  "X-Drop,X-Drop2",
		"blockuseragent": "badbot",
	}
	err = cfg.Decode(func(ptr any) error {
		d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook:       mapstructure.StringToSliceHookFunc(","),
			WeaklyTypedInput: true,
			Result:           ptr,
		})
		if err != nil {
			return err
		}
		return d.Decode(data)
	})
	if err != nil {
		t.Fatal(err)
	}
	var seen http.Header
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = r.Header.Clone() })
	h, err := newHandler(context.Background(), p, next, cfg, "w1")
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Drop", "1")
	r.Header.Set("X-Drop2", "1")
	r.Header.Set("X-Keep", "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if seen.Get("X-Added") != "yes" || seen.Get("X-Drop") != "" || seen.Get("X-Drop2") != "" || seen.Get("X-Keep") != "1" {
		t.Errorf("headers seen by next: %v", seen)
	}
	if got := w.Header().Get("X-Plugin-Matched"); got != "2" {
		t.Errorf("X-Plugin-Matched = %q, want 2 (X-Added, X-Keep)", got)
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("User-Agent", "a BadBot")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("blocked user agent: status %d, want 403", w.Code)
	}
}

// TestDecodeConfigMapUnexported decodes a config with a map of structs that
// have an unexported field, like the DatabaseSources of
// david-garcia-garcia/traefik-geoblock. The native map
// values have no address: the conversion back to the VM panicked with
// "reflect.Value.Addr of unaddressable value".
func TestDecodeConfigMapUnexported(t *testing.T) {
	data := map[string]any{"sources": map[string]any{"db": map[string]any{"path": "/db"}}}
	for _, aot := range []bool{false, true} {
		t.Run(fmt.Sprintf("aot=%v", aot), func(t *testing.T) {
			p, err := Load(Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/mapsrcplug")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			err = cfg.Decode(func(ptr any) error {
				return mapstructure.Decode(data, ptr)
			})
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "mapsrc")
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("X-Source", "db")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Body.String() != "/db" {
				t.Errorf("source db: status %d, body %q, want 200 and /db", w.Code, w.Body.String())
			}
		})
	}
}

// TestLoadSubpackage loads a plugin that imports a package of its own module:
// Load must interpret it (and run its init) instead of looking for it in the
// native registry.
func TestLoadSubpackage(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/subplugin")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "sub")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if got := w.Header().Get("X-Inner"); got != "INNER:default" {
		t.Errorf("X-Inner = %q, want INNER:default", got)
	}
	if got := w.Header().Get("X-Generics"); got != "b+d/b+d" {
		t.Errorf("X-Generics = %q, want b+d/b+d", got)
	}
}

// TestLoadWithoutToolchain loads plugins as Traefik would have to: no go
// command in PATH and no GOROOT. The go list loader fails there; the default
// loader reads the module and type-checks against the embedded stdlib types.
func TestLoadWithoutToolchain(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded stdlib types for this platform")
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GOROOT", filepath.Join(t.TempDir(), "nonexistent"))
	if _, err := Load(Options{Dir: "..", GoList: true}, "github.com/traefik/yaegi/v2/vm/testdata/subplugin"); err == nil {
		t.Fatal("go list loader succeeded without a toolchain: the test does not test anything")
	}
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/subplugin")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("load %v, SSA %v, init %v", p.LoadTime, p.BuildTime, p.InitTime)
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "sub")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if got := w.Header().Get("X-Inner"); got != "INNER:default" {
		t.Errorf("X-Inner = %q, want INNER:default", got)
	}
	if got := w.Header().Get("X-Generics"); got != "b+d/b+d" {
		t.Errorf("X-Generics = %q, want b+d/b+d", got)
	}
}

// TestRestrictions checks the restrictions of Traefik plugins: imports
// refused at load time (os/exec always, unsafe and syscall without
// useUnsafe, in every package run from source) and exits that panic instead
// of stopping the host.
func TestRestrictions(t *testing.T) {
	const dir = "github.com/traefik/yaegi/v2/vm/testdata/restricted/"
	// useUnsafe allows the import, and a restricted plugin gets the
	// read-only syscall subset (TestUnsafeSyscall), Getpid among them. On a
	// platform without a syscall table, the preflight refuses the call at
	// load time (before, it panicked at the first call, in New).
	vendorUnsafe := ""
	if !hasStdTable("syscall") {
		vendorUnsafe = "syscall.Getpid not in registry: it is not in the read-only syscall subset"
	}
	for _, c := range []struct {
		pkg       string
		useUnsafe bool
		want      string // error substring, "" for a successful load
	}{
		{"execplug", false, `import "os/exec" is not allowed`},
		{"execplug", true, `import "os/exec" is not allowed`},
		{"unsafeplug", false, `import "unsafe" is restricted`},
		{"unsafeplug", true, ""},
		{"vendorunsafe", false, `vendorunsafe/inner: import "syscall" is restricted`},
		{"vendorunsafe", true, vendorUnsafe},
		{"procplug", true, "os.StartProcess is not allowed"},
		{"procplug", true, "os.FindProcess is not allowed"},
		{"cgiplug", true, `import "net/http/cgi" is not allowed`},
		{"testingplug", true, "testing.Main is not allowed"},
		{"testingplug", true, "testing.Init is not allowed"},
		{"internalplug", true, `import "internal/godebug" is not allowed`},
		{"hostpkgplug", true, `import "go/build" is not allowed`},
		{"hostpkgplug", true, `import "go/importer" is not allowed`},
		{"hostpkgplug", true, `import "net/http/pprof" is not allowed`},
		{"typesplug", true, `import "go/types" is not allowed`},
		{"profplug", true, "runtime/pprof.StartCPUProfile is not allowed"},
		{"profplug", true, "runtime/trace.Start is not allowed"},
		{"quickplug", true, `import "testing/quick" needs the host to import github.com/traefik/yaegi/v2/stdsyms/sideeffect`},
		{"reflectplug", false, "reflect.NewAt is restricted"},
		{"reflectplug", false, "(reflect.Value).UnsafePointer is restricted"},
		{"reflectplug", false, "an expression of type unsafe.Pointer is restricted"},
		// Allowed with useUnsafe: the VM reflect has NewAt (protobuf).
		{"reflectplug", true, ""},
	} {
		for _, golist := range []bool{false, true} {
			_, err := Load(Options{Dir: "..", UseUnsafe: c.useUnsafe, GoList: golist}, dir+c.pkg)
			switch {
			case c.want == "" && err != nil:
				t.Errorf("%s (useUnsafe %v, go list %v): %v", c.pkg, c.useUnsafe, golist, err)
			case c.want != "" && err == nil:
				t.Errorf("%s (useUnsafe %v, go list %v): no error, want %q", c.pkg, c.useUnsafe, golist, c.want)
			case c.want != "" && golist && c.pkg == "internalplug":
				// go list refuses the import itself ("use of internal package").
			case c.want != "" && !strings.Contains(err.Error(), c.want):
				t.Errorf("%s (useUnsafe %v, go list %v): error %v, want %q", c.pkg, c.useUnsafe, golist, err, c.want)
			}
		}
	}
	if _, err := Load(Options{Dir: "..", Unrestricted: true}, dir+"execplug"); err != nil {
		t.Errorf("unrestricted: %v", err)
	}

	p, err := Load(Options{Dir: ".."}, dir+"exitplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "exit")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/exit": "os.Exit(3)", "/exitvalue": "os.Exit(4)", "/fatal": "fatal 5", "/loggerfatal": "logger6",
		"/defaultfatal": "default7",
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), want) {
					t.Errorf("%s: panic %v, want %q", path, r, want)
				}
			}()
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		}()
	}

	// Panics of the plugin's goroutines go to GoPanic, the host keeps
	// running (os.Exit in a goroutine is one).
	panics := make(chan any, 1)
	p, err = Load(Options{Dir: "..", GoPanic: func(r any) { panics <- r }}, dir+"exitplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ = createConfig(p)
	if h, err = newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "exit"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/goexit": "os.Exit(8)", "/gopanic": "boom 9"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if r := <-panics; !strings.Contains(fmt.Sprint(r), want) {
			t.Errorf("%s: goroutine panic %v, want %q", path, r, want)
		}
	}
}

// TestRestrictedSelfImport checks that a restricted plugin cannot import
// yaegi v2's own packages: the engine (vm) and the AOT copies (aot/...), by the
// module path github.com/traefik/yaegi/v2. The plugins are in yaegi v2's module,
// so their sources are found. With go list, the restrictions refuse the
// packages that vm imports (stdsyms has os/exec, go/types, ...). The default
// loader already stops at the dependencies of vm that are not in the module
// (golang.org/x/tools); a plugin that vendors them gets the go list case.
func TestRestrictedSelfImport(t *testing.T) {
	const dir = "github.com/traefik/yaegi/v2/vm/testdata/restricted/"
	for _, pkg := range []string{"selfvmplug", "selfaotplug"} {
		for _, useUnsafe := range []bool{false, true} {
			for _, golist := range []bool{false, true} {
				for _, aot := range []bool{false, true} {
					_, err := Load(Options{Dir: "..", UseUnsafe: useUnsafe, GoList: golist, AOT: aot}, dir+pkg)
					want := `could not import github.com/traefik/yaegi/v2/`
					if golist {
						want = `github.com/traefik/yaegi/v2/stdsyms: import "os/exec" is not allowed in a plugin`
					}
					switch {
					case err == nil:
						t.Errorf("%s (useUnsafe %v, go list %v, aot %v): no error, want %q", pkg, useUnsafe, golist, aot, want)
					case !strings.Contains(err.Error(), want):
						t.Errorf("%s (useUnsafe %v, go list %v, aot %v): error %.300s, want %q", pkg, useUnsafe, golist, aot, err, want)
					}
				}
			}
		}
	}
}

// TestUnsafeHeaders checks that a plugin with useUnsafe cannot reach memory
// outside its objects through unsafe: a forged string or slice header, an
// unsafe.Slice or unsafe.String that is too long, a pointer made from an
// address. Each attempt is refused with a panic that the plugin can
// recover; the zero-copy idioms still work. Without useUnsafe the plugin
// does not load.
func TestUnsafeHeaders(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/unsafehdrplug"
	if _, err := Load(Options{Dir: ".."}, pkg); err == nil || !strings.Contains(err.Error(), `import "unsafe" is restricted`) {
		t.Errorf("without useUnsafe: error %v, want the import refused", err)
	}
	p, err := Load(Options{Dir: "..", UseUnsafe: true}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "unsafe")
	if err != nil {
		t.Fatal(err)
	}
	// hostAddr in the plugin: smaller on 32-bit platforms.
	hostAddr := "0xc000010000"
	if strconv.IntSize == 32 {
		hostAddr = "0xc0000100"
	}
	for path, want := range map[string]string{
		"/ok":        "allowed: Jello Jello",
		"/cap":       "refused: unsupported: unsafe: a string or slice header of length 4 and capacity 1048576 reaches outside",
		"/datafirst": "refused: unsupported: unsafe: a string or slice header of length 1048576 and capacity 16 reaches outside",
		"/strlen":    "refused: unsupported: unsafe: a string or slice header of length 4096",
		"/data":      "refused: unsupported: unsafe: the data address 0x10000 of a string or slice header does not come from memory known to the VM",
		"/dataoff":   "refused: unsupported: unsafe: the data address",
		"/built":     "refused: unsupported: unsafe: the data address 0x10000",
		"/slice":     "refused: unsupported: unsafe.Slice: 1048576 elements reach outside the memory known to the VM (4 bytes)",
		"/string":    "refused: unsupported: unsafe.String: 1048576 elements reach outside the memory known to the VM (4 bytes)",
		"/cells":     "refused: unsupported: unsafe.Slice: 3 elements reach outside the memory known to the VM (2 elements)",
		"/add":       "refused: unsupported: unsafe: pointer 0x",
		"/deref":     "refused: unsupported: unsafe: pointer " + hostAddr + " does not point to a value known to the VM",
		// fmt recovers the panic of the conversion to a native pointer
		"/native":  "allowed: %!v(PANIC=Format method: unsupported: unsafe: pointer " + hostAddr + " does not point to a value known to the VM)",
		"/read":    "allowed: 5",
		"/rodata":  "allowed: 7 OK OK xxKOx",
		"/rowrite": "refused: unsupported: unsafe: writing into the bytes of a string that lives in read-only memory",
		// the plugin imports reflect, so it uses the VM reflect: the write
		// is refused like /rowrite
		"/roreflect": "refused: unsupported: unsafe: writing into the bytes of a string that lives in read-only memory",
		"/ronative":  "refused: unsafe: a pointer to the bytes of a string in read-only memory cannot go to native code",
		// a view of bytes (byteview.go) retyped to a type that is not
		// plain data, through an unknown origin or the same layout
		"/viewptr":    "refused: unsupported: unsafe: viewing a uint64 as a github.com/traefik/yaegi/v2/vm/testdata/restricted/unsafehdrplug.withPtr needs plain data",
		"/viewstr":    "refused: unsupported: unsafe: viewing a uint64 as a github.com/traefik/yaegi/v2/vm/testdata/restricted/unsafehdrplug.withStr needs plain data",
		"/viewnative": "refused: unsupported: unsafe: viewing a uint64 as a unsafe.Pointer needs plain data",
		"/viewok":     `allowed: {"A":5,"B":7} true`,
		// the words of an interface (ifaceview.go)
		"/typeword":   "refused: unsupported: unsafe: the type word of an interface seen as a *struct{size uintptr; ptrdata uintptr}: the VM has no runtime type to read",
		"/ifacewords": "allowed: ab",
		"/forgedtype": "refused: unsupported: unsafe: ",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Body.String(); !strings.HasPrefix(got, want) {
			t.Errorf("%s: %q, want prefix %q", path, got, want)
		}
	}
	// The refused writes did not change the host string, and the plugin
	// still serves after them.
	if got := http.StatusText(200); got != "OK" {
		t.Errorf("host string changed: %q", got)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ok", nil))
	if got := rec.Body.String(); got != "allowed: Jello Jello" {
		t.Errorf("/ok after the refused writes: %q", got)
	}
}

// TestUnrestrictedSymbols checks which native table a plugin gets. Only
// Unrestricted with RealProcess, and without a Sandbox, gives the real
// os.Exit and os/exec.Command. The others (the default, a Sandbox, and
// Unrestricted alone) get the os.Exit that panics, which the host can
// recover, and no os/exec.
func TestUnrestrictedSymbols(t *testing.T) {
	osPkg, execPkg := types.NewPackage("os", "os"), types.NewPackage("os/exec", "exec")
	const exitplug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/exitplug"
	for _, c := range []struct {
		opts Options
		real bool
	}{
		{Options{Dir: ".."}, false},
		{Options{Dir: "..", RealProcess: true}, false},
		{Options{Dir: "..", Unrestricted: true}, false},
		{Options{Dir: "..", Unrestricted: true, RealProcess: true, Sandbox: &Sandbox{}}, false},
		{Options{Dir: "..", Unrestricted: true, RealProcess: true}, true},
	} {
		name := fmt.Sprintf("Unrestricted %v, RealProcess %v, Sandbox %v", c.opts.Unrestricted, c.opts.RealProcess, c.opts.Sandbox != nil)
		p, err := Load(c.opts, exitplug)
		if err != nil {
			t.Fatal(err)
		}
		exit, _ := p.M.stdSym(osPkg, "Exit")
		_, hasExec := p.M.stdSym(execPkg, "Command")
		if got := exit.Pointer() == reflect.ValueOf(os.Exit).Pointer(); got != c.real || hasExec != c.real {
			t.Errorf("%s: real os.Exit %v, os/exec.Command %v; want %v", name, got, hasExec, c.real)
		}
		if c.real {
			continue
		}
		// os.Exit and log.Fatal panic: the host recovers.
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "exit")
		if err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]string{"/exit": "os.Exit(3)", "/fatal": "fatal 5"} {
			func() {
				defer func() {
					if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), want) {
						t.Errorf("%s: %s: panic %v, want %q", name, path, r, want)
					}
				}()
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
			}()
		}
	}
}

// TestRealStdSyscall checks that a program with the standard library as in
// Go may import syscall only where yaegi v2 has a syscall table: on another
// platform, the load fails with a clear error instead of a "not in
// registry" panic at the first call.
func TestRealStdSyscall(t *testing.T) {
	sys := types.NewPackage("syscall", "syscall")
	prog := types.NewPackage("example.com/prog", "main")
	prog.SetImports([]*types.Package{types.NewPackage("fmt", "fmt"), sys})
	src := []sourcePkg{{types: prog}}
	err := checkRealStd(src, func(string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), `example.com/prog: import "syscall" is not supported on`) {
		t.Errorf("platform without a table: error %v", err)
	}
	if err := checkRealStd(src, func(string) bool { return true }); err != nil {
		t.Errorf("platform with a table: %v", err)
	}

	// Every platform of stdtypes.Platforms has a generated syscall table,
	// and this one (a platform of the list) finds it.
	for _, pl := range stdtypes.Platforms {
		goos, goarch, _ := strings.Cut(pl, "/")
		if m, _ := filepath.Glob(filepath.Join("..", "stdsyms", "go*_syscall_"+goos+"_"+goarch+".go")); len(m) == 0 {
			t.Errorf("%s: no syscall table in stdsyms", pl)
		}
	}
	if slices.Contains(stdtypes.Platforms, runtime.GOOS+"/"+runtime.GOARCH) != hasStdTable("syscall") {
		t.Errorf("hasStdTable(syscall) = %v on %s/%s", hasStdTable("syscall"), runtime.GOOS, runtime.GOARCH)
	}
	if !hasStdTable("os/exec") {
		t.Errorf("no os/exec table")
	}

	// A plugin with the real standard library that imports syscall loads
	// here (a platform with a table).
	if slices.Contains(stdtypes.Platforms, runtime.GOOS+"/"+runtime.GOARCH) {
		if _, err := Load(Options{Dir: "..", Unrestricted: true, RealProcess: true}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/vendorunsafe"); err != nil {
			t.Errorf("RealProcess plugin that imports syscall: %v", err)
		}
	}
}

// TestPluginEnv checks that a restricted plugin has its own copy of the
// environment, as yaegi gives Traefik plugins.
func TestPluginEnv(t *testing.T) {
	t.Setenv("YAEGI_TEST_VAR", "host")
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/envplug"
	serve := func(opts Options) string {
		t.Helper()
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "env")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		return w.Body.String()
	}
	path := os.Getenv("PATH")
	if got, want := serve(Options{Dir: ".."}), "host plugin false 2 x"; got != want {
		t.Errorf("restricted: %q, want %q", got, want)
	}
	if os.Getenv("YAEGI_TEST_VAR") != "host" || os.Getenv("YAEGI_TEST_OTHER") != "" || os.Getenv("PATH") != path {
		t.Fatalf("the plugin changed the host environment")
	}
	if got, want := serve(Options{Dir: "..", Env: []string{"YAEGI_TEST_VAR=given"}}), "given plugin false 2 x"; got != want {
		t.Errorf("Env: %q, want %q", got, want)
	}
	t.Cleanup(func() { os.Unsetenv("YAEGI_TEST_OTHER"); os.Setenv("PATH", path) })
	if got, want := serve(Options{Dir: "..", Unrestricted: true}), "host plugin false 2 x"; got != want {
		t.Errorf("unrestricted: %q, want %q", got, want)
	}
	if os.Getenv("YAEGI_TEST_VAR") != "plugin" {
		t.Errorf("unrestricted: the plugin did not get the host environment")
	}
}

// TestUnsafeSyscall checks the read-only syscall subset of a restricted
// plugin with UseUnsafe (unsafesyscall.go): the subset works, Getenv and
// Environ read the plugin's own environment (Options.Env), not the one of
// the host; without UseUnsafe the import is refused as before; with
// UseUnsafe, a function outside the subset is still refused at load time.
func TestUnsafeSyscall(t *testing.T) {
	if !hasStdTable("syscall") {
		t.Skip("no syscall table on this platform")
	}
	t.Setenv("YAEGI_TEST_VAR", "host")
	const dir = "github.com/traefik/yaegi/v2/vm/testdata/restricted/"
	for _, c := range []struct {
		name string
		opts Options
		want string
	}{
		{"Env", Options{Dir: "..", UseUnsafe: true, Env: []string{"YAEGI_TEST_VAR=given", "YAEGI_TEST_OTHER=x"}},
			"pid=true ppid=true ids=true page=true time=true env=given,true,false,2 errno=true timespec=true"},
		{"empty Env", Options{Dir: "..", UseUnsafe: true, Env: []string{}},
			"pid=true ppid=true ids=true page=true time=true env=,false,false,0 errno=true timespec=true"},
		{"Sandbox", Options{Dir: "..", UseUnsafe: true, Sandbox: &Sandbox{Env: []string{"YAEGI_TEST_VAR=sandbox"}}},
			"pid=true ppid=true ids=true page=true time=true env=sandbox,true,false,1 errno=true timespec=true"},
	} {
		for _, golist := range []bool{false, true} {
			opts := c.opts
			opts.GoList = golist
			p, err := Load(opts, dir+"syscallplug")
			if err != nil {
				t.Errorf("%s (go list %v): %v", c.name, golist, err)
				continue
			}
			cfg, _ := createConfig(p)
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "syscall")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != c.want {
				t.Errorf("%s (go list %v): %q, want %q", c.name, golist, got, c.want)
			}
		}
	}

	// Without UseUnsafe: the import is refused, as before.
	if _, err := Load(Options{Dir: ".."}, dir+"syscallplug"); err == nil || !strings.Contains(err.Error(), `import "syscall" is restricted`) {
		t.Errorf("without UseUnsafe: %v, want the import refused", err)
	}

	// With UseUnsafe, the functions outside the subset are refused at load
	// time, each with its position.
	_, err := Load(Options{Dir: "..", UseUnsafe: true}, dir+"syscallrefplug")
	var pe *PreflightError
	if !errors.As(err, &pe) {
		t.Fatalf("outside the subset: %v, want a *PreflightError", err)
	}
	for _, name := range []string{"Open", "Setenv", "Chdir"} {
		want := "syscall." + name + " not in registry: it is not in the read-only syscall subset"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("outside the subset: %v\nwant %q", err, want)
		}
	}
	if len(pe.Problems) != 3 {
		t.Errorf("outside the subset: problems %q, want 3", pe.Problems)
	}
	// With NoPreflight, the call panics and nothing reaches the host.
	func() {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(errString(r), "syscall.Setenv not in registry") {
				t.Errorf("NoPreflight, /setenv: panic %v, want syscall.Setenv not in registry", r)
			}
		}()
		serveOnce(t, Options{Dir: "..", UseUnsafe: true, NoPreflight: true}, dir+"syscallrefplug", "/setenv")
	}()
	if os.Getenv("YAEGI_TEST_VAR") != "host" {
		t.Errorf("the plugin changed the host environment")
	}
}

// TestPluginPrintOut checks that Options.PrintOut receives the output of the
// print and println builtins of a plugin: at init, in New, in ServeHTTP and
// in a deferred call.
func TestPluginPrintOut(t *testing.T) {
	var out bytes.Buffer
	p, err := Load(Options{Dir: "..", PrintOut: &out}, "github.com/traefik/yaegi/v2/vm/testdata/printplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "pp")
	if err != nil {
		t.Fatal(err)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if got, want := out.String(), "init 1\nnew pp\nserve /x [0/0]0x0\ndeferred true\n"; got != want {
		t.Errorf("PrintOut got %q, want %q", got, want)
	}
}

// TestDecodeConfigRecursive fills a recursive plugin config (sub-rules in a
// slice, a pointer and a map, as the RawRule of smerschjohann/mtlswhitelist)
// with mapstructure and Traefik's options: it panicked with "call of
// reflect.Value.Field on map Value". The weakly typed
// input (a port as a string, ranges as a comma-separated string) must also
// work deep in the config, and a config deeper than the first unrolled type
// makes DecodeConfig decode again.
func TestDecodeConfigRecursive(t *testing.T) {
	deep := map[string]any{"type": "leaf"}
	want := "leaf[]:0()"
	for i := 0; i < 20; i++ {
		deep = map[string]any{"type": fmt.Sprint("d", i), "fallback": deep}
		want = fmt.Sprintf("d%d[]:0()else %s", i, want)
	}
	data := map[string]any{
		"rules": []any{
			map[string]any{
				"type":   "any",
				"ranges": "10.0.0.0/8,192.0.2.0/24",
				"rules": []any{
					map[string]any{"type": "ip", "port": "443", "rules": []any{map[string]any{"type": "x", "ranges": []any{"a"}}}},
				},
				"named": map[string]any{"b": map[string]any{"type": "nb", "port": 8}},
			},
		},
		"top": deep,
	}
	want = "any[10.0.0.0/8 192.0.2.0/24]:0(ip[]:443(x[a]:0()))b=nb[]:8();top=" + want
	for _, aot := range []bool{false, true} {
		t.Run(fmt.Sprintf("aot=%v", aot), func(t *testing.T) {
			p, err := Load(Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/recplug")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = cfg.Decode(func(ptr any) error {
				calls++
				d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
					DecodeHook:       mapstructure.StringToSliceHookFunc(","),
					WeaklyTypedInput: true,
					Result:           ptr,
				})
				if err != nil {
					return err
				}
				return d.Decode(data)
			})
			if err != nil {
				t.Fatal(err)
			}
			// The first call fills the cuts with maps; their keys give
			// the shape of the second one, which is deep enough.
			if calls != 2 {
				t.Errorf("decode called %d times, want 2", calls)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "rec")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != want {
				t.Errorf("config seen by New:\n got %s\nwant %s", got, want)
			}
		})
	}
}

// recShape returns a rule of the plugin recplug as JSON, depth levels deep,
// whose shape (sub-rule in rules, fallback or named at each level) comes from
// i, and its output by the plugin.
func recShape(i, depth int) (body, want string) {
	body, want = `{"type":"leaf"}`, "leaf[]:0()"
	for l := depth - 1; l >= 0; l-- {
		typ := fmt.Sprint("t", l)
		n := i
		for range l % 5 {
			n /= 3
		}
		switch n % 3 {
		case 0:
			body, want = fmt.Sprintf(`{"type":%q,"rules":[%s]}`, typ, body), typ+"[]:0("+want+")"
		case 1:
			body, want = fmt.Sprintf(`{"type":%q,"fallback":%s}`, typ, body), typ+"[]:0()else "+want
		case 2:
			body, want = fmt.Sprintf(`{"type":%q,"named":{"a":%s}}`, typ, body), typ+"[]:0()a="+want
		}
	}
	return body, want
}

// TestJSONUnmarshalRecursiveShapes sends request bodies of many shapes to a
// plugin that decodes them with json.Unmarshal into a recursive type, from
// several goroutines. reflect never frees the native types built for the
// data: the bytes of the new types must stay bounded whatever the data
// sends. Before the limits, each new shape 40 levels deep kept about 0.5 MB
// of host memory forever. Data that fits a uniform type must still work
// when the budget of data-shaped types is spent.
func TestJSONUnmarshalRecursiveShapes(t *testing.T) {
	p, err := Load(Options{Dir: "..", Modes: nativeJSON}, "github.com/traefik/yaegi/v2/vm/testdata/recplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "rec")
	if err != nil {
		t.Fatal(err)
	}
	post := func(body string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		return w.Code, w.Body.String()
	}

	// A budget of 4 MB of data-shaped types for this Machine.
	setShapedLimits(t, 4<<20, noProcessCap)
	before := unrolledBytes.Load()

	const requests, depth, workers = 120, 40, 4
	var ok, refused atomic.Int64
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < requests; i += workers {
				body, want := recShape(i, depth)
				code, got := post(body)
				switch {
				case code == http.StatusOK && got == want:
					ok.Add(1)
				case code == http.StatusBadRequest && strings.Contains(got, "reached its limit"):
					refused.Add(1)
				default:
					t.Errorf("shape %d: status %d, body %.200s\nwant %.200s", i, code, got, want)
				}
			}
		})
	}
	wg.Wait()
	grown := unrolledBytes.Load() - before
	t.Logf("%d decoded, %d refused, %d KB of new native types", ok.Load(), refused.Load(), grown>>10)
	if limit := int64(4<<20 + maxUniformBytes); grown > limit {
		t.Errorf("%d KB of new native types for %d shapes, want at most %d KB", grown>>10, requests, limit>>10)
	}
	if ok.Load() == 0 || refused.Load() == 0 {
		t.Errorf("%d decoded and %d refused, want some of both", ok.Load(), refused.Load())
	}

	// Shallow data of a new shape fits a uniform type: it still works.
	for i := range 9 {
		body, want := recShape(i+1000, 4)
		if code, got := post(body); code != http.StatusOK || got != want {
			t.Errorf("shallow shape %d: status %d, body %s, want %s", i, code, got, want)
		}
	}
}

// nativeJSON keeps encoding/json native: with the AOT copies in the host,
// json.Unmarshal goes to the AOT copy of json (aotroute.go), which decodes
// recursive types without native types. The data-shaped types serve native
// json (a host without the AOT copies, or an explicit mode).
var nativeJSON = map[string]Mode{"encoding/json": ModeNative}

// noProcessCap is a process budget so large that only the Machine budget
// acts. It is 1<<40 on 64-bit platforms; 1<<40 does not fit in an int on
// 32-bit platforms (linux/386, linux/arm), so there it is math.MaxInt.
const noProcessCap = min(1<<40, math.MaxInt)

// setShapedLimits sets the budgets of new data-shaped types (per Machine and
// for the process, for each use) for the test.
func setShapedLimits(t *testing.T, machine, process int) {
	t.Helper()
	shaped.mu.Lock()
	oldM, oldP := maxShapedMachine, maxShapedProcess
	maxShapedMachine, maxShapedProcess = machine, process
	shaped.mu.Unlock()
	t.Cleanup(func() {
		shaped.mu.Lock()
		maxShapedMachine, maxShapedProcess = oldM, oldP
		shaped.mu.Unlock()
	})
}

// recPlugin is an instance of the plugin recplug (a new Machine at each
// call).
type recPlugin struct {
	t *testing.T
	p *Plugin
	h http.Handler
}

func loadRecPlugin(t *testing.T) *recPlugin {
	t.Helper()
	p, err := Load(Options{Dir: "..", Modes: nativeJSON}, "github.com/traefik/yaegi/v2/vm/testdata/recplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "rec")
	if err != nil {
		t.Fatal(err)
	}
	return &recPlugin{t: t, p: p, h: h}
}

// post sends body to the plugin, which decodes it with json.Unmarshal.
func (r *recPlugin) post(body string) (int, string) {
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
	return w.Code, w.Body.String()
}

// postOK checks that the plugin decodes the rule of shape i.
func (r *recPlugin) postOK(what string, i, depth int) {
	r.t.Helper()
	body, want := recShape(i, depth)
	if code, got := r.post(body); code != http.StatusOK || got != want {
		r.t.Errorf("%s: status %d, body %.300s\nwant %.300s", what, code, got, want)
	}
}

// spend sends new shapes from i until the plugin refuses one because the
// budget is spent.
func (r *recPlugin) spend(i, depth int) {
	r.t.Helper()
	for n := range 1000 {
		body, _ := recShape(i+n, depth)
		code, got := r.post(body)
		if code == http.StatusBadRequest && strings.Contains(got, "reached its limit") {
			return
		}
		if code != http.StatusOK {
			r.t.Fatalf("shape %d: status %d, body %.300s", i+n, code, got)
		}
	}
	r.t.Fatal("the budget of data-shaped types is never spent")
}

// decodeConfig runs DecodeConfig with a config whose top rule is a chain of
// fallback rules, depth levels deep, with a sub-rule named name at the end,
// and checks what New sees.
func (r *recPlugin) decodeConfig(what, name string, depth int) error {
	r.t.Helper()
	top := map[string]any{"type": "leaf", "named": map[string]any{"b": map[string]any{"type": name}}}
	want := "leaf[]:0()b=" + name + "[]:0()"
	for i := range depth {
		top = map[string]any{"type": fmt.Sprint("f", i), "fallback": top}
		want = fmt.Sprintf("f%d[]:0()else %s", i, want)
	}
	cfg, err := createConfig(r.p)
	if err != nil {
		r.t.Fatal(err)
	}
	err = cfg.Decode(func(ptr any) error {
		return mapstructure.Decode(map[string]any{"top": top}, ptr)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	h, err := newHandler(context.Background(), r.p, http.NotFoundHandler(), cfg, "rec")
	if err != nil {
		r.t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if got := w.Body.String(); got != "top="+want {
		r.t.Errorf("%s: config seen by New:\n got %s\nwant top=%s", what, got, want)
	}
	return nil
}

// TestJSONUnmarshalRecursiveKnownShapes checks that data whose native types
// were already built still decodes once the budgets of new data-shaped types
// are spent, in the same Machine and in a new one. Before the fix, the budget
// was checked before looking for the type: a spent budget refused known
// shapes too.
func TestJSONUnmarshalRecursiveKnownShapes(t *testing.T) {
	setShapedLimits(t, 2<<20, noProcessCap)
	depth, cdepth := freshDepth(), freshDepth()
	a := loadRecPlugin(t)
	a.postOK("first decode of the known shape", 0, depth)
	if err := a.decodeConfig("first config", "known", cdepth); err != nil {
		t.Fatal(err)
	}
	a.spend(1, depth)
	a.postOK("known shape after the budget is spent", 0, depth)

	// A new Machine that may build no new type at all.
	shaped.mu.Lock()
	maxShapedMachine = 0
	shaped.mu.Unlock()
	b := loadRecPlugin(t)
	b.postOK("known shape in a new Machine", 0, depth)
	if err := b.decodeConfig("same config in a new Machine", "known", cdepth); err != nil {
		t.Error(err)
	}
	if err := b.decodeConfig("new config in a new Machine", "new", freshDepth()); err == nil || !strings.Contains(err.Error(), "reached its limit") {
		t.Errorf("new config with no budget: got %v, want the budget error", err)
	}
	body, _ := recShape(200, depth)
	if code, got := b.post(body); code != http.StatusBadRequest || !strings.Contains(got, "reached its limit") {
		t.Errorf("new shape once the process budget is spent: status %d, body %.300s", code, got)
	}
}

// TestJSONUnmarshalRecursiveBudgetPerPlugin checks that the clients of one
// plugin cannot spend the budget of new data-shaped types of the other
// plugins, nor the budget of DecodeConfig. Before the fix, one budget for the
// whole process was shared by all plugins and DecodeConfig: about 5 request
// bodies of new shapes made every new config with a chain of 5 fallback
// rules or more fail in every plugin.
func TestJSONUnmarshalRecursiveBudgetPerPlugin(t *testing.T) {
	setShapedLimits(t, 2<<20, noProcessCap)
	depth, cdepth := freshDepth(), freshDepth()
	a := loadRecPlugin(t)
	a.spend(0, depth)
	// The configuration of the same plugin has its own budget.
	if err := a.decodeConfig("new config of the plugin whose clients spent their budget", "cfgA", cdepth); err != nil {
		t.Error(err)
	}
	// Another plugin keeps its budgets.
	b := loadRecPlugin(t)
	if err := b.decodeConfig("new config of another plugin", "cfgB", freshDepth()); err != nil {
		t.Error(err)
	}
	b.postOK("new shape in another plugin", 200, depth)

	// The cap of the process is shared: once it is spent, new shapes fail
	// in every plugin.
	shaped.mu.Lock()
	maxShapedProcess = shaped.bytes[shapedData]
	shaped.mu.Unlock()
	c := loadRecPlugin(t)
	body, _ := recShape(220, depth)
	if code, got := c.post(body); code != http.StatusBadRequest || !strings.Contains(got, "reached its limit") {
		t.Errorf("new shape once the process cap is spent: status %d, body %.300s", code, got)
	}
}

// freshDepth gives a depth not used yet in this process for the shapes of
// recShape and the configs of decodeConfig: data-shaped types are kept by the
// process, so a test run again (-count) needs new ones.
func freshDepth() int {
	return 30 + int(recDepthSeq.Add(1))
}

var recDepthSeq atomic.Int64

// TestPluginNativeEqual checks == on native struct values (netip.Addr,
// netip.Prefix) in a plugin: it was always true, so every client matched
// the allow list (tomMoulard/fail2ban).
func TestPluginNativeEqual(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/allowplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p) // allow list: 192.0.2.1
	if err != nil {
		t.Fatal(err)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h, err := newHandler(context.Background(), p, ok, cfg, "allow")
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]int{
		"192.0.2.1:1234":   http.StatusOK,
		"192.0.2.2:1234":   http.StatusTooManyRequests,
		"198.51.100.1:80":  http.StatusTooManyRequests,
		"10.1.2.3:80":      http.StatusOK, // in the allowed prefix
		"11.1.2.3:80":      http.StatusTooManyRequests,
		"[2001:db8::1]:80": http.StatusTooManyRequests,
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s: status %d, want %d", addr, rec.Code, want)
		}
	}
}

// TestPluginReflectCallNamed checks that reflect.Value.Call in a subpackage
// keeps the named types of the plugin package, which does not import
// reflect itself: results, arguments, Value.Interface and
// Type.Out. Default mode and -aot, with both loaders.
func TestPluginReflectCallNamed(t *testing.T) {
	const want = "reflcallplug.Builder true 42\nS true 1\nL true 2\nM true 1\n*S true\n" +
		"arg Builder true\narg any L true\narg S true\nOut reflcallplug.Builder\n"
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"default", Options{Dir: ".."}},
		{"aot", Options{Dir: "..", AOT: true}},
		{"golist", Options{Dir: "..", GoList: true}},
		{"golist-aot", Options{Dir: "..", GoList: true, AOT: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Load(tc.opts, "github.com/traefik/yaegi/v2/vm/testdata/reflcallplug")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "reflcall")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// TestRunMainReflectCallNamed is the RunMain case of TestPluginReflectCallNamed: a
// module whose path has no dot, a package of it given with -src calls the
// functions of main with reflect.Value.Call, and main does not import
// reflect. The full-syntax load lost the module of the packages, so lib did
// not count as the program's own and native reflect was used.
func TestRunMainReflectCallNamed(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module repro\n\ngo 1.22\n",
		"lib/lib.go": `package lib

import "reflect"

func Call(f any, args ...any) any {
	in := make([]reflect.Value, len(args))
	for i, a := range args {
		in[i] = reflect.ValueOf(a)
	}
	return reflect.ValueOf(f).Call(in)[0].Interface()
}
`,
		"main.go": `package main

import "repro/lib"

type Builder func() int
type S struct{ A int }
type L []int

func main() {
	if _, ok := lib.Call(func() Builder { return func() int { return 42 } }).(Builder); !ok {
		panic("result Builder lost")
	}
	if _, ok := lib.Call(func() S { return S{} }).(S); !ok {
		panic("result S lost")
	}
	if _, ok := lib.Call(func(v any) any { return v }, L{1}).(L); !ok {
		panic("argument L lost")
	}
}
`,
	}
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, aot := range []bool{false, true} {
		if _, err := RunMain(RunOptions{Dir: dir, Src: []string{"repro/lib"}, Restricted: true, AOT: aot}, "."); err != nil {
			t.Errorf("aot=%v: %v", aot, err)
		}
	}
}

// TestPluginBudget checks that Options.Budget stops a plugin that never
// returns, in init and in a call from the host.
func TestPluginBudget(t *testing.T) {
	const dir = "github.com/traefik/yaegi/v2/vm/testdata/restricted/"
	if _, err := Load(Options{Dir: "..", Budget: 1e6}, dir+"initloop"); err == nil || !strings.Contains(err.Error(), ErrBudget.Error()) {
		t.Errorf("init: error %v, want %v", err, ErrBudget)
	}
	p, err := Load(Options{Dir: "..", Budget: 1e6}, dir+"loopplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "loop")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // every call gets the whole budget
		func() {
			defer func() {
				if r := recover(); r != ErrBudget {
					t.Errorf("ServeHTTP: panic %v, want %v", r, ErrBudget)
				}
			}()
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		}()
	}
}

// TestPluginBudgetCallbacks checks that the budget also holds in callbacks
// from native code: fmt must not turn the budget error of a String method
// into "%!v(PANIC=...)" text, and the callbacks of one call share its budget
// instead of each one getting a new budget.
func TestPluginBudgetCallbacks(t *testing.T) {
	p, err := Load(Options{Dir: "..", Budget: 1e6}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/cbbudgetplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "cb")
	if err != nil {
		t.Fatal(err)
	}
	serve := func(path string) (body string, r any) {
		rec := httptest.NewRecorder()
		defer func() { body, r = rec.Body.String(), recover() }()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return
	}
	for _, path := range []string{"/fmt", "/map", "/defer"} {
		if body, r := serve(path); r != ErrBudget || body != "" {
			t.Errorf("%s: panic %v and body %q, want %v and no body", path, r, body, ErrBudget)
		}
	}
	// Under the budget, callbacks run, and an ordinary panic in a String
	// method is still printed by fmt, as with go run.
	// go run prints "100 %!v(PANIC=String method: boom)"; main already
	// prints "Format method: panic: boom" (the VM gives fmt a value with a
	// Format method), so only the common part is checked here.
	if body, r := serve("/small"); r != nil || !strings.HasPrefix(body, "100 %!v(PANIC=") || !strings.HasSuffix(body, "boom)") {
		t.Errorf("/small: panic %v and body %q", r, body)
	}
}

// TestPluginBudgetGoNative checks that a goroutine started by the program
// with a native function ("go fmt.Sprint(x)", "go strings.Map(f, s)") has a
// budget that its callbacks share: the goroutine stops with ErrBudget, which
// GoPanic gets. fmt must not turn the error into text, and the callbacks
// must not each get a new budget.
func TestPluginBudgetGoNative(t *testing.T) {
	panics := make(chan any, 1)
	p, err := Load(Options{Dir: "..", Budget: 1e6, GoPanic: func(r any) { panics <- r }}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/cbbudgetplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/gofmt", "/gomap"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		select {
		case r := <-panics:
			if r != ErrBudget {
				t.Errorf("%s: goroutine panic %v, want %v", path, r, ErrBudget)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("%s: no goroutine panic, want %v", path, ErrBudget)
		}
	}
}

// TestPluginBudgetChain checks that each call from the host into a plugin
// has its own budget, also when the same plugin comes twice in a middleware
// chain: the second instance runs inside the call of the first one, but it
// is a new call from the host (through next), not a callback.
func TestPluginBudgetChain(t *testing.T) {
	p, err := Load(Options{Dir: "..", Budget: 1e6}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/cbbudgetplug")
	if err != nil {
		t.Fatal(err)
	}
	last := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "end") })
	var h http.Handler = last
	for _, name := range []string{"second", "first"} {
		cfg, _ := createConfig(p)
		if h, err = newHandler(context.Background(), p, h, cfg, name); err != nil {
			t.Fatal(err)
		}
	}
	// The host calls the chain twice: the budget does not carry over
	// from one request to the next either.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("request %d: panic %v", i, r)
				}
			}()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/chain", nil))
		}()
		if body := rec.Body.String(); body != "00end" {
			t.Errorf("request %d: body %q, want %q", i, body, "00end")
		}
	}
}

// TestPluginBudgetRecover checks that recover() and "defer recover()" in a
// later deferred call cannot stop ErrBudget or ErrStackOverflow: the host
// must get the panic.
func TestPluginBudgetRecover(t *testing.T) {
	p, err := Load(Options{Dir: "..", Budget: 1e6}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/recoverplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "recover")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]error{
		"/deferrecover": ErrBudget,
		"/recover":      ErrBudget,
		"/stack":        ErrStackOverflow,
		"/stackdefer":   ErrStackOverflow,
	} {
		func() {
			defer func() {
				if r := recover(); r != want {
					t.Errorf("%s: panic %v, want %v", path, r, want)
				}
			}()
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		}()
	}
}

// TestPluginGoexit checks runtime.Goexit in plugin code: New returns
// ErrGoexit, and ServeHTTP runs the deferred calls and then ends the
// goroutine of the host (a real runtime.Goexit, as a native plugin would),
// with no panic that net/http could log.
func TestPluginGoexit(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/goexitplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	for _, name := range []string{"goexit", "oncedo"} {
		if _, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, name); err != ErrGoexit {
			t.Errorf("New(%s): error %v, want %v", name, err, ErrGoexit)
		}
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "h")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/fmt"} {
		w := httptest.NewRecorder()
		var returned bool
		var panicked any
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { panicked = recover() }()
			h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			returned = true
		}()
		<-done
		if returned || panicked != nil {
			t.Errorf("%s: ServeHTTP returned %v, panic %v; want the end of the goroutine", path, returned, panicked)
		}
		if got := w.Body.String(); got != "deferred" {
			t.Errorf("%s: body %q, want %q", path, got, "deferred")
		}
	}
}

// TestPluginGoexitValue checks that runtime.Goexit given as a func value to
// native code that runs it on its own goroutine (time.AfterFunc, a go
// statement of a native method) ends that goroutine quietly: the host
// process must survive.
func TestPluginGoexitValue(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/goexitvalplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "v")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	time.Sleep(50 * time.Millisecond)
	if got := w.Body.String(); got != "alive" {
		t.Errorf("body %q, want %q", got, "alive")
	}
}

// TestPluginStackOverflow checks that a plugin that recurses without end
// stops with ErrStackOverflow at the plugin depth limit (not the larger
// limit of RunMain programs).
func TestPluginStackOverflow(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/recplug")
	if err != nil {
		t.Fatal(err)
	}
	if p.M.MaxDepth != 100000 || p.M.MaxStack != DefaultMaxStack {
		t.Errorf("plugin limits: MaxDepth %d MaxStack %d, want 100000 and %d", p.M.MaxDepth, p.M.MaxStack, DefaultMaxStack)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "rec")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != ErrStackOverflow {
			t.Errorf("ServeHTTP: panic %v, want %v", r, ErrStackOverflow)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

// TestPluginNativeRecursion checks that a plugin that recurses without end
// through callbacks of native code (each callback runs on a new ctx, with
// real Go stack between them) stops with ErrStackOverflow. Before, the
// depth of each callback started at zero, and gc killed the process with
// "fatal error: stack overflow".
func TestPluginNativeRecursion(t *testing.T) {
	panics := make(chan any, 1)
	p, err := Load(Options{Dir: "..", GoPanic: func(r any) { panics <- r }}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/nativerecplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "rec")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/map", "/sort", "/stringer", "/defer", "/deep", "/map"} {
		func() {
			defer func() {
				if r := recover(); r != ErrStackOverflow {
					t.Errorf("%s: panic %v, want %v", path, r, ErrStackOverflow)
				}
			}()
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		}()
	}
	// In a goroutine of the plugin, the error goes to GoPanic.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/go", nil))
	if r := <-panics; r != ErrStackOverflow {
		t.Errorf("/go: goroutine panic %v, want %v", r, ErrStackOverflow)
	}
	// The host and the plugin still work.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Body.String() != "survived" {
		t.Errorf("after the overflows: body %q, want %q", w.Body.String(), "survived")
	}
	// With a budget, the callbacks share the budget of the call from the
	// host: the recursion through sort.Slice stops on the budget.
	p, err = Load(Options{Dir: "..", Budget: 1000}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/nativerecplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ = createConfig(p)
	if h, err = newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "rec"); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if r := recover(); r != ErrBudget {
				t.Errorf("budget: panic %v, want %v", r, ErrBudget)
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/sort", nil))
	}()
}

// TestPluginErrorCycle checks that errors.Is, errors.As and errors.AsType
// on a cyclic error chain stop the plugin with ErrBudget or
// ErrStackOverflow, in default mode and in AOT mode, and that the host
// and the plugin still work after that.
func TestPluginErrorCycle(t *testing.T) {
	const plug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/errcycplug"
	serve := func(h http.Handler, path string) (body string, r any) {
		defer func() { r = recover() }()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Body.String(), nil
	}
	load := func(t *testing.T, opts Options) http.Handler {
		t.Helper()
		opts.Dir = ".."
		p, err := Load(opts, plug)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "cyc")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	fns := []string{"is", "as", "astype"}
	for _, aot := range []bool{false, true} {
		t.Run(fmt.Sprintf("aot=%v", aot), func(t *testing.T) {
			// With a budget, every cycle ends with ErrBudget: the walk
			// counts a step for each error, and AOT code that calls an
			// interpreted Unwrap counts a step for each call. The budget is
			// small enough to end before MaxNest: in AOT mode, a level of
			// the join cycle has a native Unwrap (no step) and an
			// interpreted one (one step).
			h := load(t, Options{Budget: 1000, AOT: aot})
			for _, fn := range fns {
				for _, name := range []string{"self", "multi", "native", "join"} {
					path := "/" + fn + "/" + name
					if _, r := serve(h, path); r != ErrBudget {
						t.Errorf("%s: panic %v, want %v", path, r, ErrBudget)
					}
				}
			}
			if body, r := serve(h, "/ok"); r != nil || body != "true" {
				t.Errorf("/ok after the cycles: body %q, panic %v", body, r)
			}
			// Without a budget, a cycle of Unwrap() []error ends with
			// ErrStackOverflow, not with a fatal Go stack overflow that
			// kills the host: MaxDepth for the walk of the default mode,
			// MaxNest for the recursion of the AOT copy of errors
			// (NestAOT).
			h = load(t, Options{AOT: aot})
			for _, fn := range fns {
				for _, name := range []string{"multi", "join"} {
					path := "/" + fn + "/" + name
					if _, r := serve(h, path); r != ErrStackOverflow {
						t.Errorf("%s: panic %v, want %v", path, r, ErrStackOverflow)
					}
				}
			}
			if body, r := serve(h, "/ok"); r != nil || body != "true" {
				t.Errorf("/ok after the overflows: body %q, panic %v", body, r)
			}
		})
	}
}

// TestPluginJSONCycle checks that json.Marshal of a value that contains
// itself returns the UnsupportedValueError of go run in a plugin, with and
// without a budget, in default and AOT mode. Before the fix, the host died
// with a fatal Go stack overflow, which recover cannot stop: in AOT mode for
// pointer cycles (the cycle check of the AOT copy never matched: vm.PtrKey),
// in default mode for maps and slices (the conversion to native data never
// ended: convGraph.enterComp).
func TestPluginJSONCycle(t *testing.T) {
	const plug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/jsoncycplug"
	const prefix = "json: unsupported value: encountered a cycle via "
	want := map[string]string{
		"self":  "*jsoncycplug.node",
		"two":   "*jsoncycplug.left",
		"map":   "map[string]interface {}",
		"slice": "[]interface {}",
	}
	for _, aot := range []bool{false, true} {
		for _, budget := range []int64{0, 1_000_000} {
			t.Run(fmt.Sprintf("aot=%v/budget=%d", aot, budget), func(t *testing.T) {
				p, err := Load(Options{Dir: "..", AOT: aot, Budget: budget}, plug)
				if err != nil {
					t.Fatal(err)
				}
				cfg, _ := createConfig(p)
				h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "cyc")
				if err != nil {
					t.Fatal(err)
				}
				for shape, via := range want {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("GET", "/"+shape, nil))
					got := w.Body.String()
					// default mode too: json.Marshal goes to the AOT copy
					// of json (aotroute.go), which names the interpreted
					// type, as go run
					if got != prefix+via {
						t.Errorf("%s: %q, want %q", shape, got, prefix+via)
					}
				}
			})
		}
	}
}

// TestLoadPaths checks that a plugin cannot load code from outside its
// module.
func TestLoadPaths(t *testing.T) {
	_, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/../restricted/execplug")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("import path with ..: error %v", err)
	}
	mod := t.TempDir()
	os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "p.go"), []byte("package p\n\nimport _ \"example.com/p/out\"\n"), 0o644)
	out := t.TempDir()
	os.WriteFile(filepath.Join(out, "out.go"), []byte("package out\n"), 0o644)
	if err := os.Symlink(out, filepath.Join(mod, "out")); err != nil {
		t.Skip(err)
	}
	if _, err := Load(Options{Dir: mod}, "example.com/p"); err == nil || !strings.Contains(err.Error(), "outside the module") {
		t.Errorf("symbolic link out of the module: error %v", err)
	}
}

// TestLoadSrcStd checks that the standard library packages of Options.Src
// are not subject to the restrictions (they import unsafe).
func TestLoadSrcStd(t *testing.T) {
	if _, err := Load(Options{Dir: "..", Src: []string{"strings"}}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/envplug"); err != nil {
		t.Error(err)
	}
}

// TestVendorGoVersion checks the language version of vendored modules: the
// go line of vendor/modules.txt, or go1.16 for a module without one, as gc.
func TestVendorGoVersion(t *testing.T) {
	std, err := stdtypes.Load()
	if err != nil {
		t.Skip(err)
	}
	mod := t.TempDir()
	os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644)
	os.MkdirAll(filepath.Join(mod, "vendor"), 0o755)
	os.WriteFile(filepath.Join(mod, "vendor", "modules.txt"), []byte(
		"# example.com/old v1.0.0\n## explicit\nexample.com/old\n# example.com/new v1.0.0\n## explicit; go 1.21\nexample.com/new/sub\n"), 0o644)
	l, err := newSrcLoader(context.Background(), mod, "", "", "example.com/p", std, newLoadBudget(DefaultLoadLimits))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"example.com/p/x": "go1.22", "example.com/old": "go1.16", "example.com/new/sub": "go1.21",
	} {
		if got := l.goVersionOf(path); got != want {
			t.Errorf("goVersionOf(%s) = %s, want %s", path, got, want)
		}
	}
}

// TestNestedPluginModule loads a plugin package that is its own nested module
// (testdata/nestedmod/plugin: its own go.mod and vendor/) with Options.Dir at
// the root module, as Traefik does for darkweak/souin and darkweak/go-esi.
// Go builds it with the go.mod and the vendor directory of the nested
// module: lib.Name is the vendored copy. Before, the toolchain-free loader
// read the root module sources ("root module source"), and go list ran in
// the root module, which does not contain the package.
func TestNestedPluginModule(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	dir := filepath.Join("testdata", "nestedmod")
	for _, goList := range []bool{false, true} {
		if got := serveOnce(t, Options{Dir: dir, GoList: goList}, "example.com/root/plugin", "/"); got != "plugin vendor copy" {
			t.Errorf("GoList %v: body %q, want %q", goList, got, "plugin vendor copy")
		}
	}
}

// TestNestedLibVendor loads a root plugin package (testdata/nestedlib) that
// imports example.com/root/lib, where lib/ has its own go.mod and vendor/ has
// a copy of it. lib/ is not part of the main module, so Go builds the vendor
// copy. Before, the toolchain-free loader read the lib/ directory ("root lib
// directory"); go list already used the vendor copy.
func TestNestedLibVendor(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	dir := filepath.Join("testdata", "nestedlib")
	for _, goList := range []bool{false, true} {
		if got := serveOnce(t, Options{Dir: dir, GoList: goList}, "example.com/root", "/"); got != "root vendor copy" {
			t.Errorf("GoList %v: body %q, want %q", goList, got, "root vendor copy")
		}
	}
}

// TestNestedModuleMetadataSymlink checks that the toolchain-free loader
// refuses a nested go.mod or a vendor/modules.txt that is a symbolic link to
// a file outside the plugin archive, as it refuses such links to Go files.
// Before, both were read through the link (metadata only: the module path,
// the go and version lines).
func TestNestedModuleMetadataSymlink(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	out := t.TempDir()
	os.WriteFile(filepath.Join(out, "go.mod"), []byte("module example.com/root/plugin\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(out, "modules.txt"), []byte("# example.com/x v1.0.0\n## explicit; go 1.22\nexample.com/x\n"), 0o644)
	newMod := func(t *testing.T) string {
		mod := t.TempDir()
		os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.com/root\n\ngo 1.22\n"), 0o644)
		os.MkdirAll(filepath.Join(mod, "plugin", "vendor"), 0o755)
		os.WriteFile(filepath.Join(mod, "plugin", "plugin.go"), []byte("package plugin\n"), 0o644)
		return mod
	}
	t.Run("go.mod", func(t *testing.T) {
		mod := newMod(t)
		if err := os.Symlink(filepath.Join(out, "go.mod"), filepath.Join(mod, "plugin", "go.mod")); err != nil {
			t.Skip(err)
		}
		if _, err := Load(Options{Dir: mod}, "example.com/root/plugin"); err == nil || !strings.Contains(err.Error(), "outside the module") {
			t.Errorf("symbolic link go.mod: error %v", err)
		}
	})
	t.Run("modules.txt", func(t *testing.T) {
		mod := newMod(t)
		os.WriteFile(filepath.Join(mod, "plugin", "go.mod"), []byte("module example.com/root/plugin\n\ngo 1.22\n"), 0o644)
		if err := os.Symlink(filepath.Join(out, "modules.txt"), filepath.Join(mod, "plugin", "vendor", "modules.txt")); err != nil {
			t.Skip(err)
		}
		if _, err := Load(Options{Dir: mod}, "example.com/root/plugin"); err == nil || !strings.Contains(err.Error(), "outside the module") {
			t.Errorf("symbolic link modules.txt: error %v", err)
		}
	})
}

// TestSandbox checks the capability sandbox: the plugin sees only its own
// environment, reads and writes only the granted directories, and its dials
// to an ungranted host are refused.
func TestSandbox(t *testing.T) {
	t.Setenv("YAEGI_SB", "host-visible") // must NOT leak into the plugin
	base := t.TempDir()
	inDir := filepath.Join(base, "in")
	outDir := filepath.Join(base, "out")
	os.MkdirAll(inDir, 0o755)
	os.MkdirAll(outDir, 0o755)
	inFile := filepath.Join(inDir, "data.txt")
	os.WriteFile(inFile, []byte("secret"), 0o644)
	denyFile := filepath.Join(base, "forbidden.txt")
	os.WriteFile(denyFile, []byte("nope"), 0o644)

	p, err := Load(Options{Dir: "..", Sandbox: &Sandbox{
		Env:        []string{"YAEGI_SB=cap", "YAEGI_IN=" + inFile, "YAEGI_OUT=" + filepath.Join(outDir, "w.txt"), "YAEGI_DENY=" + denyFile},
		ReadPaths:  []string{inDir},
		WritePaths: []string{outDir},
	}}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/sandboxplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "sb")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	want := `env="cap" in=ok(secret) denyread=denied write=ok denywrite=denied dial=denied`
	if w.Body.String() != want {
		t.Errorf("got %q\nwant %q", w.Body.String(), want)
	}
	// The granted write happened, the denied one did not.
	if _, err := os.Stat(filepath.Join(outDir, "w.txt")); err != nil {
		t.Errorf("granted write did not land: %v", err)
	}
	if b, _ := os.ReadFile(denyFile); string(b) != "nope" {
		t.Errorf("denied file was modified: %q", b)
	}
}

// TestLoadCachedFieldTrack checks that a cached reload keeps FieldTrack: with
// GOEXPERIMENT=fieldtrack, the //go:nointerface method of the plugin must not
// satisfy the interface after LoadCached either.
func TestLoadCachedFieldTrack(t *testing.T) {
	t.Setenv("GOEXPERIMENT", "fieldtrack")
	p, err := Load(Options{Dir: "..", GoList: true}, "github.com/traefik/yaegi/v2/vm/testdata/nointerface")
	if err != nil {
		t.Fatal(err)
	}
	q, err := p.LoadCached(filepath.Join("testdata", "nointerface"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		p    *Plugin
	}{{"Load", p}, {"LoadCached", q}} {
		if !c.p.M.FieldTrack {
			t.Errorf("%s: FieldTrack = false, want true", c.name)
		}
		cfg, err := createConfig(c.p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := newHandler(context.Background(), c.p, http.NotFoundHandler(), cfg, "noiface")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if got := w.Header().Get("X-Bad"); got != "false" {
			t.Errorf("%s: X-Bad = %q, want false (*U.Bad is marked //go:nointerface)", c.name, got)
		}
	}
}

// TestPluginProvider runs a provider plugin as Traefik does: New returns the
// provider, the host calls Init, Provide with a native channel of
// json.Marshaler, reads configurations marshaled by interpreted MarshalJSON
// methods, then calls Stop.
func TestPluginProvider(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/providerplugin")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Decode(func(ptr any) error {
		return mapstructure.Decode(map[string]any{"names": []string{"a", "b"}}, ptr)
	})
	if err != nil {
		t.Fatal(err)
	}
	prov, err := newProvider(context.Background(), p, cfg, "plugin-test")
	if err != nil {
		t.Fatal(err)
	}

	res, err := callMethod(prov, "Init")
	if err != nil || len(res) != 1 || res[0] != nil {
		t.Fatalf("Init: %v, %v", res, err)
	}

	cfgChan := make(chan json.Marshaler)
	res, err = callMethod(prov, "Provide", (chan<- json.Marshaler)(cfgChan))
	if err != nil || len(res) != 1 || res[0] != nil {
		t.Fatalf("Provide: %v, %v", res, err)
	}

	for range 2 {
		select {
		case msg := <-cfgChan:
			b, err := msg.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			want := `{"http":{"routers":{"a":{"service":"svc-a","rule":"Host(` + "`a.localhost`" + `)"},"b":{"service":"svc-b","rule":"Host(` + "`b.localhost`" + `)"}}}}`
			if string(b) != want {
				t.Fatalf("configuration:\n got %s\nwant %s", b, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no configuration from the provider")
		}
	}

	if _, err := callMethod(prov, "Stop"); err != nil {
		t.Fatal(err)
	}
	if _, err := callMethod(prov, "Missing"); err == nil {
		t.Error("calling a missing method succeeded")
	}
}

// TestPluginProviderNewError checks that the error of New is returned.
func TestPluginProviderNewError(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/providerplugin")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newProvider(context.Background(), p, cfg, "plugin-test"); err == nil || err.Error() != "no names" {
		t.Fatalf("NewProvider error = %v, want no names", err)
	}
}

// loadProvider loads the provider plugin with opts and returns it with a
// config for mode (see the testdata package).
func loadProvider(t *testing.T, opts Options, mode string) (*Plugin, *Object) {
	t.Helper()
	opts.Dir = ".."
	p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/providerplugin")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetFields(map[string]any{"Names": []string{"a"}, "Mode": mode}); err != nil {
		t.Fatal(err)
	}
	return p, cfg
}

// returns runs f on a new goroutine and reports whether f returned: a
// runtime.Goexit that ends the goroutine of the host makes it false.
func returns(t *testing.T, f func()) bool {
	t.Helper()
	returned := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
		returned = true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the call did not end")
	}
	return returned
}

// waitLive reports whether all the goroutines of m end within d.
func waitLive(m *Machine, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		m.waitGoroutines()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// TestPluginProviderGoexit checks that runtime.Goexit in the New of a
// provider plugin, or in a method that the host calls (Init), returns
// ErrGoexit, as Plugin.New does: the goroutine of the host must go on.
func TestPluginProviderGoexit(t *testing.T) {
	p, cfg := loadProvider(t, Options{}, "newgoexit")
	var err error
	if !returns(t, func() { _, err = newProvider(context.Background(), p, cfg, "goexit") }) || err != ErrGoexit {
		t.Errorf("NewProvider: error %v, want %v and a return", err, ErrGoexit)
	}

	p, cfg = loadProvider(t, Options{}, "initgoexit")
	prov, err := newProvider(context.Background(), p, cfg, "goexit")
	if err != nil {
		t.Fatal(err)
	}
	if !returns(t, func() { _, err = callMethod(prov, "Init") }) || err != ErrGoexit {
		t.Errorf("Init: error %v, want %v and a return", err, ErrGoexit)
	}
}

// TestPluginProviderErrors checks that ErrBudget and a panic in New and in
// Init come back as errors.
func TestPluginProviderErrors(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{
		{"newloop", ErrBudget.Error()},
		{"newpanic", "boom in new"},
		{"initloop", ErrBudget.Error()},
		{"initpanic", "boom in init"},
	} {
		p, cfg := loadProvider(t, Options{Budget: 1e6}, tc.mode)
		prov, err := newProvider(context.Background(), p, cfg, tc.mode)
		if strings.HasPrefix(tc.mode, "init") {
			if err != nil {
				t.Fatalf("%s: NewProvider: %v", tc.mode, err)
			}
			_, err = callMethod(prov, "Init")
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want %q", tc.mode, err, tc.want)
		}
	}
}

// TestPluginProviderGoPanic checks that ErrBudget and a panic in the
// goroutine that Provide starts go to GoPanic, with Options and with a
// Sandbox: the host keeps running and the goroutine ends.
func TestPluginProviderGoPanic(t *testing.T) {
	for _, sandbox := range []bool{false, true} {
		for _, tc := range []struct {
			mode string
			want any
		}{
			{"goloop", ErrBudget},
			{"gopanic", "boom in go"},
		} {
			panics := make(chan any, 1)
			opts := Options{Budget: 1e6, GoPanic: func(r any) { panics <- r }}
			if sandbox {
				opts = Options{Sandbox: &Sandbox{Budget: 1e6, GoPanic: func(r any) { panics <- r }}}
			}
			p, cfg := loadProvider(t, opts, tc.mode)
			prov, err := newProvider(context.Background(), p, cfg, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			cfgChan := make(chan json.Marshaler)
			if res, err := callMethod(prov, "Provide", (chan<- json.Marshaler)(cfgChan)); err != nil || res[0] != nil {
				t.Fatalf("sandbox %v, %s: Provide: %v, %v", sandbox, tc.mode, res, err)
			}
			select {
			case r := <-panics:
				// ErrBudget is the value itself; a panic of the plugin is
				// a VM value that prints like the Go panic.
				ok := r == tc.want
				if s, isText := tc.want.(string); isText {
					ok = fmt.Sprint(r) == "panic: "+s
				}
				if !ok {
					t.Errorf("sandbox %v, %s: goroutine panic %v, want %v", sandbox, tc.mode, r, tc.want)
				}
			case <-time.After(10 * time.Second):
				t.Errorf("sandbox %v, %s: no goroutine panic, want %v", sandbox, tc.mode, tc.want)
			}
			if !waitLive(p.M, 10*time.Second) {
				t.Errorf("sandbox %v, %s: the goroutine did not end", sandbox, tc.mode)
			}
		}
	}
}

// TestPluginProviderStop checks that Stop really ends the goroutine that
// Provide starts.
func TestPluginProviderStop(t *testing.T) {
	p, cfg := loadProvider(t, Options{}, "")
	prov, err := newProvider(context.Background(), p, cfg, "stop")
	if err != nil {
		t.Fatal(err)
	}
	cfgChan := make(chan json.Marshaler)
	if _, err := callMethod(prov, "Provide", (chan<- json.Marshaler)(cfgChan)); err != nil {
		t.Fatal(err)
	}
	<-cfgChan // the goroutine runs
	if waitLive(p.M, 50*time.Millisecond) {
		t.Fatal("no live goroutine after Provide")
	}
	if _, err := callMethod(prov, "Stop"); err != nil {
		t.Fatal(err)
	}
	// Like Traefik, the host keeps reading: a send started before Stop
	// must not block the goroutine.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-cfgChan:
			case <-stop:
				return
			}
		}
	}()
	if !waitLive(p.M, 10*time.Second) {
		t.Error("the goroutine of Provide did not end after Stop")
	}
}

// TestPluginAsyncCallbackPanic checks that a callback that native code runs
// on a goroutine of its own (time.AfterFunc, context.AfterFunc, a
// finalizer) cannot end the host: its panic, a runtime error, or the end of
// its budget goes to GoPanic, in every mode of the plugin. In Go, the
// process would end.
func TestPluginAsyncCallbackPanic(t *testing.T) {
	modes := []struct {
		name string
		opts func(goPanic func(any)) Options
	}{
		{"default", func(gp func(any)) Options { return Options{Dir: "..", Budget: 1e6, GoPanic: gp} }},
		{"sandbox", func(gp func(any)) Options {
			return Options{Dir: "..", Sandbox: &Sandbox{Budget: 1e6, GoPanic: gp}}
		}},
		{"aot", func(gp func(any)) Options { return Options{Dir: "..", AOT: true, Budget: 1e6, GoPanic: gp} }},
	}
	cases := []struct {
		name string
		want string // the value GoPanic gets, printed
	}{
		{"timer", "panic: timer boom"},
		{"timerloop", ErrBudget.Error()},
		{"ctx", "panic: ctx boom"},
		{"ctxloop", ErrBudget.Error()},
		{"ctxerr", "assignment to entry in nil map"},
		{"finalizer", "panic: finalizer boom"},
	}
	for _, mode := range modes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				panics := make(chan any, 1)
				p, err := Load(mode.opts(func(r any) { panics <- r }), "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
				if err != nil {
					t.Fatal(err)
				}
				cfg, _ := createConfig(p)
				if _, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, tc.name); err != nil {
					t.Fatal(err)
				}
				// The finalizer runs after a GC sees the object unreachable.
				deadline := time.After(10 * time.Second)
				tick := time.NewTicker(10 * time.Millisecond)
			wait:
				for {
					select {
					case r := <-panics:
						if s := fmt.Sprint(r); !strings.Contains(s, tc.want) {
							t.Errorf("GoPanic got %q, want %q", s, tc.want)
						}
						break wait
					case <-tick.C:
						runtime.GC()
					case <-deadline:
						t.Errorf("GoPanic got nothing, want %q", tc.want)
						break wait
					}
				}
				tick.Stop()
			})
		}
	}
}

// TestPluginRecursiveNamed loads a plugin with recursive named types that
// are not structs (vm/testdata/restricted/recnamedplug), with and without a
// budget. In default mode the first value that crossed to native code
// killed the test binary with a fatal Go stack overflow (reflectType never
// ended), which recover, Budget and MaxNest cannot stop.
func TestPluginRecursiveNamed(t *testing.T) {
	const plug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/recnamedplug"
	for _, aot := range []bool{false, true} {
		for _, budget := range []int64{0, 1_000_000} {
			t.Run(fmt.Sprintf("aot=%v/budget=%d", aot, budget), func(t *testing.T) {
				p, err := Load(Options{Dir: "..", AOT: aot, Budget: budget}, plug)
				if err != nil {
					t.Fatal(err)
				}
				cfg, err := createConfig(p)
				if err != nil {
					t.Fatal(err)
				}
				err = cfg.Decode(func(ptr any) error {
					return mapstructure.Decode(map[string]any{
						"tree":  map[string]any{"a": map[string]any{"b": map[string]any{}}},
						"lists": []any{[]any{}, []any{[]any{}}},
					}, ptr)
				})
				if err != nil {
					t.Fatal(err)
				}
				h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "recnamed")
				if err != nil {
					t.Fatal(err)
				}
				for _, c := range []struct{ path, body, want string }{
					{"/config", "", "map[a:map[b:map[]]] [[] [[]]]"},
					{"/cycle", "", "true"},
					{"/", `{"x":{"y":{}},"z":null}`, `map[x:map[y:map[]] z:map[]] {"x":{"y":{}},"z":null} false`},
					// go run: an UnmarshalTypeError, and the rest is kept
					{"/", `{"x":1,"y":{"z":{}}}`, `map[x:map[] y:map[z:map[]]] {"x":null,"y":{"z":{}}} true`},
				} {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("POST", c.path, strings.NewReader(c.body)))
					if got := w.Body.String(); w.Code != http.StatusOK || got != c.want {
						t.Errorf("%s %s: got %d %q, want 200 %q", c.path, c.body, w.Code, got, c.want)
					}
				}
			})
		}
	}
}

// TestPluginAsyncCallbackGoexit checks that runtime.Goexit in a callback
// that native code runs on its own goroutine ends that goroutine only, as
// in Go: the deferred calls run, and GoPanic gets nothing.
func TestPluginAsyncCallbackGoexit(t *testing.T) {
	panics := make(chan any, 1)
	p, err := Load(Options{Dir: "..", GoPanic: func(r any) { panics <- r }}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "timergoexit")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Body.String(); got != "done" {
		t.Errorf("got %q, want %q (the deferred call of the callback must run)", got, "done")
	}
	select {
	case r := <-panics:
		t.Errorf("GoPanic got %v, want nothing", r)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestPluginSyncCallbackPanic checks that a panic in a callback that native
// code calls on the goroutine of the plugin (the less function of
// sort.Slice) still goes up to the plugin, where recover gets it, as in Go.
// The same holds for the sequence of iter.Pull, which the VM runs on a
// goroutine of its own: its panic goes to the caller of next.
func TestPluginSyncCallbackPanic(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"sort", "recovered: less boom"},
		{"pull", "recovered: pull boom"},
	} {
		p, err := Load(Options{Dir: "..", GoPanic: func(r any) { t.Errorf("%s: GoPanic got %v", tc.name, r) }}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, tc.name)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if got := rec.Body.String(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestPluginNativeRecover checks that a panic in a handler that net/http
// runs on a goroutine of its own and recovers (nativeRecovers) goes to
// net/http, as with go run, and not to GoPanic:
//   - http.TimeoutHandler panics again on the goroutine of its ServeHTTP,
//     where the plugin can recover it, or the host gets it (also for the end
//     of the budget);
//   - the HTTP/1 and HTTP/2 servers abort the response on
//     http.ErrAbortHandler: the client gets an error, not a 200.
func TestPluginNativeRecover(t *testing.T) {
	modes := []struct {
		name string
		opts func(goPanic func(any)) Options
	}{
		{"default", func(gp func(any)) Options { return Options{Dir: "..", Budget: 1e6, GoPanic: gp} }},
		{"sandbox", func(gp func(any)) Options {
			return Options{Dir: "..", Sandbox: &Sandbox{Budget: 1e6, GoPanic: gp}}
		}},
		{"aot", func(gp func(any)) Options { return Options{Dir: "..", AOT: true, Budget: 1e6, GoPanic: gp} }},
	}
	cases := []struct {
		name      string
		body      string // the body of the response, when ServeHTTP returns
		hostPanic string // the panic that ServeHTTP gives to the host, printed
	}{
		{name: "timeoutrecover", body: "recovered: inner boom"},
		{name: "timeoutpanic", hostPanic: "inner boom"},
		{name: "timeoutloop", hostPanic: ErrBudget.Error()},
		{name: "serve1", body: "client error"},
		{name: "serve2", body: "client error"},
	}
	for _, mode := range modes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				p, err := Load(mode.opts(func(r any) { t.Errorf("GoPanic got %v", r) }), "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
				if err != nil {
					t.Fatal(err)
				}
				cfg, _ := createConfig(p)
				h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, tc.name)
				if err != nil {
					t.Fatal(err)
				}
				rec := httptest.NewRecorder()
				r := func() (r any) {
					defer func() { r = recover() }()
					h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
					return nil
				}()
				if tc.hostPanic != "" {
					if s := fmt.Sprint(r); r == nil || !strings.Contains(s, tc.hostPanic) {
						t.Errorf("host got panic %v (code %d, body %q), want a panic with %q", r, rec.Code, rec.Body.String(), tc.hostPanic)
					}
					return
				}
				if r != nil {
					t.Fatalf("host got panic %v", r)
				}
				if got := rec.Body.String(); got != tc.body {
					t.Errorf("got %q, want %q", got, tc.body)
				}
			})
		}
	}
}

// TestPluginRecoverHandler checks Options.RecoverHandler: a panic of
// ServeHTTP (of the plugin, or the end of its budget) goes to GoPanic and
// the response gets the status 500; nothing panics out of ServeHTTP, also
// on a goroutine of the host without a recover, where the panic ended the
// process. Without the option, the host gets the panic, as in Go
// (TestPluginNativeRecover).
func TestPluginRecoverHandler(t *testing.T) {
	for _, tc := range []struct{ name, goPanic string }{
		{"timeoutpanic", "inner boom"},
		{"timeoutloop", ErrBudget.Error()},
	} {
		for _, aot := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/aot=%v", tc.name, aot), func(t *testing.T) {
				var mu sync.Mutex
				var got []string
				opts := Options{Dir: "..", AOT: aot, Budget: 1e6, RecoverHandler: true, GoPanic: func(r any) {
					mu.Lock()
					defer mu.Unlock()
					got = append(got, fmt.Sprint(r))
				}}
				p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
				if err != nil {
					t.Fatal(err)
				}
				cfg, _ := createConfig(p)
				h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, tc.name)
				if err != nil {
					t.Fatal(err)
				}
				rec := httptest.NewRecorder()
				done := make(chan struct{})
				go func() { // no recover here: a panic would end the test binary
					defer close(done)
					h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
				}()
				<-done
				if rec.Code != http.StatusInternalServerError {
					t.Errorf("code %d, want 500", rec.Code)
				}
				mu.Lock()
				defer mu.Unlock()
				if len(got) != 1 || !strings.Contains(got[0], tc.goPanic) {
					t.Errorf("GoPanic got %q, want one panic with %q", got, tc.goPanic)
				}
			})
		}
	}
}

// TestPluginNativeNextPanic checks the panic of a plugin B that a plugin A
// calls as its next from a goroutine that A starts (B is the next
// middleware, or the same plugin twice in a chain). In Go, the process ends.
// Here, nothing on that goroutine gives the panic back to the host, so it
// goes to the GoPanic of A, the plugin whose goroutine runs the call:
//   - fcginext: native code (net/http/fcgi) calls next on a goroutine of
//     its own and does not recover;
//   - timernext: a callback of A on a timer goroutine calls next;
//   - gonext: A runs go next.ServeHTTP(...);
//   - fcgitimeout, fcgitimeoutnext: A gives a TimeoutHandler of its own
//     handler or of next to fcgi; TimeoutHandler recovers the panic on its
//     goroutine, but panics again on the fcgi goroutine;
//   - fcgictx: the host gives the handler of B to A in the context of the
//     request, and A gives it to fcgi;
//   - pullstop: a timer calls the stop func of iter.Pull (a VM intrinsic),
//     and the sequence panics;
//   - fcgifs, fcgiread, fcgiproxy: A gives fcgi a native handler that calls
//     a method of A with results that panics (Open of a FileServer, Read of
//     its file, RoundTrip of a ReverseProxy). Zero results crashed the host
//     (nil File, nil Response) or looped for ever (io.ReadFull and (0, nil)):
//     the method returns an error now, and fcgi closes the connection;
//   - fcgiisdir: the same with a method without an error result (IsDir of
//     the FileInfo of a file): the handler that fcgi runs ends;
//   - fcgidefault: A gives nil to fcgi (http.DefaultServeMux), where the
//     host registered a TimeoutHandler whose handler panics;
//   - timeronce: a timer calls the func of sync.OnceFunc, which panics again
//     with the value it saved, with no interpreted code on its stack.
//
// When the host calls B through A (A returns its next, passnext), the
// panic still goes to the host, as before. Each case runs in a child
// process, because the panic ended the test binary before the fix.
func TestPluginNativeNextPanic(t *testing.T) {
	if c := os.Getenv("YAEGI_NATIVE_NEXT"); c != "" {
		nativeNextChild(t, c)
		return
	}
	cases := []struct {
		name string
		want string // a line the child prints
		body string // the body that A writes, when not empty
		n    int    // the number of panics
	}{
		{"fcginext", "outer GoPanic: panic: inner chain boom", "", 1},
		{"timernext", "outer GoPanic: panic: inner chain boom", "", 1},
		{"gonext", "outer GoPanic: panic: inner chain boom", "", 1},
		{"passnext", "host panic: panic: inner chain boom", "", 1},
		{"fcgitimeout", "outer GoPanic: panic: own timeout boom", "", 1},
		{"fcgitimeoutnext", "outer GoPanic: panic: inner chain boom", "", 1},
		{"fcgictx", "outer GoPanic: panic: inner chain boom", "", 1},
		// The panic goes through native code (the coroutine of iter.Pull,
		// sync.OnceFunc), which gets the value of the program, as in gc
		// (nativePanic): the host gets that value, not the VM's panic.
		{"pullstop", "outer GoPanic: stop boom", "", 1},
		{"fcgifs", "outer GoPanic: panic: open boom", "done", 1},
		// Two calls of Read: http.ServeContent ignores the error of the
		// sniff read, then copies the file.
		{"fcgiread", "outer GoPanic: panic: read boom", "done", 2},
		// IsDir has no error result: fcgi gets the abort of the handler.
		{"fcgiisdir", "outer GoPanic: panic: isdir boom", "done", 1},
		{"fcgiproxy", "outer GoPanic: panic: roundtrip boom", "done", 1},
		{"fcgidefault", "outer GoPanic: host boom", "done", 1},
		{"timeronce", "outer GoPanic: once boom", "recovered: once boom", 1},
		// net/http calls these methods of a plugin net.Conn outside of
		// the recover of (*conn).serve. RemoteAddr and LocalAddr have no
		// error result: the conn goroutine ends (runtime.Goexit, before
		// serve reads the request; the plugin's client times out). String
		// gives panicString, Close an error.
		{"connremote", "outer GoPanic: panic: remoteaddr boom", "write pipe: i/o timeout", 1},
		{"connaddrstring", "outer GoPanic: panic: addrstring boom", "HTTP/1.0 200 OK hello", 1},
		{"connlocal", "outer GoPanic: panic: localaddr boom", "write pipe: i/o timeout", 1},
		{"connclose", "outer GoPanic: panic: conn close boom", "HTTP/1.0 200 OK hello", 1},
		// Not HTTP: the rpc client reads with a bufio.Reader on a
		// goroutine of its own; Read gets errCallbackPanic.
		{"rpcread", "outer GoPanic: panic: rpc read boom", "call error", 1},
		{"dnserror", "outer GoPanic: panic: error method boom", "lookup error", 1},
	}
	for _, mode := range []string{"default", "sandbox", "aot"} {
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				cmd := exec.Command(os.Args[0], "-test.run=^TestPluginNativeNextPanic$", "-test.count=1", "-test.v")
				cmd.Env = append(os.Environ(), "YAEGI_NATIVE_NEXT="+mode+"/"+tc.name)
				out, err := cmd.CombinedOutput()
				if err != nil || !strings.Contains(string(out), tc.want+"\n") || strings.Count(string(out), tc.want+"\n") != tc.n || strings.Count(string(out), "GoPanic:")+strings.Count(string(out), "host panic:") != tc.n {
					t.Errorf("child: %v, want the line %q (%d times) and no other panic, output:\n%s", err, tc.want, tc.n, out)
				}
				if body := fmt.Sprintf("body: %q\n", tc.body); tc.body != "" && !strings.Contains(string(out), body) {
					t.Errorf("child: want the line %q, output:\n%s", body, out)
				}
			})
		}
	}
}

// nativeNextChild runs one case of TestPluginNativeNextPanic: mode/name.
// It prints what the host and each GoPanic get.
func nativeNextChild(t *testing.T, c string) {
	mode, name, _ := strings.Cut(c, "/")
	panics := make(chan string, 4)
	load := func(who string) *Plugin {
		gp := func(r any) { panics <- fmt.Sprintf("%s GoPanic: %v", who, r) }
		opts := Options{Dir: "..", Budget: 1e6, GoPanic: gp}
		switch mode {
		case "sandbox":
			opts = Options{Dir: "..", Sandbox: &Sandbox{Budget: 1e6, GoPanic: gp}}
		case "aot":
			opts.AOT = true
		}
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	inner, outer := load("inner"), load("outer")
	cfg, _ := createConfig(inner)
	next, err := newHandler(context.Background(), inner, http.NotFoundHandler(), cfg, "innerpanic")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ = createConfig(outer)
	h, err := newHandler(context.Background(), outer, next, cfg, name)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	if name == "fcgidefault" {
		// A host that registers a handler in the default mux.
		http.Handle("/", http.TimeoutHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("host boom")
		}), 10*time.Second, "timeout"))
	}
	if name == "fcgictx" {
		// A host that gives the handler of a plugin to plugins.
		//lint:ignore SA1029 the plugin reads this key: a key type of the host cannot be named in interpreted code
		req = req.WithContext(context.WithValue(req.Context(), "handler", next))
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("host panic: %v\n", r)
			}
		}()
		h.ServeHTTP(rec, req)
	}()
	fmt.Printf("body: %q\n", rec.Body.String())
	timeout := time.After(10 * time.Second)
	if name == "passnext" {
		timeout = time.After(200 * time.Millisecond)
	}
	n := 0
	for {
		select {
		case s := <-panics:
			fmt.Println(s)
			if n++; n == 10 {
				return // a callback that panics in a loop
			}
			timeout = time.After(200 * time.Millisecond) // wait for a second one
		case <-timeout:
			return
		}
	}
}

// TestPluginCallbackResults checks what a func of the plugin with results
// gives when it panics, called by native code on a goroutine without
// interpreted code (here a goroutine of the host). Its panic goes to
// GoPanic, and zero results are not safe for the caller (io.ReadFull loops
// on (0, nil), FileServer uses a nil File): a func whose last result is an
// error returns errCallbackPanic, any other func with results ends the
// goroutine with runtime.Goexit (its deferred calls run).
func TestPluginCallbackResults(t *testing.T) {
	panics := make(chan any, 4)
	p, err := Load(Options{Dir: "..", GoPanic: func(r any) { panics <- r }}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "slots")
	if err != nil {
		t.Fatal(err)
	}
	var fint func() int
	var finterr func() (int, error)
	//lint:ignore SA1029 the plugin reads this key: a key type of the host cannot be named in interpreted code
	ctx := context.WithValue(context.Background(), "int", &fint)
	//lint:ignore SA1029 the plugin reads this key
	ctx = context.WithValue(ctx, "interr", &finterr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil).WithContext(ctx))
	if rec.Body.String() != "stored" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "stored")
	}
	wantPanic := func(want string) {
		t.Helper()
		select {
		case r := <-panics:
			if s := fmt.Sprint(r); !strings.Contains(s, want) {
				t.Errorf("GoPanic got %q, want %q", s, want)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("GoPanic got nothing, want %q", want)
		}
	}

	type result struct {
		n        int
		err      error
		returned bool
	}
	done := make(chan result, 1)
	go func() {
		var res result
		defer func() { done <- res }()
		res.n, res.err = finterr()
		res.returned = true
	}()
	wantPanic("interr boom")
	if res := <-done; !res.returned || res.n != 0 || !errors.Is(res.err, errCallbackPanic) {
		t.Errorf("func() (int, error): got %+v, want (0, errCallbackPanic)", res)
	}

	go func() {
		var res result
		defer func() { done <- res }()
		res.n = fint()
		res.returned = true
	}()
	wantPanic("int boom")
	if res := <-done; res.returned {
		t.Errorf("func() int: returned %d, want runtime.Goexit", res.n)
	}
}
