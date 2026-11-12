package vm

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"testing"
)

// TestRoutedFuncValues checks that a restricted plugin gets its own version
// of the routed functions and methods (env.go, exit.go, stdio.go,
// flagvars.go, hostregs.go) when it reaches them through a value and not a
// direct call: a func value, a func in a map, a func that native code
// calls, a method value, a method expression, reflect. The host keeps its
// environment, its default loggers, its flags and its http.DefaultServeMux,
// and os.Exit and log.Fatal panic instead of ending the
// test.
func TestRoutedFuncValues(t *testing.T) {
	const want = `env: value map reflect
cleared: 0
exit value: panic os.Exit(3)
exit map: panic os.Exit(4)
exit reflect: panic os.Exit(5)
exit native: panic os.Exit(6)
fatal value: panic a
fatal method value: panic b
fatal method expression: panic c
fatal reflect: panic d
log: "log\n" true
parse method value: panic os.Exit(2)
parse method expression: panic os.Exit(2)
flag: true
mux: /funcval
after: after
`
	t.Setenv("FUNCVAL_HOST", "host")
	log0, flags0, slog0 := log.Writer(), log.Flags(), slog.Default()
	for _, c := range []struct {
		name string
		opts Options
	}{
		{"default", Options{}},
		{"aot", Options{AOT: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := c.opts
			opts.Dir = ".."
			opts.Env = []string{"FUNCVAL_HOST=plugin"}
			p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/funcvalplug")
			if err != nil {
				t.Fatal(err)
			}
			res, err := p.Call(context.Background(), "Run")
			if err != nil {
				t.Fatal(err)
			}
			v, err := res[0].Interface()
			if err != nil {
				t.Fatal(err)
			}
			if got := v.(string); got != want {
				t.Errorf("the plugin got\n%s\nwant\n%s", got, want)
			}
			if got := os.Getenv("FUNCVAL_HOST"); got != "host" {
				t.Errorf("host FUNCVAL_HOST = %q, want host", got)
			}
			for _, k := range []string{"FUNCVAL_MAP", "FUNCVAL_REFLECT", "FUNCVAL_AFTER"} {
				if v, ok := os.LookupEnv(k); ok {
					t.Errorf("the plugin set %s=%q in the host", k, v)
				}
			}
			if log.Writer() != log0 || log.Flags() != flags0 || slog.Default() != slog0 {
				t.Errorf("the plugin changed the host loggers")
			}
			if lvl := slog.SetLogLoggerLevel(slog.LevelInfo); lvl != slog.LevelInfo {
				t.Errorf("the plugin changed the level of the host log bridge to %v", lvl)
			}
			if flag.Lookup("funcval-flag") != nil {
				t.Errorf("the plugin defined a flag of the host")
			}
			if _, pattern := http.DefaultServeMux.Handler(&http.Request{Method: "GET", URL: &url.URL{Path: "/funcval"}}); pattern != "" {
				t.Errorf("the plugin registered %q in the host's http.DefaultServeMux", pattern)
			}
		})
	}
}
