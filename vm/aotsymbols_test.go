package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/internal/srchash"
)

// TestAOTMissingSymbol: a function of a package run in AOT mode that the
// AOT symbol table does not have is a load error, not a panic during the
// first request that calls it ("native func ... not in registry").
func TestAOTMissingSymbol(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "vendor", "example.com", "aotlib")
	os.MkdirAll(lib, 0o755)
	os.WriteFile(filepath.Join(lib, "lib.go"), []byte("package aotlib\n\nfunc A() string { return \"src-a\" }\n\nfunc B() string { return \"src-b\" }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "vendor", "modules.txt"), []byte("# example.com/aotlib v1.0.0\n## explicit; go 1.21\nexample.com/aotlib\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/plug\n\ngo 1.22\n\nrequire example.com/aotlib v1.0.0\n"), 0o644)
	plugin := func(call string) {
		os.WriteFile(filepath.Join(dir, "plug.go"), []byte(`package plug

import (
	"context"
	"io"
	"net/http"

	"example.com/aotlib"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type P struct{}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return P{}, nil
}

func (P) ServeHTTP(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `+call+`) }
`), 0o644)
	}
	h, err := srchash.Dir(lib)
	if err != nil {
		t.Fatal(err)
	}
	// The AOT table has A only: the copy of an older version, say.
	RegisterAOT(&Package{
		Path: "example.com/aotlib", AOTPath: "example.com/aotlib/aotcopy",
		Module: "example.com/aotlib", Version: "v1.0.0", Sources: map[string]string{"example.com/aotlib": h},
		Funcs: map[string]reflect.Value{"A": reflect.ValueOf(func() string { return "aot-a" })},
	})
	t.Cleanup(func() {
		regMu.Lock()
		delete(aotPkgs, "example.com/aotlib")
		delete(aotOrig, "example.com/aotlib/aotcopy")
		regMu.Unlock()
	})

	plugin("aotlib.A()")
	p, err := Load(Options{Dir: dir, AOT: true}, "example.com/plug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	hd, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "plug")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	hd.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if got := w.Body.String(); got != "aot-a" {
		t.Errorf("A() = %q, want the AOT copy's %q", got, "aot-a")
	}

	plugin("aotlib.A() + aotlib.B()")
	_, err = Load(Options{Dir: dir, AOT: true}, "example.com/plug")
	if err == nil || !strings.Contains(err.Error(), "example.com/aotlib.B") || strings.Contains(err.Error(), "aotlib.A") {
		t.Errorf("load error %v, want one about example.com/aotlib.B only", err)
	}
	// Without AOT the package is interpreted: both functions work.
	if _, err := Load(Options{Dir: dir}, "example.com/plug"); err != nil {
		t.Errorf("load without AOT: %v", err)
	}
}
