// Command run measures the Traefik plugin catalog (compare/catalog/README.md):
// for every repository cloned by fetch.sh whose .traefik.yml declares a yaegi
// middleware, it runs the scenario of vmhost/driver.go with a native build of
// the plugin (twice, the reference), yaegi v0.16.1 configured as Traefik,
// yaegi v2 and yaegi v2 -aot, then classifies each plugin and writes
// results/catalog.tsv and results/summary.txt. In the results, the engine
// key of yaegi v2 is "govm" (its name when they were measured) and "aot" is
// yaegi v2 -aot.
//
//	sh compare/catalog/fetch.sh
//	cd compare && go run ./catalog/run -j 8
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

var (
	jobs    = flag.Int("j", 4, "plugins measured in parallel")
	only    = flag.String("only", "", "regexp: measure only the matching repositories")
	timeout = flag.Duration("timeout", 300*time.Second, "timeout of each host process")
	rebuild = flag.Bool("rebuild", false, "rebuild the native hosts")
	reuse   = flag.Bool("reuse", false, "classify the outputs of a previous run (out/) without running the hosts")
	srcDir  = flag.String("src", "", "directory of the plugins and of their COMMITS.tsv (default: src/ of the catalog)")
	vmExec  = flag.String("vmexec", "", "os/exec of the plugins under yaegi v2: \"\" (refused, as Traefik) or \"mock\" (vm.ExecMock, the -exec flag of vmhost)")
)

// manifest is the part of .traefik.yml read here (pkg/plugins/types.go).
type manifest struct {
	Type      string         `yaml:"type"`
	Runtime   string         `yaml:"runtime"`
	Import    string         `yaml:"import"`
	BasePkg   string         `yaml:"basePkg"`
	UseUnsafe bool           `yaml:"useUnsafe"`
	TestData  map[string]any `yaml:"testData"`
}

type plugin struct {
	repo, version, dir, name string
	modDir                   string // module of the plugin package (nested modules: not dir)
	m                        manifest
	scope                    string // "" when measured, else why not

	res    map[string]outcome // native, native2, yaegi, govm, aot
	class  string             // yaegi vs govm
	classA string             // yaegi vs govm -aot
	note   string
}

// outcome is the result of one host.
type outcome struct {
	transcript string
	stage      string // ok, load, config, new, run (panic or timeout in a request), crash, build
	loadMS     string
	errLine    string
	engineRace bool // conc scenario: a data race in the engine (extra.go, markEngineRaces)
}

var root string // compare/catalog

func main() {
	flag.Parse()
	if err := checkScenarios(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	exe, _ := os.Getwd()
	root = findRoot(exe)
	ps := readPlugins()
	buildHosts()
	var re *regexp.Regexp
	if *only != "" {
		re = regexp.MustCompile(*only)
	}
	startScenarios()
	var wg sync.WaitGroup
	sem := make(chan struct{}, *jobs)
	for _, p := range ps {
		if (p.scope != "" && !wantsScenarios(p)) || (re != nil && !re.MatchString(p.repo)) {
			continue
		}
		wg.Add(1)
		go func(p *plugin) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// The extra scenarios (extra.go) run inside measure: they
			// need the native host and the outcomes of the middleware one.
			if p.scope == "" && (runs("middleware") || len(extraList()) > 0) {
				measure(p)
				fmt.Fprintf(os.Stderr, "%-55s native=%-6s yaegi=%-6s govm=%-6s aot=%-6s %s | %s\n", p.repo,
					p.res["native"].stage, verdict(p, "yaegi"), verdict(p, "govm"), verdict(p, "aot"), p.class, p.classA)
			}
			measureScenarios(p)
		}(p)
	}
	wg.Wait()
	if re == nil {
		if runs("middleware") {
			writeResults(ps)
		}
		writeScenarioResults(ps)
		writeExtraResults(ps)
		runSoakPhase(ps)
	}
}

