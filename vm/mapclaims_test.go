package vm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginMapClaims decodes JWT claims as golang-jwt/jwt/v5 does: a non nil
// MapClaims in an interface, filled with json.Unmarshal and
// json.Decoder.Decode through a pointer to a copy of the map. The interface
// must see the decoded claims, in default mode and in AOT mode
// (agilezebra/jwt-middleware refused every token).
func TestPluginMapClaims(t *testing.T) {
	for _, aot := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/mapclaims")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "jwt")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if got, want := w.Body.String(), "traefik aud,exp;traefik aud,exp"; w.Code != http.StatusOK || got != want {
			t.Errorf("AOT %v: got %d %q, want 200 %q", aot, w.Code, got, want)
		}
	}
}
