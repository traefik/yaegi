package vm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginHeaderRewrite runs a plugin that rewrites the values of the
// request headers, of the query and of the response headers in place
// (vs[i] = ... in a range loop over http.Header and url.Values), in
// parallel requests. The native next handler must see the new values, and
// the client the new response headers. Before the fix, the plugin wrote
// into copies and the changes were lost. Run it with -race: the plugin also
// reads a package-level http.Header from all the requests.
func TestPluginHeaderRewrite(t *testing.T) {
	for _, aot := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/hdrrewrite")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.SetFields(map[string]any{"From": "foo", "To": "bar"}); err != nil {
			t.Fatal(err)
		}
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := fmt.Sprintf("%v %v %v %s", r.Header["X-Foo"], r.Header["X-Single"], r.Header["X-Default"], r.URL.RawQuery)
			if want := "[bar-1 bar-2] [a-bar!] [own d2] k=bar&k=bar-bar"; got != want {
				http.Error(w, got, http.StatusTeapot)
				return
			}
			w.Header().Add("X-Out", "out-1")
			w.Header().Add("X-Out", "out-2")
		})
		h, err := pluginhost.NewHandler(context.Background(), p, next, cfg, "rewrite")
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan string, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 200 {
					r := httptest.NewRequest("GET", "/?k=foo&k=foo-foo", nil)
					r.Header.Add("X-Foo", "foo-1")
					r.Header.Add("X-Foo", "foo-2")
					r.Header.Set("X-Single", "a-foo")
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					if w.Code != http.StatusOK || !slices.Equal(w.Header()["X-Out"], []string{"OUT-1", "OUT-2"}) {
						errs <- fmt.Sprintf("%d %q %v", w.Code, w.Body.String(), w.Header())
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for got := range errs {
			t.Errorf("AOT %v: %s", aot, got)
		}
	}
}