func findRoot(dir string) string {
	for d := dir; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "compare/catalog/plugins.txt")); err == nil {
			return filepath.Join(d, "compare/catalog")
		}
	}
	fmt.Fprintln(os.Stderr, "run from the yaegi v2 repository")
	os.Exit(1)
	return ""
}

func readPlugins() []*plugin {
	src := filepath.Join(root, "src")
	if *srcDir != "" {
		src, _ = filepath.Abs(*srcDir)
	}
	f, err := os.Open(filepath.Join(src, "COMMITS.tsv"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "run fetch.sh first:", err)
		os.Exit(1)
	}
	defer f.Close()
	var ps []*plugin
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fl := strings.Split(sc.Text(), "\t")
		p := &plugin{repo: fl[0], version: fl[1], name: strings.ReplaceAll(fl[0], "/", "_"), res: map[string]outcome{}}
		p.dir = filepath.Join(src, p.name)
		ps = append(ps, p)
		if fl[1] == "CLONE-FAILED" {
			p.scope = "clone failed"
			continue
		}
		b, err := os.ReadFile(filepath.Join(p.dir, ".traefik.yml"))
		if err != nil {
			b, err = os.ReadFile(filepath.Join(p.dir, ".traefik.yaml"))
		}
		if err != nil {
			p.scope = "no manifest"
			continue
		}
		if err := yaml.Unmarshal(b, &p.m); err != nil {
			p.scope = "bad manifest"
			continue
		}
		p.modDir = moduleDir(p.dir, strings.Trim(p.m.Import, `"`))
		switch {
		case p.m.Type == "provider":
			p.scope = "provider"
		case p.m.Runtime == "wasm":
			p.scope = "wasm"
		case p.m.Type != "middleware":
			p.scope = "type " + p.m.Type
		}
	}
	return ps
}

// moduleDir returns the directory of the module that holds package imp of the
// repository cloned in dir: dir itself, or a nested module (a plugin in a
// subdirectory with its own go.mod).
func moduleDir(dir, imp string) string {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return dir
	}
	rel, ok := strings.CutPrefix(imp, modfile.ModulePath(b))
	if !ok {
		return dir
	}
	for d := filepath.Join(dir, filepath.FromSlash(rel)); strings.HasPrefix(d, dir) && d != dir; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	return dir
}

func buildHosts() {
	bin := filepath.Join(root, "bin")
	run := func(dir string, args ...string) {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "build:", args, err)
			os.Exit(1)
		}
	}
	run(filepath.Join(root, ".."), "build", "-o", filepath.Join(bin, "vmhost"), "./catalog/vmhost")
	run(filepath.Join(root, "yhost"), "build", "-o", filepath.Join(bin, "yhost"), ".")
	buildRaceHosts()
}

func measure(p *plugin) {
	out := filepath.Join(root, "out", p.name)
	os.MkdirAll(out, 0o755)
	td := filepath.Join(out, "testdata.json")
	ensureToolchain(p)
	b, err := json.Marshal(jsonable(p.m.TestData))
	if err != nil {
		p.note = "testData: " + err.Error()
		b = []byte("{}")
	}
	os.WriteFile(td, b, 0o644)
	args := []string{"-import", strings.Trim(p.m.Import, `"`), "-basepkg", p.m.BasePkg, "-testdata", td}
	if p.m.UseUnsafe {
		args = append(args, "-unsafe")
	}

	if *reuse {
		if _, err := os.Stat(filepath.Join(out, "nativehost")); err != nil {
			b, _ := os.ReadFile(filepath.Join(out, "native.build.txt"))
			p.res["native"] = outcome{stage: "build", errLine: firstLine(string(b))}
			p.res["native2"] = p.res["native"]
		} else {
			p.res["native"] = readOutcome(out, "native")
			p.res["native2"] = readOutcome(out, "native2")
		}
	} else if nat, err := buildNative(p, out); err != nil {
		p.res["native"] = outcome{stage: "build", errLine: firstLine(err.Error())}
		p.res["native2"] = p.res["native"]
	} else {
		p.res["native"] = host(p, out, "native", nat, append(args, "-dir", p.dir))
		p.res["native2"] = host(p, out, "native2", nat, append(args, "-dir", p.dir))
	}
	p.res["yaegi"] = host(p, out, "yaegi", filepath.Join(root, "bin/yhost"), append(args, "-dir", p.dir))
	vmArgs := append(slices.Clip(args), "-dir", p.modDir)
	if *vmExec != "" {
		vmArgs = append(vmArgs, "-exec", *vmExec)
	}
	p.res["govm"] = host(p, out, "govm", filepath.Join(root, "bin/vmhost"), vmArgs)
	p.res["aot"] = host(p, out, "aot", filepath.Join(root, "bin/vmhost"), append(slices.Clip(vmArgs), "-aot"))
	p.class = classify(p, "govm")
	p.classA = classify(p, "aot")
	measureExtra(p, out, args)
}

