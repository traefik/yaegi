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

// TestAOTErrorsAsRace sends concurrent requests to an AOT plugin that looks
// for its own errors with errors.As and errors.Is. The wrappers of equal
// values share one payload (optPayload), and the type keeps the last one
// (RType.optLast) to find it fast. Before the fix, a request compared its
// value with the last payload of the type with the == of the VM, which
// reads the variable a pointer points to: the target of errors.As of
// another request, which the AOT errors.as was writing at the same time.
// The race detector stopped every host test that ran an AOT plugin. Run it
// with -race.
func TestAOTErrorsAsRace(t *testing.T) {
	p, err := vm.Load(vm.Options{Dir: "..", AOT: true}, "github.com/traefik/yaegi/v2/vm/testdata/errasrace")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pluginhost.CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "errasrace")
	if err != nil {
		t.Fatal(err)
	}
	const n, reqs = 64, 20
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
				want := fmt.Sprintf("[true op%d true %d true %d true %d true true true false false]", id, id, id, id)
				if got := w.Body.String(); got != want {
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
