package shims_test

import (
	"context"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/host"
	"github.com/traefik/yaegi/v2/shims"
	"github.com/traefik/yaegi/v2/vm"
)

// TestSources checks that each shim parses, with the build constraints of
// the file it replaces.
func TestSources(t *testing.T) {
	for _, sh := range shims.All() {
		f, err := parser.ParseFile(token.NewFileSet(), sh.File, sh.Src, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s: %v", sh.File, err)
		}
		if f.Name.Name == "" || len(sh.SHA256) == 0 || sh.Why == "" {
			t.Errorf("%s: package %q, %d sums, why %q", sh.File, f.Name.Name, len(sh.SHA256), sh.Why)
		}
	}
}

// TestDolthubMaphash loads a restricted plugin that uses
// github.com/dolthub/maphash v0.1.0 (vendored) with both loaders. With the
// shim, its hashers work and Plugin.Notes names the file; without it, the
// call fails (the VM has no runtime map type).
func TestDolthubMaphash(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=vendor")
	dir := filepath.Join("testdata", "maphashplug")
	for _, goList := range []bool{false, true} {
		opts := host.Options()
		opts.Dir, opts.Root, opts.UseUnsafe, opts.GoList, opts.AOT = dir, dir, true, goList, false
		got, notes, err := serve(opts)
		if err != nil {
			t.Fatalf("GoList %v: %v", goList, err)
		}
		if got != "true true true true true\n" {
			t.Errorf("GoList %v: body %q", goList, got)
		}
		if len(notes) != 1 || !strings.Contains(notes[0], "runtime.go: replaced by a shim") {
			t.Errorf("GoList %v: notes %q", goList, notes)
		}

		opts.Shims = nil
		if got, notes, err := serve(opts); err == nil && got == "true true true true true\n" {
			t.Errorf("GoList %v: without the shim, body %q, notes %q", goList, got, notes)
		}
	}
}

// serve loads the plugin, serves one request and returns the body.
func serve(opts vm.Options) (body string, notes []string, err error) {
	p, err := vm.Load(opts, "example.com/maphashplug")
	if err != nil {
		return "", nil, err
	}
	defer p.Close()
	cfg, err := host.CreateConfig(p)
	if err != nil {
		return "", p.Notes, err
	}
	h, err := host.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		return "", p.Notes, err
	}
	defer func() {
		if r := recover(); r != nil {
			body = ""
		}
	}()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	return rec.Body.String(), p.Notes, nil
}
