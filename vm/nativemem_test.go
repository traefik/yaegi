package vm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Tests of the memory checks before native calls (nativemem.go). Plugin:
// testdata/restricted/nativememplug.

const nativeMemPlug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/nativememplug"

// nativeMemChildEnv names the route of TestNativeMemChild ("" in the
// parent), and nativeMemAOTEnv makes it load in AOT mode.
const (
	nativeMemChildEnv = "YAEGI_NATIVE_MEM_ROUTE"
	nativeMemAOTEnv   = "YAEGI_NATIVE_MEM_AOT"
)

// TestNativeMem: with a quota of 64 MiB, the native calls whose result size
// is known from their arguments (strings.Repeat, bytes.Repeat,
// strings.Builder.Grow, bytes.Buffer.Grow, reflect.MakeSlice) and
// io.ReadAll panic with ErrMemory in the plugin when the result does not
// fit, and the plugin recovers it. Small results still work. Before, the
// VM did not count these allocations: a plugin that asked for 1 TiB ended
// the host with a fatal "out of memory", and io.ReadAll of an endless
// reader grew until the host died. The same check runs for
// io/ioutil.ReadAll, a function value, a method called through an
// interface and a deferred call, and a Grow that fits in the free capacity
// is not charged. Each case runs in a child process whose
// heap is watched (startHeapWatchdog), so a check that fails does not take
// down the machine.
func TestNativeMem(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("runs child processes")
	}
	const big = "268435456"      // 256 MiB: over the quota, but the host could allocate it
	const huge = "1099511627776" // 1 TiB: a fatal "out of memory" of the host
	const free = "31457280"      // 30 MiB: fits once, not twice with 40 MiB held
	tests := []struct{ route, want string }{
		{"/repeat?n=1000", "ok"},
		{"/repeat?n=" + big, ErrMemory.Error()},
		{"/repeat?n=" + huge, ErrMemory.Error()},
		{"/repeat8?n=" + huge, ErrMemory.Error()},
		{"/bytesrepeat?n=1000", "ok"},
		{"/bytesrepeat?n=" + huge, ErrMemory.Error()},
		{"/buildergrow?n=1000", "ok"},
		{"/buildergrow?n=" + huge, ErrMemory.Error()},
		{"/buffergrow?n=1000", "ok"},
		{"/buffergrow?n=" + huge, ErrMemory.Error()},
		{"/makeslice?n=1000", "ok"},
		{"/makeslice?n=" + huge, ErrMemory.Error()},
		{"/makeslicecap?n=" + huge, ErrMemory.Error()},
		{"/readall?n=1000", "ok"},
		{"/readall?n=1048576", "ok"},
		{"/readall?n=" + big, ErrMemory.Error()},
		{"/readallendless", ErrMemory.Error()},
		// io/ioutil.ReadAll calls io.ReadAll natively.
		{"/ioutilreadall", ErrMemory.Error()},
		// Calls that are not static: a function value, an interface method,
		// a deferred call.
		{"/repeatvalue?n=1000", "ok"},
		{"/repeatvalue?n=" + huge, ErrMemory.Error()},
		{"/readallvalue", ErrMemory.Error()},
		{"/growvalue?n=1000", "ok"},
		{"/growvalue?n=" + huge, ErrMemory.Error()},
		{"/buildergrowiface?n=1000", "ok"},
		{"/buildergrowiface?n=" + huge, ErrMemory.Error()},
		{"/buffergrowiface?n=1000", "ok"},
		{"/buffergrowiface?n=" + huge, ErrMemory.Error()},
		{"/repeatdefer?n=1000", "ok"},
		{"/repeatdefer?n=" + huge, ErrMemory.Error()},
		{"/readalldefer", ErrMemory.Error()},
		// Grow that fits in the free capacity allocates nothing: no
		// ErrMemory for a plugin near its quota.
		{"/buildergrowfree?n=" + free, "ok"},
		{"/buffergrowfree?n=" + free, "ok"},
	}
	for _, aot := range []bool{false, true} {
		for _, tt := range tests {
			name := tt.route
			if aot {
				name += "/aot"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeMemChild$", "-test.count=1")
				cmd.Env = append(os.Environ(), nativeMemChildEnv+"="+tt.route, "GOMEMLIMIT=1GiB")
				if aot {
					cmd.Env = append(cmd.Env, nativeMemAOTEnv+"=1")
				}
				out, err := cmd.CombinedOutput()
				want := "RESULT: " + tt.want + "\n"
				if err != nil || !strings.Contains(string(out), want) {
					t.Fatalf("child process (%v), want %q:\n%s", err, want, out)
				}
			})
		}
	}
}

// TestNativeMemChild is run by TestNativeMem in a child process: it serves
// one route of nativememplug with MaxMemory 64 MiB and prints the result.
func TestNativeMemChild(t *testing.T) {
	route := os.Getenv(nativeMemChildEnv)
	if route == "" {
		t.Skip("helper process of TestNativeMem")
	}
	startHeapWatchdog()
	p, err := Load(Options{Dir: "..", MaxMemory: 64 << 20, AOT: os.Getenv(nativeMemAOTEnv) != ""}, nativeMemPlug)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		t.Fatal(err)
	}
	body, r := serveBG(h, route)
	if r != nil {
		t.Fatalf("%s: panic %v", route, r)
	}
	fmt.Printf("RESULT: %s\n", body)
}
