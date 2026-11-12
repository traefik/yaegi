// Package sideeffecttest tests yaegi v2 in a host that imports
// stdsyms/sideeffect, the tables of the standard library packages whose
// init changes the host (expvar, net/http/pprof, testing/quick). It is a
// package of its own so that the test binary of package vm keeps the
// host's http.DefaultServeMux and flag.CommandLine as they are.
package sideeffecttest

import (
	"context"
	"expvar"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	_ "github.com/traefik/yaegi/v2/stdsyms/sideeffect"
	"github.com/traefik/yaegi/v2/vm"
)

const plugin = "github.com/traefik/yaegi/v2/vm/testdata/hostregvarplug"

// hostOnlyPath is a route of the host's http.DefaultServeMux: the plugins
// must not see it.
const hostOnlyPath = "/hostreg-host-only"

var hostOnlyOnce sync.Once

// TestPluginHostRegistriesSideEffect is the expvar and net/http/pprof part
// of vm.TestPluginHostRegistries. Restricted plugins may not import
// net/http/pprof, so the plugin loads only in the modes without restrictions
// (Unrestricted, with and without -aot). RealProcess is not tested: a
// machine with the real standard library uses the host's tables, as in
// Go. Each instance must see its own expvar table and its own
// /debug/vars and /debug/pprof/ routes, the second one must not panic on
// a duplicate, and the host must see nothing.
func TestPluginHostRegistriesSideEffect(t *testing.T) {
	hostOnlyOnce.Do(func() {
		http.HandleFunc(hostOnlyPath, func(http.ResponseWriter, *http.Request) {})
	})
	modes := []struct {
		name string
		opts vm.Options
	}{
		{"unrestricted", vm.Options{Unrestricted: true}},
		{"unrestricted aot", vm.Options{Unrestricted: true, AOT: true}},
	}
	kinds := []struct {
		kind string
		want []string // %d: the number of requests of the instance
		host func(t *testing.T)
	}{
		{"expvar", []string{
			`host route: ""`,
			"expvar: %d true",
			"expvar.Do: cmdline hostreg_f hostreg_func hostreg_hits hostreg_m hostreg_s memstats",
			"/debug/vars: 200 true true",
		}, func(t *testing.T) {
			expvar.Do(func(kv expvar.KeyValue) {
				if strings.HasPrefix(kv.Key, "hostreg") {
					t.Errorf("the host expvar has the variable of a plugin: %s", kv.Key)
				}
			})
		}},
		{"pprof", []string{`host route: ""`, `pprof route: "/debug/pprof/cmdline"`, "/debug/pprof/: 200 true"}, func(*testing.T) {}},
	}
	for _, mode := range modes {
		mode.opts.Dir = "../.."
		for _, k := range kinds {
			t.Run(mode.name+"/"+k.kind, func(t *testing.T) {
				var hs []http.Handler
				for i := range 2 {
					p, err := vm.Load(mode.opts, plugin)
					if err != nil {
						t.Fatalf("load %d: %v", i, err)
					}
					cfg, err := pluginhost.CreateConfig(p)
					if err != nil {
						t.Fatal(err)
					}
					if err := cfg.SetFields(map[string]any{"Kind": k.kind}); err != nil {
						t.Fatal(err)
					}
					h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "hostreg")
					if err != nil {
						t.Fatalf("New %d: %v", i, err)
					}
					hs = append(hs, h)
				}
				want := strings.Join(k.want, "\n") + "\n"
				// The first instance serves two requests, the second one:
				// each one counts its own expvar hits.
				for i, h := range []http.Handler{hs[0], hs[0], hs[1]} {
					w := httptest.NewRecorder()
					r := httptest.NewRequest("GET", "/", nil)
					r.Header.Set("X-Host-Path", hostOnlyPath)
					h.ServeHTTP(w, r)
					want := want
					if strings.Contains(want, "%d") {
						want = fmt.Sprintf(want, []int{1, 2, 1}[i])
					}
					if got := w.Body.String(); got != want {
						t.Errorf("request %d: the plugin sees\n%s\nwant\n%s", i, got, want)
					}
				}
				k.host(t)
			})
		}
	}
}

