package host

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/vm"
)

// writeModule writes a plugin module example.com/plug with the source src
// and returns its directory.
func writeModule(t *testing.T, src string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "plug")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/plug\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plug.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const plugHead = `package plug

import (
	"context"
	"encoding/json"
	"net/http"
)

var _ json.Marshaler

type Config struct{ Name string }

`

const goodMiddleware = plugHead + `func CreateConfig() *Config { return &Config{} }

type handler struct{ name string }

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { w.Write([]byte(h.name)) }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{cfg.Name + "/" + name}, nil
}
`

const goodProvider = plugHead + `func CreateConfig() *Config { return &Config{} }

type Provider struct{ name string }

func (p *Provider) Init() error { return nil }

func (p *Provider) Provide(cfgChan chan<- json.Marshaler) error { return nil }

func (p *Provider) Stop() error { return nil }

func New(ctx context.Context, cfg *Config, name string) (*Provider, error) {
	return &Provider{name}, nil
}
`

// TestLoadKind checks LoadKind and CheckEntryPoints: a plugin without the
// entry points of its kind (CreateConfig, New with the parameters the host
// gives it, the methods of a provider) fails at load with an *EntryError,
// which is a *vm.LoadError, as under yaegi (Traefik reads New and
// CreateConfig when it builds the plugin). Before, the host found out at
// the first New, at each router build.
func TestLoadKind(t *testing.T) {
	ctx := context.Background()
	load := func(src string, kind Kind) (*vm.Plugin, error) {
		t.Helper()
		opts := Options()
		opts.Dir = writeModule(t, src)
		return LoadKind(ctx, opts, "example.com/plug", kind)
	}
	p, err := load(goodMiddleware, Middleware)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetFields(map[string]any{"Name": "cfg"}); err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(ctx, p, http.NotFoundHandler(), cfg, "mw")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "cfg/mw" {
		t.Errorf("body %q, want %q", rec.Body.String(), "cfg/mw")
	}

	p, err = load(goodProvider, Provider)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err = CreateConfig(p); err != nil {
		t.Fatal(err)
	}
	o, err := NewProvider(ctx, p, cfg, "prov")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct {
		name string
		args []any
	}{{"Init", nil}, {"Provide", []any{make(chan<- json.Marshaler)}}, {"Stop", nil}} {
		if res, err := CallMethod(o, m.name, m.args...); err != nil || len(res) != 1 || res[0] != nil {
			t.Errorf("%s: %v, %v", m.name, res, err)
		}
	}

	for _, c := range []struct {
		name, src string
		kind      Kind
		want      string
	}{
		{"provider as middleware", goodProvider, Middleware, "New is func(ctx context.Context, cfg *example.com/plug.Config, name string) (*example.com/plug.Provider, error), want func(ctx context.Context, next http.Handler"},
		{"middleware as provider", goodMiddleware, Provider, "want func(ctx context.Context, config C, name string) (P, error)"},
		{"no CreateConfig", strings.Replace(goodMiddleware, "func CreateConfig()", "func createConfig()", 1), Middleware, "package example.com/plug has no function CreateConfig"},
		{"no New", strings.Replace(goodMiddleware, "func New(", "func new2(", 1), Middleware, "package example.com/plug has no function New"},
		{"CreateConfig with a parameter", strings.Replace(goodMiddleware, "func CreateConfig() *Config { return &Config{} }", "func CreateConfig(n int) *Config { return &Config{} }", 1), Middleware, "CreateConfig is func(n int)"},
		{"another config type", strings.Replace(goodMiddleware, "func CreateConfig() *Config { return &Config{} }", "func CreateConfig() Config { return Config{} }", 1), Middleware, "New takes a *example.com/plug.Config, but CreateConfig returns a example.com/plug.Config"},
		{"not a handler", strings.Replace(strings.Replace(goodMiddleware, "func (h handler) ServeHTTP", "func (h handler) Serve", 1), ") (http.Handler, error) {", ") (handler, error) {", 1), Middleware, "New returns a example.com/plug.handler, which is not an http.Handler"},
		{"next needs more than ServeHTTP", strings.Replace(goodMiddleware, "next http.Handler", "next interface {\n\thttp.Handler\n\tFlush()\n}", 1), Middleware, "want func(ctx context.Context, next http.Handler"},
		{"New returns a string", strings.Replace(goodMiddleware, ") (http.Handler, error) {\n\treturn handler{cfg.Name + \"/\" + name}, nil", ") (string, error) {\n\treturn name, nil", 1), Middleware, "New returns a string, which is not an http.Handler"},
		{"no Stop", strings.Replace(goodProvider, "func (p *Provider) Stop()", "func (p *Provider) stop()", 1), Provider, "has no method Stop"},
		{"Init without error", strings.Replace(goodProvider, "func (p *Provider) Init() error { return nil }", "func (p *Provider) Init() {}", 1), Provider, "the method Init of *example.com/plug.Provider is func(), want 0 parameters and an error result"},
		{"Provider by value", strings.Replace(goodProvider, ") (*Provider, error) {\n\treturn &Provider{name}, nil", ") (Provider, error) {\n\treturn Provider{name}, nil", 1), Provider, "New returns a example.com/plug.Provider, which has no method Init"},
	} {
		_, err := load(c.src, c.kind)
		var ee *EntryError
		var le *vm.LoadError
		if !errors.As(err, &ee) || !errors.As(err, &le) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an *EntryError (and a *vm.LoadError) with %q", c.name, err, c.want)
		}
	}
}

