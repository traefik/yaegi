package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPluginOnceValuePanic checks that the functions of sync.OnceValue and
// sync.OnceValues panic again with the same value on every call when f
// panics, as with go run, with both loaders and in both modes.
func TestPluginOnceValuePanic(t *testing.T) {
	const want = "recovered: boom\nrecovered: boom\nrecovered: boom 2\nrecovered: boom 2\n42 42 runs: 3\n" // go run
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
			p, err := Load(tc.opts, "github.com/traefik/yaegi/v2/vm/testdata/genonceplug")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "genonce")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestPluginGenericStd checks that a plugin can use generic types and
// functions of native standard library packages (atomic.Pointer,
// math/rand/v2.N, weak.Make) without importing slices, maps, iter or cmp,
// with both loaders.
func TestPluginGenericStd(t *testing.T) {
	const want = "cfg! true true true true true\n"
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
			p, err := Load(tc.opts, "github.com/traefik/yaegi/v2/vm/testdata/genstdplug")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := cfg.SetFields(map[string]any{"Name": "cfg"}); err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "genstd")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
