package vm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginPackageStateIsolation loads the same plugin twice, in each
// mode: package variables (set by init functions in dependency order) are
// shared by the handlers of one load, and not shared between two loads.
func TestPluginPackageStateIsolation(t *testing.T) {
	for _, aot := range []bool{false, true} {
		t.Run(fmt.Sprint("aot=", aot), func(t *testing.T) {
			load := func() *vm.Plugin {
				p, err := vm.Load(vm.Options{Dir: "..", AOT: aot, Env: []string{}}, "github.com/traefik/yaegi/v2/vm/testdata/isoplug")
				if err != nil {
					t.Fatal(err)
				}
				return p
			}
			serve := func(p *vm.Plugin, name string) func() string {
				cfg, err := pluginhost.CreateConfig(p)
				if err != nil {
					t.Fatal(err)
				}
				h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, name)
				if err != nil {
					t.Fatal(err)
				}
				return func() string {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
					return w.Body.String()
				}
			}
			p1, p2 := load(), load()
			a := serve(p1, "a")
			b := serve(p1, "b")
			c := serve(p2, "c")
			for _, step := range []struct {
				h    func() string
				want string
			}{
				{a, "a 101 2 var,init1,init2"},
				{b, "b 102 2 var,init1,init2"},
				{a, "a 103 2 var,init1,init2"},
				{c, "c 101 1 var,init1,init2"},
				{b, "b 104 2 var,init1,init2"},
				{c, "c 102 1 var,init1,init2"},
			} {
				if got := step.h(); got != step.want {
					t.Errorf("got %q, want %q", got, step.want)
				}
			}
		})
	}
}
