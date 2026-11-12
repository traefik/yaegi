package vm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginRTypeRace sends the first requests of a fresh plugin at the same
// time, as Traefik does just after a plugin loads. Each request runs a
// handler that boxes one of 8 types into interfaces for the first time
// (lazy compile), then calls its methods through interfaces, a type switch
// and fmt (native code that calls String). Before the fix, the run-time type
// descriptor was visible to other goroutines before its method table was
// filled: the process died with "concurrent map read and map write", or a
// method call found no callee. Run it with -race.
func TestPluginRTypeRace(t *testing.T) {
	const n = 64
	for round := range 6 {
		p, err := vm.Load(vm.Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/rtyperace")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pluginhost.CreateConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "race")
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		got := make([]string, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						got[i] = fmt.Sprint("panic: ", r)
					}
				}()
				<-start
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/%d", i), nil))
				got[i] = w.Body.String()
			}()
		}
		close(start)
		wg.Wait()
		for i := range n {
			if want := rtypeRaceWant(i); got[i] != want {
				t.Errorf("round %d, /%d: got %q, want %q", round, i, got[i], want)
			}
		}
	}
}

// rtypeRaceWant is the output of handler i of testdata/rtyperace in Go.
func rtypeRaceWant(i int) string {
	typ, f := i%8, i/8
	if f%2 == 0 {
		return fmt.Sprintf("t%d:%d 0 v%d <%d:%d>;", typ, i, typ, typ, i)
	}
	v := i + 1
	return fmt.Sprintf("t%d:%d %d p%d <%d:%d>;", typ, v, v*(typ+1), typ, typ, v)
}

// TestRegisterNativeRace registers native hooks while a plugin compiles in
// AOT mode (compile looks the hooks up). Before the fix, RegisterNative wrote
// the hook map with no lock. Run it with -race.
func TestRegisterNativeRace(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			vm.RegisterNative(fmt.Sprintf("example.com/none.F%d", i%16), func(*vm.Machine, []vm.Value) vm.Value { return vm.Value{} })
		}
	}()
	defer func() {
		close(stop)
		<-done
	}()
	p, err := vm.Load(vm.Options{Dir: "..", AOT: true}, "github.com/traefik/yaegi/v2/vm/testdata/sharedmap")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pluginhost.CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "hooks")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}
