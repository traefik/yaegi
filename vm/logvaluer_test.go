package vm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// TestLogValuer checks that the LogValue method of a plugin type
// (slog.LogValuer, a type that hides a secret in the logs) is called by the
// native log/slog: when a plugin without the AOT copy of log/slog logs the
// value, and when the host logs a value that the plugin gave it. Native slog
// got a native copy or a handle without the method, and wrote the fields of
// the value: the secret went to the host's log. In AOT mode, LogValue of the
// plugin returns the slog.Value of the copy: the host gets its text.
func TestLogValuer(t *testing.T) {
	const wantPlugin = `level=INFO msg=direct t=token-hidden n=named-hidden e=auth-hidden g.user=bob g.n=1
level=INFO msg=value t=token-hidden e=auth-hidden
is true`
	for _, c := range []struct {
		name      string
		opts      Options
		wantGroup string // how the host writes the group of Group
	}{
		// a plugin that imports log/slog runs the AOT copies when the host
		// has them (stdAOTModes)
		{"default", Options{}, `g="[user=bob n=1]"`},
		{"native-slog", Options{Modes: map[string]Mode{"log/slog": ModeNative}}, "g.user=bob g.n=1"},
		{"aot", Options{AOT: true}, `g="[user=bob n=1]"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := c.opts
			opts.Dir = ".."
			opts.Env = []string{}
			p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/logvaluerplug")
			if err != nil {
				t.Fatal(err)
			}
			res, err := p.Call(context.Background(), "Logs")
			if err != nil {
				t.Fatal(err)
			}
			v, err := res[0].Interface()
			if err != nil {
				t.Fatal(err)
			}
			if got := v.(string); got != wantPlugin {
				t.Errorf("the plugin logged\n%s\nwant\n%s", got, wantPlugin)
			}

			res, err = p.Call(context.Background(), "Values")
			if err != nil {
				t.Fatal(err)
			}
			v, err = res[0].Interface()
			if err != nil {
				t.Fatal(err)
			}
			vs := v.([]any)
			var buf bytes.Buffer
			l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			}}))
			l.Info("host", "t", vs[0], "n", vs[1], "e", vs[2], "g", vs[3])
			want := "level=INFO msg=host t=token-hidden n=named-hidden e=auth-hidden " + c.wantGroup + "\n"
			if got := buf.String(); got != want {
				t.Errorf("the host logged\n%s\nwant\n%s", got, want)
			}
			if strings.Contains(buf.String(), "pw-") {
				t.Errorf("the host logged a secret: %s", buf.String())
			}
			// The wrappers keep the other methods of the values.
			if s := fmt.Sprint(vs[1]); s != "named" {
				t.Errorf("String of Named: %q", s)
			}
			e, ok := vs[2].(error)
			if !ok {
				t.Fatalf("ErrAuth is a %T, not an error", vs[2])
			}
			if e.Error() != "auth failed" || errors.Unwrap(e) == nil {
				t.Errorf("ErrAuth: Error %q, Unwrap %v", e.Error(), errors.Unwrap(e))
			}
		})
	}
}

// TestLogValuerPanic checks that the host goroutine goes on when the host
// logs, with its native log/slog, a plugin value whose LogValue panics or
// runs out of budget. slog.Value.Resolve recovers the panic of LogValue and
// logs "LogValue panicked", as with go run (slogResolveName). Before, the
// outermost run of LogValue gave the panic to GoPanic and then ended the
// goroutine of the host (runtime.Goexit in afterPanic).
func TestLogValuerPanic(t *testing.T) {
	for _, c := range []struct {
		name string
		opts Options
	}{
		{"default", Options{}},
		{"aot", Options{AOT: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var panics []string
			opts := c.opts
			opts.Dir = ".."
			opts.Env = []string{}
			opts.Budget = 10_000_000
			opts.GoPanic = func(v any) {
				mu.Lock()
				defer mu.Unlock()
				panics = append(panics, fmt.Sprint(v))
			}
			p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/logvaluerpanicplug")
			if err != nil {
				t.Fatal(err)
			}
			res, err := p.Call(context.Background(), "Values")
			if err != nil {
				t.Fatal(err)
			}
			v, err := res[0].Interface()
			if err != nil {
				t.Fatal(err)
			}
			vs := v.([]any)
			for i, name := range []string{"bad", "spin"} {
				var buf bytes.Buffer
				l := slog.New(slog.NewTextHandler(&buf, nil))
				done := make(chan bool)
				go func() {
					defer close(done)
					l.Info("host", name, vs[i])
					done <- true
				}()
				if !<-done {
					t.Fatalf("%s: the host goroutine ended in slog", name)
				}
				if !strings.Contains(buf.String(), name+`="LogValue panicked`) {
					t.Errorf("%s: the host logged %q, want LogValue panicked", name, buf.String())
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if len(panics) != 2 || !strings.Contains(panics[0], "bad-value") || !strings.Contains(panics[1], "budget") {
				t.Errorf("GoPanic got %q, want the panic of Bad and the budget error of Spin", panics)
			}
		})
	}
}
