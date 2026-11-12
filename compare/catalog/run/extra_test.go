package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// g2Race is a data race report of vmhost -race -aot in the conc scenario, on
// a plugin that decodes a json.RawMessage in each request (bug G2 of
// docs/CATALOG.md): the engine races in its own code. Stacks shortened.
const g2Race = `==================
WARNING: DATA RACE
Read at 0x00c001e8a030 by goroutine 34:
  reflect.Value.IsNil()
      /usr/local/go/src/reflect/value.go:1647 +0xa94
  github.com/traefik/yaegi/v2/vm.(*Machine).fromNativeDeepC()
      /src/yaegi/vm/native.go:2195 +0xab4
  github.com/traefik/yaegi/v2/vm.(*shadowRef).detach()
      /src/yaegi/vm/twin_value.go:2010 +0xbc
  main.serveRec.func1()
      /src/yaegi/compare/catalog/vmhost/driver_extra.go:474 +0xa0

Previous write at 0x00c001e8a030 by goroutine 33:
  github.com/traefik/yaegi/v2/aot/encoding/json.(*RawMessage).UnmarshalJSON()
      /src/yaegi/aot/encoding/json/stream.go:280 +0x94
  main.serveRec.func1()
      /src/yaegi/compare/catalog/vmhost/driver_extra.go:474 +0xa0
==================
`

// ownRace is a data race report of vmhost -race on a plugin that writes a
// counter without a lock: the race is the plugin's, but its frames are the
// ones of the yaegi v2 interpreter that runs the plugin code.
const ownRace = `==================
WARNING: DATA RACE
Read at 0x00c001c23698 by goroutine 32:
  github.com/traefik/yaegi/v2/vm.(*ctx).exec()
      /src/yaegi/vm/exec.go:1365 +0xb7c0
  main.serveRec.func1()
      /src/yaegi/compare/catalog/vmhost/driver_extra.go:474 +0xa0

Previous write at 0x00c001c23698 by goroutine 31:
  github.com/traefik/yaegi/v2/vm.(*ctx).exec()
      /src/yaegi/vm/exec.go:1378 +0xbc2c
  main.serveRec.func1()
      /src/yaegi/compare/catalog/vmhost/driver_extra.go:474 +0xa0
==================
`

// nativeRace is the same race in the native host (the plugin is compiled).
const nativeRace = `==================
WARNING: DATA RACE
Read at 0x00c000123456 by goroutine 12:
  example.com/racyplug.(*plug).ServeHTTP()
      /home/user/go/src/github.com/traefik/yaegi/v2/work/racyplug.go:25 +0x44
==================
`

// concOutcomes writes the stderr of each conc host into a new out directory
// and returns that directory and outcomes whose transcripts are all equal.
func concOutcomes(t *testing.T, errs map[string]string) (string, map[string]outcome) {
	t.Helper()
	out := t.TempDir()
	r := map[string]outcome{}
	for _, m := range extraModes {
		if err := os.WriteFile(filepath.Join(out, extraMode("conc", m)+".err"), []byte(errs[m]), 0o644); err != nil {
			t.Fatal(err)
		}
		r[m] = outcome{stage: "ok", transcript: "load: ok\nconfig: ok\nnew: ok\nreq 1 GET /\n  status 200\n"}
	}
	return out, r
}

func TestEngineRaceVerdict(t *testing.T) {
	// The -aot host races in yaegi v2, native does not race: the output is
	// the same as native, but the verdict is "race" and the class a
	// yaegi v2 failure.
	out, r := concOutcomes(t, map[string]string{"aot": g2Race})
	markEngineRaces(out, r)
	if v := extraVerdict("conc", r, "aot"); v != "race" {
		t.Errorf("aot verdict %q, want race", v)
	}
	if c := extraClassify("conc", r, "aot"); c != "yaegi v2 fails" {
		t.Errorf("aot class %q, want yaegi v2 fails", c)
	}
	if v := extraVerdict("conc", r, "govm"); v != "ok" {
		t.Errorf("govm verdict %q, want ok", v)
	}

	// A race in yaegi is a yaegi failure.
	out, r = concOutcomes(t, map[string]string{"yaegi": "==================\nWARNING: DATA RACE\nWrite at 0x1 by goroutine 7:\n  github.com/traefik/yaegi/interp.genValue.func1()\n      /go/pkg/mod/github.com/traefik/yaegi@v0.16.1/interp/value.go:1 +0x1\n==================\n"})
	markEngineRaces(out, r)
	if v := extraVerdict("conc", r, "yaegi"); v != "race" {
		t.Errorf("yaegi verdict %q, want race", v)
	}
	if c := extraClassify("conc", r, "govm"); c != "yaegi fails" {
		t.Errorf("govm class %q, want yaegi fails", c)
	}
}

