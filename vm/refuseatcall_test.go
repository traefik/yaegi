package vm

import (
	"context"
	"strings"
	"testing"
)

// TestRefuseAtCall checks Options.RefuseAtCall: without it, a plugin that
// names refused functions, or imports plugin and net/http/pprof, is refused
// at load time; with it, the plugin loads, the code that calls no refused
// function runs, and each call of a refused function panics with a
// *RefusedError.
func TestRefuseAtCall(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/refuseatcallplug"
	_, err := Load(Options{Dir: ".."}, pkg)
	if err == nil {
		t.Fatal("without RefuseAtCall: the plugin loads")
	}
	for _, want := range []string{
		`import "plugin" is not allowed`, `import "net/http/pprof" is not allowed`,
		"os.FindProcess is not allowed", "runtime/debug.SetMemoryLimit is not allowed",
		"os/signal.Ignore is not allowed", "(*net.UnixConn).WriteMsgUnix is not allowed",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("without RefuseAtCall: %v\nwant %q", err, want)
		}
	}

	for _, opts := range []Options{
		{Dir: "..", RefuseAtCall: true},
		{Dir: "..", RefuseAtCall: true, AOT: true},
	} {
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatalf("AOT=%v: %v", opts.AOT, err)
		}
		for _, name := range []string{"Fine", "Value"} {
			res, err := p.Call(context.Background(), name)
			if err != nil {
				t.Fatalf("AOT=%v %s: %v", opts.AOT, name, err)
			}
			if v, _ := res[0].Interface(); v != true {
				t.Errorf("AOT=%v %s: %v, want true", opts.AOT, name, v)
			}
		}
		for name, what := range map[string]string{
			"FindProcess": "os.FindProcess",
			"CallValue":   "os.FindProcess",
			"MemoryLimit": "runtime/debug.SetMemoryLimit",
			"Ignore":      "os/signal.Ignore",
			"WriteMsg":    "(*net.UnixConn).WriteMsgUnix",
			"Open":        "plugin.Open",
			"Lookup":      "(*plugin.Plugin).Lookup",
			"Pprof":       "net/http/pprof.Handler",
		} {
			_, err := p.Call(context.Background(), name)
			if err == nil || !strings.Contains(err.Error(), what+" ") || !strings.Contains(err.Error(), "is not allowed in a plugin") {
				t.Errorf("AOT=%v %s: %v, want %s refused", opts.AOT, name, err, what)
			}
		}
	}
}
