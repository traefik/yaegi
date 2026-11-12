package vm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginAOTRoute loads a plugin with the plugin restrictions in a host
// that links the AOT copies (aot/all, imported by aotstate_test.go): fmt
// sees the plugin's types through its AOT copy, and the plugin's own
// reflect calls stay native, so Set of an unexported field of a host
// object panics with the message of native reflect.
func TestPluginAOTRoute(t *testing.T) {
	if len(vm.AOTPackages()) == 0 {
		t.Fatal("no AOT copy registered")
	}
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/routeplug"
	for _, golist := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", GoList: golist}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := pluginhost.CreateConfig(p)
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "route")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		want := "routeplug.T T!2 routeplug.P{A:3, B:4}|PUT http.Request|" +
			"reflect: reflect.Value.Set using value obtained using unexported field"
		if got := w.Body.String(); got != want {
			t.Errorf("go list %v:\n got %q\nwant %q", golist, got, want)
		}
	}
}