// jsonable converts the map[string]any of yaml.v3 into JSON-encodable values.
func jsonable(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, e := range x {
			m[k] = jsonable(e)
		}
		return m
	case map[any]any:
		m := map[string]any{}
		for k, e := range x {
			m[fmt.Sprint(k)] = jsonable(e)
		}
		return m
	case []any:
		for i := range x {
			x[i] = jsonable(x[i])
		}
	}
	return v
}

// buildNative copies the plugin repository and adds a main package that calls
// the plugin directly; dependencies come from vendor/, or from the module
// proxy when the plugin does not vendor them or its vendor/ is inconsistent.
func buildNative(p *plugin, out string) (string, error) {
	exe := filepath.Join(out, "nativehost")
	stamp := filepath.Join(out, "nativehost.stamp")
	if _, err := os.Stat(exe); err == nil && !*rebuild {
		// The cached host is reused only if it was built from the same
		// driver files.
		if b, _ := os.ReadFile(stamp); string(b) == driverStamp() {
			return exe, nil
		}
	}
	os.Remove(exe)
	defer func() {
		if _, err := os.Stat(exe); err == nil {
			os.WriteFile(stamp, []byte(driverStamp()), 0o644)
		}
	}()
	work := filepath.Join(root, "work", p.name)
	os.RemoveAll(work)
	if err := copyTree(p.dir, work); err != nil {
		return "", err
	}
	rel, _ := filepath.Rel(p.dir, p.modDir)
	work = filepath.Join(work, rel)
	gomod, err := os.ReadFile(filepath.Join(work, "go.mod"))
	if err != nil {
		return "", err
	}
	mf, err := modfile.Parse("go.mod", gomod, nil)
	if err != nil {
		return "", err
	}
	if mf.Go == nil || semverLess(mf.Go.Version, "1.21") {
		// Old go directives disable generics and other features
		// that yaegi and yaegi v2 accept anyway; the driver needs 1.21.
		mf.AddGoStmt("1.21")
	}
	mf.AddRequire(mapstructurePath, mapstructureVersion)
	nb, _ := mf.Format()
	os.WriteFile(filepath.Join(work, "go.mod"), nb, 0o644)
	hd := filepath.Join(work, "zz_cataloghost")
	os.MkdirAll(hd, 0o755)
	// driver.go, driver_scenarios.go and driver_extra.go.
	drvs, _ := filepath.Glob(filepath.Join(root, "vmhost/driver*.go"))
	for _, d := range drvs {
		drv, _ := os.ReadFile(d)
		os.WriteFile(filepath.Join(hd, filepath.Base(d)), drv, 0o644)
	}
	main := strings.ReplaceAll(nativeMain, "IMPORT", strings.Trim(p.m.Import, `"`))
	os.WriteFile(filepath.Join(hd, "main.go"), []byte(main), 0o644)
	prov := strings.ReplaceAll(nativeProvider, "IMPORT", strings.Trim(p.m.Import, `"`))
	os.WriteFile(filepath.Join(hd, "provider.go"), []byte(prov), 0o644)
	gocmd := func(env []string, args ...string) error {
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Env = work, append(os.Environ(), env...)
		o, err := cmd.CombinedOutput()
		if err != nil {
			os.WriteFile(filepath.Join(out, "native.build.txt"), o, 0o644)
			return fmt.Errorf("%s: %s", strings.Join(args, " "), o)
		}
		return nil
	}
	// With the vendored dependencies first (they may be patched copies, as
	// yaegi and yaegi v2 see them), mapstructure added to vendor/.
	vendor := filepath.Join(work, "vendor")
	if _, err := os.Stat(vendor); err == nil {
		mc, _ := exec.Command("go", "env", "GOMODCACHE").Output()
		src := filepath.Join(strings.TrimSpace(string(mc)), mapstructurePath+"@"+mapstructureVersion)
		if err := copyTree(src, filepath.Join(vendor, filepath.FromSlash(mapstructurePath))); err == nil {
			f, _ := os.OpenFile(filepath.Join(vendor, "modules.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
			fmt.Fprintf(f, "# %s %s\n## explicit; go 1.14\n%s\n", mapstructurePath, mapstructureVersion, mapstructurePath)
			f.Close()
			if gocmd([]string{"GOFLAGS=-mod=vendor", "GOPROXY=off", "GOWORK=off", "GOTOOLCHAIN=auto"}, "build", "-o", exe, "./zz_cataloghost") == nil {
				os.Remove(filepath.Join(out, "native.build.txt"))
				return exe, nil
			}
		}
		os.RemoveAll(vendor)
	}
	env := []string{"GOFLAGS=-mod=mod", "GOPROXY=https://proxy.golang.org", "GOSUMDB=sum.golang.org", "GOWORK=off", "GOTOOLCHAIN=auto"}
	if err := gocmd(env, "mod", "tidy", "-e"); err != nil {
		return "", err
	}
	if err := gocmd(env, "build", "-o", exe, "./zz_cataloghost"); err != nil {
		return "", err
	}
	os.Remove(filepath.Join(out, "native.build.txt"))
	return exe, nil
}

const (
	mapstructurePath    = "github.com/mitchellh/mapstructure"
	mapstructureVersion = "v1.5.1-0.20231216201459-8508981c8b6c" // Traefik's
)

// ensureToolchain downloads into the module cache the toolchain that the
// go.mod of p requires when it is newer than the local one, so that go list
// (yaegi v2's loader) works offline with GOTOOLCHAIN=auto.
func ensureToolchain(p *plugin) {
	if *reuse {
		return
	}
	b, err := os.ReadFile(filepath.Join(p.modDir, "go.mod"))
	if err != nil {
		return
	}
	mf, err := modfile.ParseLax("go.mod", b, nil)
	if err != nil {
		return
	}
	want := ""
	if mf.Go != nil {
		want = mf.Go.Version
	}
	if mf.Toolchain != nil {
		if v := strings.TrimPrefix(mf.Toolchain.Name, "go"); semverLess(want, v) {
			want = v
		}
	}
	local := strings.TrimPrefix(runtime.Version(), "go")
	if !semverLess(local, want) {
		return
	}
	toolchainMu.Lock()
	defer toolchainMu.Unlock()
	cmd := exec.Command("go", "version")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go"+want, "GOPROXY=https://proxy.golang.org", "GOSUMDB=sum.golang.org")
	if o, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "toolchain go%s: %v %s\n", want, err, o)
	}
}

