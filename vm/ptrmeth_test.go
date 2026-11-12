package vm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginJSONPointerMethods decodes JSON into types with pointer methods
// defined in a subpackage of the plugin, shaped like the Response of
// negasus/traefik-plugin-bridge and the index of
// holysoles/bot-wrangler-traefik-plugin. Before
// a fix they stayed empty and json returned no error (New then
// failed with "empty index"); a String method made json refuse the value.
func TestPluginJSONPointerMethods(t *testing.T) {
	want := strings.Join([]string{
		"index bot,crawler",
		"response <nil> 201 map[A:1] hi",
		`tojson <nil> {"status":201,"headers":{"A":"1"},"body":"aGk="}`,
		"decode <nil> 202 map[B:2]",
		"named <nil> Named(n)",
		"upper <nil> UP",
		"index2 <nil> <nil> new,old",
	}, "\n")
	for _, aot := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/ptrmethplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "ptrmeth")
		if err != nil {
			t.Fatalf("AOT %v: New: %v", aot, err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"status":202,"headers":{"B":"2"}}`)))
		if got := w.Body.String(); w.Code != http.StatusOK || got != want {
			t.Errorf("AOT %v: got %d\n%s\nwant 200\n%s", aot, w.Code, got, want)
		}
	}
}