func TestPluginRaceIsNotEngineRace(t *testing.T) {
	// The plugin races by itself: native races too, the reports of yaegi v2
	// (with frames of the interpreter) do not change the verdict.
	out, r := concOutcomes(t, map[string]string{"native": nativeRace, "native2": nativeRace, "govm": ownRace, "aot": ownRace})
	markEngineRaces(out, r)
	for _, m := range []string{"govm", "aot"} {
		if v := extraVerdict("conc", r, m); v != "ok" {
			t.Errorf("%s verdict %q, want ok", m, v)
		}
	}

	// Without native runs nothing tells the two kinds apart: no verdict.
	out, r = concOutcomes(t, map[string]string{"aot": g2Race})
	r["native"], r["native2"] = outcome{stage: "build"}, outcome{stage: "build"}
	markEngineRaces(out, r)
	if r["aot"].engineRace {
		t.Error("engine race marked without a native run")
	}
}

func TestHasEngineRace(t *testing.T) {
	if !hasEngineRace(g2Race, "github.com/traefik/yaegi/v2/") {
		t.Error("G2 report not seen as a yaegi v2 race")
	}
	if hasEngineRace(g2Race, "github.com/traefik/yaegi/") {
		t.Error("G2 report seen as a yaegi race")
	}
	// The file lines hold a path with github.com/traefik/yaegi/v2/ (checkout
	// under GOPATH): they are not frames of yaegi v2.
	if hasEngineRace(nativeRace, "github.com/traefik/yaegi/v2/") {
		t.Error("file path seen as a yaegi v2 frame")
	}
	if hasEngineRace("panic: boom\n  github.com/traefik/yaegi/v2/vm.(*ctx).exec()\n", "github.com/traefik/yaegi/v2/") {
		t.Error("frame outside a race report counted")
	}
}

func TestExtraScenariosUseNoNetProxy(t *testing.T) {
	defer func(p, s string, d time.Duration) { noNetProxy, *scenariosFlag, *soakDur = p, s, d }(noNetProxy, *scenariosFlag, *soakDur)
	noNetProxy = "http://127.0.0.1:1"
	// Every host of the extra scenarios and of the soak gets the proxy,
	// the hosts of the middleware scenario do not.
	for _, sc := range append(extraScenarios, "soak") {
		for _, m := range extraModes {
			if env := noNetEnv(extraMode(sc, m)); len(env) == 0 {
				t.Errorf("no proxy for %s", extraMode(sc, m))
			}
		}
	}
	if env := noNetEnv("govm"); env != nil {
		t.Errorf("proxy for the middleware scenario: %v", env)
	}
	// The proxy is started when only extra scenarios or only the soak run.
	*soakDur = 0
	for _, s := range []string{"headers", "middleware,stream", "conc"} {
		*scenariosFlag = s
		if !needsNoNet() {
			t.Errorf("-scenarios %s: no proxy started", s)
		}
	}
	*scenariosFlag = "middleware"
	if needsNoNet() {
		t.Error("-scenarios middleware: proxy started")
	}
	*soakDur = time.Minute
	if !needsNoNet() {
		t.Error("-soak: no proxy started")
	}
}

func TestOneScenariosFlag(t *testing.T) {
	defer func(s string) { *scenariosFlag = s }(*scenariosFlag)
	// The default runs every scenario of both lists.
	*scenariosFlag = flag.Lookup("scenarios").DefValue
	for _, s := range []string{"middleware", "auth", "reload", "reloadcancel", "reloadsandbox", "provider", "headers", "stream", "conc"} {
		if !runs(s) {
			t.Errorf("default -scenarios does not run %s", s)
		}
	}
	if err := checkScenarios(); err != nil {
		t.Error(err)
	}
	*scenariosFlag = "middleware, conc"
	if l := extraList(); len(l) != 1 || l[0] != "conc" {
		t.Errorf("extraList %v, want [conc]", l)
	}
	*scenariosFlag = "middleware,soak"
	if err := checkScenarios(); err == nil {
		t.Error("unknown scenario soak accepted")
	}
}
