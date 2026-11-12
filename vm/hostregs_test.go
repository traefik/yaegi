package vm

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"database/sql"
	"errors"
	"image"
	"mime"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// hostOnlyPath is a route of the host's http.DefaultServeMux: the plugins
// must not see it.
const hostOnlyPath = "/hostreg-host-only"

var hostOnlyOnce sync.Once

// rpcGo is the rpc line of the plugin, as in Go. In default mode too: a
// plugin that imports net/rpc runs it as its AOT copy (stdAOTTriggers),
// which sees the methods of the plugin's types.
const rpcGo = "rpc call: 5 <nil> (register: <nil>)"

// TestPluginHostRegistries loads two instances of a plugin that writes to
// one process-wide registry of the standard library (http.DefaultServeMux,
// database/sql, mime, image, net/rpc, http.DefaultClient and
// http.DefaultTransport, crypto.RegisterHash, x509.SetFallbackRoots,
// archive/zip), in each mode. Each instance must see its own
// registrations, the second one must not panic on a duplicate, and the
// host must see nothing. expvar and net/http/pprof are tested in
// vm/sideeffecttest: restricted plugins may not import them, and the
// other modes need a host that imports stdsyms/sideeffect.
func TestPluginHostRegistries(t *testing.T) {
	hostOnlyOnce.Do(func() {
		http.HandleFunc(hostOnlyPath, func(http.ResponseWriter, *http.Request) {})
	})
	idle0 := http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost
	modes := []struct {
		name string
		opts Options
		rpc  string
	}{
		{"default", Options{}, rpcGo},
		{"unrestricted", Options{Unrestricted: true}, rpcGo},
		{"sandbox", Options{Sandbox: &Sandbox{}}, rpcGo},
		{"aot", Options{AOT: true}, rpcGo},
		{"sandbox aot", Options{Sandbox: &Sandbox{}, AOT: true}, rpcGo},
	}
	kinds := []struct {
		kind string
		want []string
		host func(t *testing.T)
	}{
		{"mux", []string{
			"mux: plugin mux",
			`host route: ""`,
			"httptest.NewServer(nil): 200 plugin route",
			"http.Serve(l, nil): 200 plugin route",
			"Server{}.Serve: 200 plugin route",
			"httptest.Server{}.Start: 200 plugin route",
			"NewUnstartedServer, nil Handler, Start: 200 plugin route",
			"NewUnstartedServer, nil Handler, StartTLS: 200 plugin route",
			"late nil Handler: 200 plugin route, host route: 404, nil: true",
			"Server.Serve, late nil Handler: 200 plugin route, nil: true",
			"Server.Serve, whole struct: 404",
		}, func(t *testing.T) { checkHostRoutes(t, "/hostreg", "/hostreg/handle", "/hostreg/mux") }},
		{"sql", []string{"sql: true hostreg: no conn dsn", "sql.Drivers: true"}, func(t *testing.T) {
			if slices.Contains(sql.Drivers(), "hostregdrv") {
				t.Errorf("the host has the sql driver of a plugin: %q", sql.Drivers())
			}
			for _, d := range sql.Drivers() {
				if strings.HasPrefix(d, sqlPrefixTag) {
					t.Errorf("sql.Drivers of the host shows the plugin driver %q", d)
				}
			}
		}},
		{"mime", []string{`mime: "text/x-hostreg; charset=utf-8" "text/x-hostreg; charset=utf-8" [".hostreg"] <nil>`}, func(t *testing.T) {
			if typ := mime.TypeByExtension(".hostreg"); typ != "" {
				t.Errorf("the host mime table has the type of a plugin: %q", typ)
			}
		}},
		{"image", []string{"image.Decode: hostreg (0,0)-(12,1)", "image.DecodeConfig: hostreg 7 <nil>"}, func(t *testing.T) {
			if _, f, err := image.Decode(strings.NewReader("HOSTREG data")); !errors.Is(err, image.ErrFormat) {
				t.Errorf("the host image.Decode uses the format of a plugin: %q %v", f, err)
			}
		}},
		{"rpc", []string{`rpc route: "/_goRPC_"`, "{rpc call}"}, func(t *testing.T) { checkHostRoutes(t, "/_goRPC_", "/debug/rpc") }},
		{"client", []string{"client: 42s 77"}, func(t *testing.T) {
			if c := http.DefaultClient.Timeout; c != 0 {
				t.Errorf("a plugin changed the host http.DefaultClient: Timeout %v", c)
			}
			if n := http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost; n != idle0 {
				t.Errorf("a plugin changed the host http.DefaultTransport: MaxIdleConnsPerHost %d", n)
			}
		}},
		{"crypto", []string{"crypto: 7 true"}, func(t *testing.T) {
			if n := crypto.SHA256.New().Size(); n != 32 {
				t.Errorf("a plugin replaced the SHA-256 of the host: size %d", n)
			}
		}},
		{"x509", []string{"x509: registered"}, func(*testing.T) {}},
		{"zip", []string{"zip: true hello"}, func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			if _, err := zw.CreateHeader(&zip.FileHeader{Name: "a", Method: 99}); err != zip.ErrAlgorithm {
				t.Errorf("the host zip writer uses the compressor of a plugin: %v", err)
			}
		}},
	}
	for _, mode := range modes {
		mode.opts.Dir = ".."
		for _, k := range kinds {
			t.Run(mode.name+"/"+k.kind, func(t *testing.T) {
				var hs []http.Handler
				for i := range 2 {
					p, err := Load(mode.opts, "github.com/traefik/yaegi/v2/vm/testdata/hostregplug")
					if err != nil {
						t.Fatalf("load %d: %v", i, err)
					}
					cfg, err := createConfig(p)
					if err != nil {
						t.Fatal(err)
					}
					if err := cfg.SetFields(map[string]any{"Kind": k.kind}); err != nil {
						t.Fatal(err)
					}
					h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "hostreg")
					if err != nil {
						t.Fatalf("New %d: %v", i, err)
					}
					hs = append(hs, h)
				}
				lines := k.want
				if raceEnabled && k.kind == "mux" {
					// the plugin leaves out the writes of a running server,
					// data races in Go (X-Race, hostregplug.go)
					lines = slices.DeleteFunc(slices.Clone(lines), func(l string) bool {
						return strings.Contains(l, "late nil Handler") || strings.Contains(l, "whole struct")
					})
				}
				want := strings.Join(lines, "\n") + "\n"
				want = strings.Replace(want, "{rpc call}", mode.rpc, 1)
				// The first instance serves two requests, the second one.
				for i, h := range []http.Handler{hs[0], hs[0], hs[1]} {
					w := httptest.NewRecorder()
					r := httptest.NewRequest("GET", "/", nil)
					r.Header.Set("X-Host-Path", hostOnlyPath)
					if raceEnabled {
						r.Header.Set("X-Race", "1")
					}
					h.ServeHTTP(w, r)
					if got := w.Body.String(); got != want {
						t.Errorf("request %d: the plugin sees\n%s\nwant\n%s", i, got, want)
					}
				}
				k.host(t)
			})
		}
	}
}

// checkHostRoutes checks that the host's http.DefaultServeMux has none of
// the routes of the plugin.
func checkHostRoutes(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, p := http.DefaultServeMux.Handler(httptest.NewRequest("GET", path, nil)); p != "" {
			t.Errorf("the host mux has the route %q of a plugin", p)
		}
	}
}
