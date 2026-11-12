package vm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginIfacePtr decodes JSON into a pointer to an interface that holds
// a *struct, as the helper of astappiev/traefik-umami-feeder does: the
// struct must be filled, with json.Unmarshal and json.Decoder.Decode, in
// default mode and in AOT mode.
func TestPluginIfacePtr(t *testing.T) {
	for _, aot := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/ifaceptrplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "ifaceptr")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if got, want := w.Body.String(), "tok-123 tok-456"; w.Code != http.StatusOK || got != want {
			t.Errorf("AOT %v: got %d %q, want 200 %q", aot, w.Code, got, want)
		}
	}
}
