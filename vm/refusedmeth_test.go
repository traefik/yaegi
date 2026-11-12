package vm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestRefusedMethods checks that a restricted plugin cannot call
// (*os.File).Chdir without naming it (refusedmeth.go): through its reflect,
// an interface, a promoted method and the templates, in the default mode
// and in AOT mode. The working directory of the host must not change.
func TestRefusedMethods(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	dir := t.TempDir()
	paths := []string{
		"/methodbyname", "/method", "/methodinterface", "/typemethod", "/typemethodindex",
		"/interface", "/embedreflect", "/template", "/templatemap", "/htmltemplate",
	}
	changePaths := []string{"/templatetree", "/htmltemplatetree", "/templateadd", "/templatenested"}
	// What the native templates give for changePaths: the copy runs without
	// the change, the added template is not in the copy, a template may not
	// call Execute. In default mode the plugin gets the AOT copies of the
	// templates when the host has them (stdAOTModes): nativeTmpl keeps the
	// native ones.
	nativeTmpl := map[string]Mode{"text/template": ModeNative, "html/template": ModeNative}
	wantDefault := map[string]string{
		"/templateadd":    `template "x" not defined`,
		"/templatenested": "the method Execute, which is not allowed in a plugin",
	}
	// serve runs path and returns the panic or the error of the plugin, and
	// whether the working directory changed.
	serve := func(h http.Handler, path string) (msg string, changed bool) {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
			now, _ := os.Getwd()
			changed = now != wd
			os.Chdir(wd)
		}()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path+"?dir="+dir, nil))
		return rec.Body.String(), false
	}
	load := func(opts Options) http.Handler {
		t.Helper()
		opts.Dir = ".."
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/chdirplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "chdir")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	for _, opts := range []Options{{}, {Modes: nativeTmpl}, {AOT: true}} {
		aot := opts.AOT
		h := load(opts)
		for _, path := range paths {
			msg, changed := serve(h, path)
			if changed {
				t.Errorf("aot %v, %s: the working directory of the host changed", aot, path)
			}
			if !strings.Contains(msg, "Chdir") || !strings.Contains(msg, "is not allowed in a plugin") {
				t.Errorf("aot %v, %s: got %q, want a refusal of Chdir", aot, path, msg)
			}
		}
		// A template that calls no refused method still runs.
		if msg, changed := serve(h, "/templateok"); changed || msg != dir {
			t.Errorf("aot %v, /templateok: got %q (changed %v), want %q", aot, msg, changed, dir)
		}
	}
	// The plugin changes the templates while they run (a FuncMap function
	// changes a tree or adds a template), or a template executes another
	// template. The working directory must not change. The native
	// templates run a private copy of the templates, so the change has no
	// effect or the added template is not found; the AOT copies (-aot, and
	// default mode with the copies) refuse the call to Chdir.
	for _, opts := range []Options{{}, {Modes: nativeTmpl}, {AOT: true}} {
		h := load(opts)
		for _, path := range changePaths {
			msg, changed := serve(h, path)
			if changed {
				t.Errorf("aot %v, modes %v, %s: the working directory of the host changed (%q)", opts.AOT, opts.Modes, path, msg)
			}
			if want := wantDefault[path]; opts.Modes != nil && !strings.Contains(msg, want) {
				t.Errorf("native templates, %s: got %q, want %q", path, msg, want)
			}
		}
	}
	// Without the restrictions, the method runs as in Go.
	h := load(Options{Unrestricted: true})
	for _, path := range append(paths, changePaths...) {
		if msg, changed := serve(h, path); !changed {
			t.Errorf("unrestricted, %s: the working directory did not change (%q)", path, msg)
		}
	}
}

// TestRefusedMethodsComplete checks that each method of refusedFuncs is
// refused at run time too.
func TestRefusedMethodsComplete(t *testing.T) {
	var native []string
	for _, what := range refusedNative {
		native = append(native, what)
	}
	for name := range refusedFuncs {
		if strings.HasPrefix(name, "(") && !slices.Contains(native, name) {
			t.Errorf("%s is in refusedFuncs but not in refusedNative", name)
		}
	}
	for k := range refusedMethods() {
		if _, ok := k.t.MethodByName(k.name); !ok {
			t.Errorf("%v has no method %s", k.t, k.name)
		}
	}
}

// TestHostImport checks that a host that imports package vm keeps its
// http.DefaultServeMux and its flag.CommandLine as they are: the tables of
// expvar, net/http/pprof and testing/quick are in stdsyms/sideeffect.
func TestHostImport(t *testing.T) {
	out, err := exec.Command("go", "run", "./testdata/hostimport").CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	if len(out) > 0 {
		t.Errorf("importing vm changed the global state of the host:\n%s", out)
	}
}
