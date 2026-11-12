package vm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// concplugWant is the body that the concplug plugin must write for id.
func concplugWant(id int) string {
	sum := 0
	for i := range 10 {
		sum += (id + i) * (id + i)
	}
	words := []string{"c" + strconv.Itoa(id), "a" + strconv.Itoa(id+1), "b" + strconv.Itoa(id+2)}
	sort.Strings(words)
	errField := ""
	if id%2 == 1 {
		errField = `,"err":"checked: odd id"`
	}
	return fmt.Sprintf(`{"id":%d,"sum":%d,"score":%d,"words":["%s"],"cached":"v%d"%s}`+"\n",
		id, sum, id%7, strings.Join(words, `","`), id%10, errField)
}

// TestPluginConcurrentHandler sends concurrent requests to a plugin whose
// handler starts goroutines, uses channels, select, mutexes, atomics,
// sync.Map, a generic cache, recover and json on every request, in each
// plugin mode, with and without the guard rails (budget, goroutine cap). Every response is checked, and the shared state
// must count every request. Run it with -race: it covers the VM state that
// requests share (goroutine table, budget accounts, type caches, the shadows
// of the AOT copies).
func TestPluginConcurrentHandler(t *testing.T) {
	for _, c := range []struct {
		name string
		opts vm.Options
	}{
		{"default", vm.Options{}},
		{"opaque", vm.Options{OpaqueAny: true}},
		{"aot", vm.Options{AOT: true}},
		{"aot-budget", vm.Options{AOT: true, Budget: 1_000_000, MaxGoroutines: 10_000}},
		{"default-budget", vm.Options{Budget: 1_000_000, MaxGoroutines: 10_000}},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := c.opts
			opts.Dir = ".."
			opts.Env = []string{}
			p, err := vm.Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/concplug")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := pluginhost.CreateConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "conc")
			if err != nil {
				t.Fatal(err)
			}
			const n, reqs = 8, 25
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
						if got, want := w.Body.String(), concplugWant(id); w.Code != 200 || got != want {
							bad[g] = fmt.Sprintf("/%d: status %d, got %q, want %q", id, w.Code, got, want)
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
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/stats", nil))
			if got, want := w.Body.String(), fmt.Sprintf("total=%d paths=50 seen=50", n*reqs); got != want {
				t.Errorf("stats: got %q, want %q", got, want)
			}
		})
	}
}
