package vm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// TestPluginCleanupPanic checks that a cleanup of runtime.AddCleanup cannot
// end the host. The runtime runs the cleanup on a goroutine of its own, where
// nothing recovers: a panic, ErrBudget or ErrStackOverflow of the cleanup
// must go to Options.GoPanic, as for a goroutine of the plugin. Each case
// runs in a child process (this test binary), so that a crash is seen.
func TestPluginCleanupPanic(t *testing.T) {
	if c := os.Getenv("VM_CLEANUP_CHILD"); c != "" {
		mode, path, _ := strings.Cut(c, " ")
		got := make(chan any, 1)
		p, err := vm.Load(vm.Options{
			Dir:     "..",
			AOT:     mode == "aot",
			Budget:  10_000_000,
			GoPanic: func(r any) { got <- r },
		}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/cleanupplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := pluginhost.CreateConfig(p)
		h, err := pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "cleanup")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		deadline := time.After(30 * time.Second)
		for {
			runtime.GC()
			select {
			case r := <-got:
				fmt.Printf("body %q\ngopanic %v\nhost still alive\n", w.Body.String(), r)
				return
			case <-deadline:
				t.Fatal("the cleanup did not run")
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	for _, c := range []struct{ path, panic string }{
		{"/panic", "panic: cleanup boom"}, // a *vm.PanicError, as for a go statement
		{"/loop", vm.ErrBudget.Error()},
		{"/deep", vm.ErrStackOverflow.Error()},
	} {
		for _, mode := range []string{"default", "aot"} {
			cmd := exec.Command(os.Args[0], "-test.run=^TestPluginCleanupPanic$", "-test.v")
			cmd.Env = append(os.Environ(), "VM_CLEANUP_CHILD="+mode+" "+c.path)
			out, err := cmd.CombinedOutput()
			want := fmt.Sprintf("body %q\ngopanic %v\nhost still alive\n", "added", c.panic)
			if err != nil || !strings.Contains(string(out), want) {
				t.Errorf("%s %s: child process: %v, output:\n%s\nwant:\n%s", mode, c.path, err, out, want)
			}
		}
	}
}