var toolchainMu sync.Mutex

var (
	modCacheOnce sync.Once
	modCacheDir  string
)

// hostModCache is the module cache of the hosts: only the Go toolchains of
// the real cache (and its checksum database cache), so that go list sees the
// plugin and its vendor/ directory, nothing more, as yaegi in Traefik sees the
// plugin archive and nothing more.
func hostModCache() string {
	modCacheOnce.Do(func() {
		out, _ := exec.Command("go", "env", "GOMODCACHE").Output()
		real := strings.TrimSpace(string(out))
		modCacheDir = filepath.Join(root, "work", "modcache")
		os.RemoveAll(modCacheDir)
		link := func(rel string) {
			dst := filepath.Join(modCacheDir, rel)
			os.MkdirAll(filepath.Dir(dst), 0o755)
			os.Symlink(filepath.Join(real, rel), dst)
		}
		tcs, _ := filepath.Glob(filepath.Join(real, "golang.org", "toolchain@*"))
		for _, tc := range tcs {
			rel, _ := filepath.Rel(real, tc)
			link(rel)
		}
		link("cache/download/golang.org/toolchain")
		link("cache/download/sumdb")
	})
	return modCacheDir
}

// semverLess compares Go versions (1.26, 1.26.2).
func semverLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscan(pa[i], &x)
		}
		if i < len(pb) {
			fmt.Sscan(pb[i], &y)
		}
		if x != y {
			return x < y
		}
	}
	return false
}

