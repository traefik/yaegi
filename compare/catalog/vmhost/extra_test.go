package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const streamPlug = "github.com/traefik/yaegi/v2/compare/catalog/vmhost/testdata/streamplug"

// loadStreamPlug builds the handler of testdata/streamplug with yaegi v2, as
// runMain does.
func loadStreamPlug(t *testing.T, next http.Handler) http.Handler {
	t.Helper()
	e := &vmEngine{}
	if err := e.load("../..", streamPlug, "streamplug", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := e.createConfig(map[string]any{"value": "rewritten"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := e.newHandler(context.Background(), next, cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHeadersScenario(t *testing.T) {
	s := &seen{}
	h := loadStreamPlug(t, echoNext{s})
	s.start(1)
	res, ok := serve(h, headerScenario[0], 1, 10*time.Second)
	if !ok {
		t.Fatal("timeout")
	}
	// The in-place write of the plugin must reach next, and the response
	// headers set by next must keep all their values.
	for _, want := range []string{`X-Multi: [\"rewritten\" \"two\" \"three\"]`, `header Set-Cookie: ["up1=a; Path=/" "up2=b; Path=/; HttpOnly"]`, `header X-Plugin: ["3"]`} {
		if !strings.Contains(res, want) {
			t.Errorf("missing %s in\n%s", want, res)
		}
	}
	if !strings.Contains(s.String(), `X-Multi: ["rewritten" "two" "three"]`) {
		t.Errorf("next saw\n%s", s.String())
	}
}

func TestStreamScenario(t *testing.T) {
	h := loadStreamPlug(t, streamNext{})
	var tr, tm strings.Builder
	runStream(h, &tr, &tm, 20*time.Second)
	// The plugin passes the writer of the host to next: flush and hijack
	// must survive it.
	for _, want := range []string{"next called flusher=true", "next early-first-chunk=yes", "next called hijacker=true", `echo "echo ping\n"`} {
		if !strings.Contains(tr.String(), want) {
			t.Errorf("missing %s in\n%s", want, tr.String())
		}
	}
}

// TestRequestContextDone checks that the hosts end the context of each request
// when the handler returns, as net/http does. darkweak/souin starts one
// goroutine per request that waits for it: with contexts that never end, the
// conc scenario kept one goroutine per request and passed MaxGoroutines,
// natively too.
func TestRequestContextDone(t *testing.T) {
	for _, tc := range []struct {
		name string
		send func(h http.Handler)
	}{
		{"serve", func(h http.Handler) { serve(h, scenario[0], 1, 10*time.Second) }},
		{"serveAuth", func(h http.Handler) { serveAuth(h, scenario[0], 1, 10*time.Second) }},
		{"serveRec", func(h http.Handler) { serveRec(h, scenario[0], 10*time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctxs := make(chan context.Context, 1)
			tc.send(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctxs <- r.Context()
			}))
			select {
			case <-(<-ctxs).Done():
			case <-time.After(5 * time.Second):
				t.Fatal("the context of the request is not done after the handler returned")
			}
		})
	}
}

func TestConcScenario(t *testing.T) {
	*concShape = "8x20"
	h := loadStreamPlug(t, ctxNext{})
	var tr, tm strings.Builder
	runConc(h, &tr, &tm, 20*time.Second)
	out := tr.String()
	if strings.Contains(out, "  panic") || strings.Contains(out, "  timeout") {
		t.Fatalf("panic or timeout:\n%s", out)
	}
	// The plugin is deterministic: one distinct response per request.
	if n := strings.Count(out, "\nreq "); n != len(scenario) {
		t.Errorf("%d distinct responses, want %d:\n%s", n, len(scenario), out)
	}
}

func TestSoak(t *testing.T) {
	dir := t.TempDir()
	td := filepath.Join(dir, "td.json")
	os.WriteFile(td, []byte(`{"value":"soak"}`), 0o644)
	list := filepath.Join(dir, "list.tsv")
	line := "a\t../../..\t" + streamPlug + "\tstreamplug\t" + td + "\tfalse\n"
	os.WriteFile(list, []byte(line+strings.Replace(line, "a\t", "b\t", 1)), 0o644)
	newEngine = func() engine { return &vmEngine{} }
	*soakList, *soakDur, *soakSample, *soakWorkers = list, 2*time.Second, time.Second, 4
	defer func() { *soakList = "" }()
	out := filepath.Join(dir, "out.txt")
	samples := filepath.Join(dir, "samples.tsv")
	// runSoak reads -out and -times from the flag set of runMain.
	defineFlag(t, "out", out)
	defineFlag(t, "times", samples)
	if !soakRequested() {
		t.Fatal("soak not run")
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "2 plugins running") || strings.Contains(string(b), "error") ||
		strings.Count(string(b), "panics 0 timeouts 0 status 200:") != 2 {
		t.Errorf("soak output:\n%s", b)
	}
	s, _ := os.ReadFile(samples)
	for _, phase := range []string{"\nloaded\t", "\ntraffic\t", "\nend\t", "\nafter-cancel\t"} {
		if !strings.Contains(string(s), phase) {
			t.Errorf("no %q sample in\n%s", phase, s)
		}
	}
}

func defineFlag(t *testing.T, name, value string) {
	t.Helper()
	if f := flag.CommandLine.Lookup(name); f != nil {
		f.Value.Set(value)
		return
	}
	flag.CommandLine.String(name, value, "")
}

func TestOneScenarioFlag(t *testing.T) {
	// One -scenario flag for every host scenario: the request loop of
	// runMain takes middleware and the extra ones, runScenario the others.
	if f := flag.Lookup("scenario"); f == nil || f.DefValue != "middleware" {
		t.Fatalf("-scenario flag: %v", f)
	}
	for _, s := range []string{"middleware", "headers", "stream", "conc"} {
		ex, loop := loopScenario(s)
		if !loop || (s == "middleware") != (ex == nil) {
			t.Errorf("%s: loop %v, extra %v", s, loop, ex)
		}
	}
	for _, s := range []string{"auth", "reload", "provider", "unknown"} {
		if _, loop := loopScenario(s); loop {
			t.Errorf("%s runs the request loop", s)
		}
	}
}