// TestRestrictedRefusesSideEffect checks that the restricted modes refuse
// the plugin of TestPluginHostRegistriesSideEffect at load time, even when
// the host imports stdsyms/sideeffect: it imports net/http/pprof. Its
// import of expvar is allowed (TestRestrictedExpvar).
func TestRestrictedRefusesSideEffect(t *testing.T) {
	modes := []struct {
		name string
		opts vm.Options
	}{
		{"default", vm.Options{}},
		{"sandbox", vm.Options{Sandbox: &vm.Sandbox{}}},
		{"aot", vm.Options{AOT: true}},
		{"sandbox aot", vm.Options{Sandbox: &vm.Sandbox{}, AOT: true}},
		{"sandbox unrestricted", vm.Options{Sandbox: &vm.Sandbox{}, Unrestricted: true}},
	}
	for _, mode := range modes {
		mode.opts.Dir = "../.."
		t.Run(mode.name, func(t *testing.T) {
			_, err := vm.Load(mode.opts, plugin)
			if err == nil {
				t.Fatal("the plugin loaded")
			}
			if want := `import "net/http/pprof" is not allowed in a plugin`; !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
			if strings.Contains(err.Error(), `"expvar"`) {
				t.Errorf("error %q refuses expvar", err)
			}
		})
	}
}

// TestRestrictedExpvar loads a plugin that imports expvar alone (as
// prometheus/client_golang does) in the restricted modes.
// Each instance has a table of its own that starts
// empty: no cmdline and no memstats of the host in Get, Do and
// /debug/vars. Two instances publish the same names without a panic, a
// name published twice in one instance panics in the plugin as in Go, and
// the host sees none of the names.
func TestRestrictedExpvar(t *testing.T) {
	const plug = "github.com/traefik/yaegi/v2/vm/testdata/expvarplug"
	const dup = "Reuse of exported var name: expvarplug_hits\n" // log.Panicln, as expvar.Publish in Go
	modes := []struct {
		name string
		opts vm.Options
	}{
		{"default", vm.Options{}},
		{"sandbox", vm.Options{Sandbox: &vm.Sandbox{}}},
		{"aot", vm.Options{AOT: true}},
		{"sandbox aot", vm.Options{Sandbox: &vm.Sandbox{}, AOT: true}},
		{"sandbox unrestricted", vm.Options{Sandbox: &vm.Sandbox{}, Unrestricted: true}},
	}
	for _, mode := range modes {
		mode.opts.Dir = "../.."
		t.Run(mode.name, func(t *testing.T) {
			var hs []http.Handler
			for i := range 2 {
				p, err := vm.Load(mode.opts, plug)
				if err != nil {
					t.Fatalf("load %d: %v", i, err)
				}
				cfg, err := pluginhost.CreateConfig(p)
				if err != nil {
					t.Fatal(err)
				}
				h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "expvarplug")
				if err != nil {
					t.Fatalf("New %d: %v", i, err)
				}
				hs = append(hs, h)
			}
			for i, h := range []http.Handler{hs[0], hs[0], hs[1]} {
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
				want := fmt.Sprintf("duplicate: %s\nget: %d true true 3\ndo: expvarplug_func expvarplug_hits expvarplug_m\n/debug/vars: 200 true false false\n",
					dup, []int{1, 2, 1}[i])
				if got := w.Body.String(); got != want {
					t.Errorf("request %d: the plugin sees\n%s\nwant\n%s", i, got, want)
				}
			}
			expvar.Do(func(kv expvar.KeyValue) {
				if strings.HasPrefix(kv.Key, "expvarplug") {
					t.Errorf("the host expvar has the variable of a plugin: %s", kv.Key)
				}
			})
		})
	}
}

// TestRestrictedExpvarSrc checks that a restricted plugin cannot reach the
// host's expvar table with net/http interpreted (Options.Modes, Src): the
// machine has no table of its own for a native expvar, and the native
// functions are the host's. The plugin is refused at load time.
func TestRestrictedExpvarSrc(t *testing.T) {
	_, err := vm.Load(vm.Options{Dir: "../..", Modes: map[string]vm.Mode{"net/http": vm.ModeInterp}}, "github.com/traefik/yaegi/v2/vm/testdata/expvarplug")
	if want := "it would use the host's expvar table"; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("load: %v, want an error with %q", err, want)
	}
}