// TestEntryShapes checks middleware shapes that Traefik with yaegi runs
// (it calls New with reflect and asserts the result to http.Handler):
// LoadKind accepts them, and NewHandler runs them with the configuration.
func TestEntryShapes(t *testing.T) {
	ctx := context.Background()
	base := plugHead + `func CreateConfig() *Config { return &Config{} }

type handler struct{ name string }

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { w.Write([]byte(h.name)) }
`
	for _, c := range []struct{ name, src string }{
		// The config is given to an interface parameter.
		{"cfg is any", base + `func New(ctx context.Context, next http.Handler, cfg any, name string) (http.Handler, error) {
	return handler{cfg.(*Config).Name + "/" + name}, nil
}
`},
		// The handler is checked at New.
		{"New returns any", base + `func New(ctx context.Context, next http.Handler, cfg *Config, name string) (any, error) {
	return handler{cfg.Name + "/" + name}, nil
}
`},
		// http.Handler is assignable to the parameters.
		{"next is an unnamed interface", base + `func New(ctx context.Context, next interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, cfg *Config, name string) (http.Handler, error) {
	return handler{cfg.Name + "/" + name}, nil
}
`},
		{"ctx and next are any", base + `func New(ctx any, next any, cfg *Config, name string) (http.Handler, error) {
	return handler{cfg.Name + "/" + name}, nil
}
`},
	} {
		opts := Options()
		opts.Dir = writeModule(t, c.src)
		p, err := LoadKind(ctx, opts, "example.com/plug", Middleware)
		if err != nil {
			t.Errorf("%s: LoadKind: %v", c.name, err)
			continue
		}
		cfg, err := CreateConfig(p)
		if err != nil {
			t.Errorf("%s: CreateConfig: %v", c.name, err)
			continue
		}
		if err := cfg.SetFields(map[string]any{"Name": "cfg"}); err != nil {
			t.Errorf("%s: SetFields: %v", c.name, err)
			continue
		}
		h, err := NewHandler(ctx, p, http.NotFoundHandler(), cfg, "mw")
		if err != nil {
			t.Errorf("%s: NewHandler: %v", c.name, err)
			continue
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Body.String() != "cfg/mw" {
			t.Errorf("%s: body %q, want %q", c.name, rec.Body.String(), "cfg/mw")
		}
	}
}

// TestNoGoModMessage checks that the error of a plugin directory without a
// go.mod names the directory: load errors replace the plugin directory
// with "." (they show the host directories by short names), so before it
// read "no go.mod in . or its parents", and the host could not tell which
// plugin it was.
func TestNoGoModMessage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myplugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	opts := Options()
	opts.Dir = dir
	_, err := Load(opts, "example.com/myplugin")
	if err == nil || !strings.Contains(err.Error(), `named "myplugin"`) || strings.Contains(err.Error(), dir) {
		t.Errorf("Load: %v, want a no go.mod error that names myplugin, not the full path", err)
	}
}
