package vm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginSharedMap runs ServeHTTP of a plugin in parallel. Each request
// gives the shared config map and slice to native code that only reads them
// (json.Marshal through a pointer, a json.Unmarshal that does not reach
// them). yaegi v2 must not write them back: before the fix, the map was
// cleared and filled again after each call, so the map was lost or the
// process died with "concurrent map writes". Run it with -race.
func TestPluginSharedMap(t *testing.T) {
	const want = `{"Headers":{"a":"1","b":"2","c":"3"},"Names":["x","y","z"]} q 3 2 3 x,y,z`
	for _, aot := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/sharedmap")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "shared")
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan string, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 300 {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
					if got := w.Body.String(); w.Code != http.StatusOK || got != want {
						errs <- got
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for got := range errs {
			t.Errorf("AOT %v: got %q, want %q", aot, got, want)
		}
	}
}

// TestPluginSharedZero runs ServeHTTP of a plugin in parallel. Each request
// gives config maps with values equal by == but not the same (0 and -0,
// MyInt(1) and 1 in a map[string]any) to json.Marshal, which only reads
// them. Before the fix, yaegi v2 took the value of another key with an equal
// value, saw a change and wrote the shared map: "fatal error: concurrent map
// iteration and map write". Run it with -race.
func TestPluginSharedZero(t *testing.T) {
	const want = "bad 0"
	for _, aot := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/sharedzero")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "zero")
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan string, 4)
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 200 {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
					if got := w.Body.String(); w.Code != http.StatusOK || got != want {
						errs <- got
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for got := range errs {
			t.Errorf("AOT %v: got %q, want %q", aot, got, want)
		}
	}
}

// TestPluginSharedFunc runs ServeHTTP of a plugin in parallel. Each request
// gives a config with funcs (in a map, a slice, an interface and a struct
// behind a pointer) to json.Marshal, which does not read them. Before the
// fix, all the closures made for native code had the same Pointer(), so
// yaegi v2 took the func of another key, saw a change and wrote the shared
// map: "fatal error: concurrent map iteration and map write". Run it with
// -race.
func TestPluginSharedFunc(t *testing.T) {
	runSharedPlugin(t, "sharedfunc", 4, 200)
}

// TestPluginSharedNaN runs ServeHTTP of a plugin in parallel. Each request
// gives a config map with a NaN key to json.Marshal, which only reads it.
// Before the fix, yaegi v2 cleared and filled again every map with a NaN key
// after the call: "fatal error: concurrent map read and map write". Run it
// with -race.
func TestPluginSharedNaN(t *testing.T) {
	runSharedPlugin(t, "sharednan", 4, 200)
}

// TestPluginSharedMix runs ServeHTTP of a plugin in parallel, for each
// field of its config alone (see vm/testdata/sharedmix): funcs and NaN keys
// in structs behind pointers, in slices, maps and interfaces. json.Marshal
// only reads them. Before the fix, the struct behind a pointer, the map of
// structs and the []any of structs (an unexported func field) were written
// back after each call (a data race), and a struct in the []any lost its
// interpreted type. Run it with -race.
func TestPluginSharedMix(t *testing.T) {
	for _, f := range []string{"P", "SL", "SM", "SP", "NN", "I", "AR", "NS", "NI", "IS"} {
		t.Run(f, func(t *testing.T) { runSharedPluginNamed(t, "sharedmix", f, 4, 100) })
	}
}

// TestPluginSharedIfacePtr runs ServeHTTP of a plugin in parallel. Each
// request gives json.Marshal a config with pointers in an interface, a
// []*W, a map[string]*W and a []any, where each W holds an interpreted
// named value (T, N) in an interface. json.Marshal only reads them. Before
// the fix, the write-back of the pointers held by interfaces (and of the
// kept slice and map elements) wrote the pointed cells: a data race, and
// T{1} became struct { X int } (N(3) became int), so v.(T) failed. Run it
// with -race.
func TestPluginSharedIfacePtr(t *testing.T) {
	runSharedPlugin(t, "sharedifaceptr", 4, 50)
}

// runSharedPlugin loads the plugin vm/testdata/<name> (default and AOT
// modes) and runs its ServeHTTP from n goroutines, rounds times each. Every
// response must be "bad 0".
func runSharedPlugin(t *testing.T, name string, n, rounds int) {
	t.Helper()
	runSharedPluginNamed(t, name, name, n, rounds)
}

// runSharedPluginNamed is runSharedPlugin with the name inst given to New.
func runSharedPluginNamed(t *testing.T, name, inst string, n, rounds int) {
	t.Helper()
	const want = "bad 0"
	for _, aot := range []bool{false, true} {
		p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/"+name)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, inst)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan string, n)
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range rounds {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
					if got := w.Body.String(); w.Code != http.StatusOK || got != want {
						errs <- got
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for got := range errs {
			t.Errorf("AOT %v: got %q, want %q", aot, got, want)
		}
	}
}
