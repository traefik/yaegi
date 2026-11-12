package vm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestAOTShadowRace sends concurrent requests to an AOT plugin that decodes
// JSON into a json.RawMessage field and encodes a shared config that holds
// one. The AOT copy of encoding/json reads and writes these fields through
// shadows of the VM cells. Before the fix, the shadows were one list per
// machine, stored in the cells: the native to VM transition of one request
// gave back the shadows that the AOT code of other requests was still
// writing, and the field came back empty (wrong JWT checks in
// agilezebra/jwt-middleware); json.Marshal of the shared config wrote its
// cells, and its keys came back null. Run it with -race.
func TestAOTShadowRace(t *testing.T) {
	p, err := vm.Load(vm.Options{Dir: "..", AOT: true}, "github.com/traefik/yaegi/v2/vm/testdata/shadowrace")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pluginhost.CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "shadow")
	if err != nil {
		t.Fatal(err)
	}
	const n, reqs = 8, 100
	var wg sync.WaitGroup
	start := make(chan struct{})
	bad := make([]string, n)
	for g := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					bad[g] = fmt.Sprint("panic: ", r)
				}
			}()
			<-start
			for i := range reqs {
				id := g*reqs + i
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/%d", id), nil))
				if got, want := w.Body.String(), fmt.Sprintf(`RS%d [%d,2,3] {"name":"jwks","keys":[4,5]}`, id, id); got != want {
					bad[g] = fmt.Sprintf("/%d: got %q, want %q", id, got, want)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	for g := range n {
		if bad[g] != "" {
			t.Errorf("goroutine %d: %s", g, bad[g])
		}
	}
}
