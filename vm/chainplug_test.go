package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// chainWrapSrc wraps the ResponseWriter before it calls next, as
// rewrite-body, compression or cache plugins do.
const chainWrapSrc = `package p

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type rw struct{ http.ResponseWriter }

func (w rw) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(append([]byte("wrapped:"), b...))
}

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.next.ServeHTTP(rw{w}, r)
}
`

// chainLeafSrc writes the response with the ResponseWriter it gets.
const chainLeafSrc = `package p

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Leaf", "1")
	w.Write([]byte("leaf"))
}
`

// TestChainedPluginsWrappedWriter chains two plugins, each in its own
// machine, as Traefik does. The first one wraps the ResponseWriter and its
// next is the second one. The second machine got the wrapper of a value of
// the first machine and unwrapped it as one of its own values: each request
// panicked with "native type example.com/p.rw not in registry".
func TestChainedPluginsWrappedWriter(t *testing.T) {
	needSource(t)
	for _, aot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vm", true: "aot"}[aot], func(t *testing.T) {
			testChainedPlugins(t, Options{AOT: aot})
		})
	}
}

func testChainedPlugins(t *testing.T, opts Options) {
	ctx := context.Background()
	load := func(src string, next http.Handler) http.Handler {
		t.Helper()
		dir, pkgPath := writePlugin(t, src)
		o := opts
		o.Dir = dir
		p, err := Load(o, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := newHandler(ctx, p, next, cfg, "chain")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	leaf := load(chainLeafSrc, http.NotFoundHandler())
	h := load(chainWrapSrc, leaf)
	for range 3 {
		rec := httptest.NewRecorder()
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		}()
		if got := rec.Body.String(); got != "wrapped:leaf" {
			t.Fatalf("body %q, want %q", got, "wrapped:leaf")
		}
		if got := rec.Header().Get("X-Leaf"); got != "1" {
			t.Fatalf("X-Leaf header %q, want 1", got)
		}
	}
}
