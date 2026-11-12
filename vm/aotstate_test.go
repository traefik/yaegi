package vm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"weak"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// serve loads the plugin in dir in AOT mode and returns the body of one
// request, and the plugin's machine.
func serve(t *testing.T, dir string) (string, *vm.Machine) {
	t.Helper()
	p, err := vm.Load(vm.Options{Dir: dir, AOT: true}, "example.com/plug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pluginhost.CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "plug")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	return w.Body.String(), p.M
}

// TestAOTStatePerPlugin: two plugins with the same import path and the same
// type names, different fields, both encoding through the AOT copy of
// encoding/json in one process, do not see each other's types.
func TestAOTStatePerPlugin(t *testing.T) {
	for i := 0; i < 2; i++ {
		if got, _ := serve(t, "testdata/aotstate/a"); got != `{"name":"a","count":1} a` {
			t.Errorf("plugin a: %s", got)
		}
		if got, _ := serve(t, "testdata/aotstate/b"); got != `{"port":8080,"tags":["x"]} x` {
			t.Errorf("plugin b: %s", got)
		}
	}
}

// TestAOTUnload loads and drops plugins N times: the AOT caches indexed by
// type (encoding/json, encoding/xml, encoding/binary) must not keep the
// machines alive, and the heap must stay stable. encoding/gob still does
// (docs/AOTGEN.md).
func TestAOTUnload(t *testing.T) {
	if testing.Short() {
		t.Skip("loads plugins many times")
	}
	for _, c := range []struct{ dir, want string }{
		{"testdata/aotstate/a", `{"name":"a","count":1} a`},
		{"testdata/aotstate/c", "<Payload><Name>c</Name><Count>3</Count></Payload> c 6"},
	} {
		t.Run(c.dir, func(t *testing.T) {
			const n = 20
			var machines []weak.Pointer[vm.Machine]
			heap := func() uint64 {
				for i := 0; i < 3; i++ {
					runtime.GC()
				}
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				return ms.HeapAlloc
			}
			var base uint64
			for i := 0; i < n; i++ {
				body, m := serve(t, c.dir)
				if body != c.want {
					t.Fatalf("body %q, want %q", body, c.want)
				}
				machines = append(machines, weak.Make(m))
				if i == 2 {
					base = heap()
				}
			}
			end := heap()
			alive := 0
			for _, w := range machines {
				if w.Value() != nil {
					alive++
				}
			}
			t.Logf("heap after 3 loads %d KB, after %d loads %d KB; machines alive: %d", base>>10, n, end>>10, alive)
			if alive > 1 {
				t.Errorf("%d of %d unloaded machines are still alive", alive, n)
			}
			if end > base+base/4 {
				t.Errorf("heap grew from %d KB to %d KB over %d loads", base>>10, end>>10, n-3)
			}
		})
	}
}