const nativeMain = `package main

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	plug "IMPORT"
)

type nativeEngine struct{}

func (nativeEngine) load(string, string, string, bool) error { return nil }

func (nativeEngine) createConfig(testData map[string]any) (any, error) {
	c := plug.CreateConfig()
	if err := traefikDecode(c, testData); err != nil {
		return nil, err
	}
	return c, nil
}

func (nativeEngine) newHandler(ctx context.Context, next http.Handler, cfg any, name string) (http.Handler, error) {
	res := reflect.ValueOf(plug.New).Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(next), reflect.ValueOf(cfg), reflect.ValueOf(name)})
	if len(res) > 1 && res[1].Interface() != nil {
		err, ok := res[1].Interface().(error)
		if !ok {
			return nil, fmt.Errorf("invalid error type: %T", res[1].Interface())
		}
		return nil, err
	}
	h, ok := res[0].Interface().(http.Handler)
	if !ok {
		return nil, fmt.Errorf("invalid handler type: %T", res[0].Interface())
	}
	return h, nil
}

func main() { runMain(nativeEngine{}) }
`

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() && rel == ".git" {
			return filepath.SkipDir
		}
		t := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(t, b, 0o644)
	})
}

// host runs one host process in the plugin directory and records its exit
// status in <mode>.rc.
func host(p *plugin, out, mode, exe string, args []string) outcome {
	if *reuse {
		return readOutcome(out, mode)
	}
	tr := filepath.Join(out, mode+".txt")
	tm := filepath.Join(out, mode+".times")
	os.Remove(tr)
	os.Remove(tm)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, append(args, "-out", tr, "-times", tm)...)
	cmd.Dir = p.modDir
	// yaegi v2 loads with go list: vendored modules when the plugin vendors
	// them, otherwise only what is already in the module cache (no network),
	// like yaegi which only sees the GOPATH. Toolchains newer than the local
	// one come from the module cache (see ensureToolchain).
	gf := "GOFLAGS=-mod=mod"
	if st, err := os.Stat(filepath.Join(p.modDir, "vendor")); err == nil && st.IsDir() {
		gf = "GOFLAGS=-mod=vendor"
	}
	tmp := filepath.Join(out, mode+".tmp")
	os.MkdirAll(tmp, 0o755)
	defer os.RemoveAll(tmp)
	cmd.Env = append(os.Environ(), gf, "GOPROXY=off", "GOTOOLCHAIN=auto", "GOWORK=off", "TMPDIR="+tmp, "GOMODCACHE="+hostModCache())
	if engineOf(mode) == "govm" || engineOf(mode) == "aot" {
		// yaegi v2 loads without the toolchain, as in Traefik: no go command
		// in PATH, no GOROOT.
		var env []string
		for _, kv := range cmd.Env {
			if !strings.HasPrefix(kv, "PATH=") && !strings.HasPrefix(kv, "GOROOT=") {
				env = append(env, kv)
			}
		}
		cmd.Env = append(env, "PATH=/usr/bin:/bin")
	}
	cmd.Env = append(cmd.Env, noNetEnv(mode)...) // scenarios only (scenarios.go)
	ef, _ := os.Create(filepath.Join(out, mode+".err"))
	cmd.Stdout, cmd.Stderr = ef, ef
	err := cmd.Run()
	ef.Close()
	rc := "0"
	switch {
	case ctx.Err() != nil:
		rc = "timeout"
	case err != nil:
		rc = err.Error()
	}
	os.WriteFile(filepath.Join(out, mode+".rc"), []byte(rc+"\n"), 0o644)
	return readOutcome(out, mode)
}

// readOutcome reads the files written by host for mode.
func readOutcome(out, mode string) outcome {
	o := outcome{}
	b, _ := os.ReadFile(filepath.Join(out, mode+".txt"))
	o.transcript = string(b)
	if t, _ := os.ReadFile(filepath.Join(out, mode+".times")); len(t) > 0 {
		for _, l := range strings.Split(string(t), "\n") {
			if v, ok := strings.CutPrefix(l, "load_ms\t"); ok {
				o.loadMS = v
			}
		}
	}
	eb, _ := os.ReadFile(filepath.Join(out, mode+".err"))
	o.errLine = firstErr(string(eb))
	rcb, _ := os.ReadFile(filepath.Join(out, mode+".rc"))
	rc := strings.TrimSpace(string(rcb))
	switch {
	case rc == "timeout":
		o.stage = "crash"
		o.errLine = "host timeout"
	case o.transcript == "":
		o.stage = "crash"
		if o.errLine == "" {
			o.errLine = rc
		}
	case strings.HasPrefix(o.transcript, "load: error"):
		o.stage = "load"
	case strings.Contains(o.transcript, "config: error"):
		o.stage = "config"
	case strings.Contains(o.transcript, "new: error"), strings.Contains(o.transcript, "new: nil"):
		o.stage = "new"
	case strings.Contains(o.transcript, "\n  panic\n"), strings.Contains(o.transcript, "\n  timeout\n"):
		o.stage = "run"
	default:
		o.stage = "ok"
	}
	return o
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// firstErr is the first stderr line of the host that reports a failure.
func firstErr(s string) string {
	for _, l := range strings.Split(s, "\n") {
		for _, p := range []string{"load:", "config:", "new:", "panic:", "fatal error:", "panic("} {
			if strings.HasPrefix(l, p) {
				return firstLine(l)
			}
		}
	}
	return firstLine(s)
}

// matches reports whether mode prints what the native build prints; when the
// two native runs differ (time, randomness), only the coarse transcripts
// (stages, statuses, header names) are compared.
func matches(p *plugin, mode string) bool {
	n, n2, o := p.res["native"], p.res["native2"], p.res[mode]
	if n.stage == "build" || n.stage == "crash" {
		return false
	}
	if norm(n.transcript) == norm(n2.transcript) {
		return norm(o.transcript) == norm(n.transcript)
	}
	return coarse(o.transcript) == coarse(n.transcript)
}

var (
	httpDate = regexp.MustCompile(`[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} (GMT|UTC)`)
	netError = regexp.MustCompile(`dial tcp: lookup [^ ]+ on [0-9a-f.:\[\]]+: no such host|context deadline exceeded \(Client\.Timeout exceeded while awaiting headers\)`)
)

// norm replaces what changes between runs for reasons outside the engines:
// HTTP dates (Date, Expires...), and the wording of a failed connection to a
// host the measuring machine cannot reach (which resolver answered, or a
// client timeout first).
func norm(t string) string {
	return netError.ReplaceAllString(httpDate.ReplaceAllString(t, "<date>"), "<network error>")
}

var headerVal = regexp.MustCompile(`^(  (?:header )?[A-Za-z0-9-]+): .*$`)

func coarse(t string) string {
	var b strings.Builder
	for _, l := range strings.Split(t, "\n") {
		switch {
		case strings.HasPrefix(l, "  body "), strings.HasPrefix(l, "  next: "):
			continue
		case headerVal.MatchString(l):
			l = headerVal.ReplaceAllString(l, "$1")
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

// verdict of one engine: ok (as native), diff, or the failing stage.
func verdict(p *plugin, mode string) string {
	o := p.res[mode]
	if matches(p, mode) {
		return "ok"
	}
	if o.stage == "ok" {
		return "diff"
	}
	return o.stage
}

// classify compares yaegi with mode (govm or aot).
func classify(p *plugin, mode string) string {
	y, g := p.res["yaegi"], p.res[mode]
	vy, vg := verdict(p, "yaegi"), verdict(p, mode)
	switch {
	case vy == "ok" && vg == "ok":
		return "same" // equal, or both match a nondeterministic native run
	case y.stage != "crash" && g.stage != "crash" && norm(y.transcript) == norm(g.transcript):
		if vy == "load" {
			return "both fail"
		}
		return "same"
	case vy == "ok" && vg != "ok":
		return "yaegi v2 fails"
	case vg == "ok" && vy != "ok":
		return "yaegi fails"
	case vg == "load" || vg == "crash":
		if vy == "load" || vy == "crash" {
			return "both fail"
		}
		return "yaegi v2 fails"
	case vy == "load" || vy == "crash":
		return "yaegi fails"
	}
	return "different"
}

func writeResults(ps []*plugin) {
	rd := filepath.Join(root, "results")
	os.MkdirAll(rd, 0o755)
	var b strings.Builder
	b.WriteString("repo\tversion\tscope\tnative\tdepth\tyaegi\tgovm\taot\tclass\tclass_aot\tload_ms_yaegi\tload_ms_govm\tload_ms_aot\tgovm_error\taot_error\tyaegi_error\n")
	count := map[string]map[string]int{"class": {}, "class_aot": {}, "scope": {}}
	for _, p := range ps {
		if p.scope != "" {
			count["scope"][p.scope]++
			fmt.Fprintf(&b, "%s\t%s\t%s\n", p.repo, p.version, p.scope)
			continue
		}
		count["scope"]["measured"]++
		count["class"][p.class]++
		count["class_aot"][p.classA]++
		n := p.res["native"]
		depth := "requests"
		if n.stage != "ok" && n.stage != "run" {
			depth = "stops at " + n.stage
		}
		if norm(n.transcript) != norm(p.res["native2"].transcript) {
			depth += ", nondeterministic"
		}
		errOf := func(m string) string {
			if verdict(p, m) == "ok" {
				return ""
			}
			return strings.ReplaceAll(p.res[m].errLine, "\t", " ")
		}
		fmt.Fprintf(&b, "%s\t%s\t\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.repo, p.version, n.stage, depth,
			verdict(p, "yaegi"), verdict(p, "govm"), verdict(p, "aot"), p.class, p.classA,
			p.res["yaegi"].loadMS, p.res["govm"].loadMS, p.res["aot"].loadMS, errOf("govm"), errOf("aot"), errOf("yaegi"))
	}
	os.WriteFile(filepath.Join(rd, "catalog.tsv"), []byte(b.String()), 0o644)
	var s strings.Builder
	fmt.Fprintf(&s, "Traefik plugin catalog, %s\n", time.Now().Format("2006-01-02"))
	for _, k := range []string{"scope", "class", "class_aot"} {
		fmt.Fprintf(&s, "\n%s:\n", k)
		var keys []string
		for c := range count[k] {
			keys = append(keys, c)
		}
		sort.Strings(keys)
		for _, c := range keys {
			fmt.Fprintf(&s, "  %-16s %d\n", c, count[k][c])
		}
	}
	os.WriteFile(filepath.Join(rd, "summary.txt"), []byte(s.String()), 0o644)
	fmt.Print(s.String())
}
