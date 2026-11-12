package main

// The extra scenarios of the harness (vmhost/driver_extra.go, README.md
// "Extra scenarios"): headers, stream and conc run per plugin after the
// middleware scenario (they are values of -scenarios, scenarios.go); the soak
// runs at the end with many plugins in one process.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	concShape  = flag.String("conc", "32x50", "conc scenario: goroutines x requests per goroutine")
	soakDur    = flag.Duration("soak", 0, "duration of the soak (several plugins in one process); 0 skips it")
	soakOnly   = flag.String("soakonly", "", "regexp: soak only the matching repositories")
	soakSample = flag.Duration("soaksample", 0, "soak sampling period (default soak/20)")
)

// extraModes are the hosts of each extra scenario, as in the base one.
var extraModes = []string{"native", "native2", "yaegi", "govm", "aot"}

var (
	extraMu  sync.Mutex
	extraRes = map[*plugin]map[string]map[string]outcome{} // plugin, scenario, mode
)

// extraScenarios are the values of -scenarios handled in this file.
var extraScenarios = []string{"headers", "stream", "conc"}

// extraList is the extra scenarios selected with -scenarios, in run order.
func extraList() []string {
	var l []string
	for _, s := range extraScenarios {
		if runs(s) {
			l = append(l, s)
		}
	}
	return l
}

// buildRaceHosts builds the yaegi v2 and yaegi hosts with -race, for conc.
func buildRaceHosts() {
	if *reuse || !hasExtra("conc") {
		return
	}
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
	run(filepath.Join(root, ".."), "build", "-race", "-o", filepath.Join(bin, "vmhost-race"), "./catalog/vmhost")
	run(filepath.Join(root, "yhost"), "build", "-race", "-o", filepath.Join(bin, "yhost-race"), ".")
}

func hasExtra(name string) bool {
	for _, s := range extraList() {
		if s == name {
			return true
		}
	}
	return false
}

// buildNativeRace builds the native host of p with -race, from the work
// directory left by buildNative (with vendor/ when the vendored build worked).
func buildNativeRace(p *plugin, out string) (string, error) {
	exe := filepath.Join(out, "nativehost-race")
	if _, err := os.Stat(exe); err == nil && !*rebuild {
		return exe, nil
	}
	work := filepath.Join(root, "work", p.name)
	rel, _ := filepath.Rel(p.dir, p.modDir)
	work = filepath.Join(work, rel)
	env := []string{"GOFLAGS=-mod=mod", "GOPROXY=https://proxy.golang.org", "GOSUMDB=sum.golang.org", "GOWORK=off", "GOTOOLCHAIN=auto"}
	if _, err := os.Stat(filepath.Join(work, "vendor")); err == nil {
		env = []string{"GOFLAGS=-mod=vendor", "GOPROXY=off", "GOWORK=off", "GOTOOLCHAIN=auto"}
	}
	cmd := exec.Command("go", "build", "-race", "-o", exe, "./zz_cataloghost")
	cmd.Dir, cmd.Env = work, append(os.Environ(), env...)
	if o, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build -race: %s", o)
	}
	return exe, nil
}

// extraMode is the host mode of engine m in scenario sc, named as the modes
// of the other scenarios ("conc-govm"): engineOf finds the engine and noNetEnv
// gives the host the no-network proxy. Its files are <out>/<mode>.txt, .err...
func extraMode(sc, m string) string { return sc + "-" + m }

// measureExtra runs the extra scenarios of p after the base one.
func measureExtra(p *plugin, out string, args []string) {
	nat := filepath.Join(out, "nativehost")
	res := map[string]map[string]outcome{}
	for _, sc := range extraList() {
		r := map[string]outcome{}
		a := append(append([]string{}, args...), "-scenario", sc)
		natExe, yExe, vmExe := nat, filepath.Join(root, "bin/yhost"), filepath.Join(root, "bin/vmhost")
		var natErr error
		if sc == "conc" {
			a = append(a, "-conc", *concShape)
			yExe, vmExe = filepath.Join(root, "bin/yhost-race"), filepath.Join(root, "bin/vmhost-race")
			if !*reuse && p.res["native"].stage != "build" {
				natExe, natErr = buildNativeRace(p, out)
			}
		}
		if p.res["native"].stage == "build" || natErr != nil {
			line := p.res["native"].errLine
			if natErr != nil {
				line = firstLine(natErr.Error())
			}
			r["native"] = outcome{stage: "build", errLine: line}
			r["native2"] = r["native"]
		} else {
			r["native"] = host(p, out, extraMode(sc, "native"), natExe, append(a, "-dir", p.dir))
			r["native2"] = host(p, out, extraMode(sc, "native2"), natExe, append(a, "-dir", p.dir))
		}
		r["yaegi"] = host(p, out, extraMode(sc, "yaegi"), yExe, append(a, "-dir", p.dir))
		r["govm"] = host(p, out, extraMode(sc, "govm"), vmExe, append(a, "-dir", p.modDir))
		r["aot"] = host(p, out, extraMode(sc, "aot"), vmExe, append(a, "-dir", p.modDir, "-aot"))
		if sc == "conc" {
			markEngineRaces(out, r)
		}
		res[sc] = r
	}
	extraMu.Lock()
	extraRes[p] = res
	extraMu.Unlock()
}

var concCount = regexp.MustCompile(`(?m)^(req \d+ .*) x \d+$`)

// canon is the transcript of a scenario as compared: for conc, the set of
// distinct responses of each request, without their counts (the order of
// concurrent requests changes the counts of stateful plugins).
func canon(sc, t string) string {
	if sc == "conc" {
		return concCount.ReplaceAllString(t, "$1")
	}
	return t
}

func extraMatches(sc string, r map[string]outcome, mode string) bool {
	n, n2, o := r["native"], r["native2"], r[mode]
	if n.stage == "build" || n.stage == "crash" {
		return false
	}
	c := func(t string) string { return norm(canon(sc, t)) }
	if c(n.transcript) == c(n2.transcript) {
		return c(o.transcript) == c(n.transcript)
	}
	return coarseSet(sc, c(o.transcript)) == coarseSet(sc, c(n.transcript))
}

// coarseSet is coarse for a scenario transcript. For conc it is the set of
// the coarse responses: with random values in the responses (session ids),
// each request has its own distinct response, and only the set of their
// coarse forms can be compared.
func coarseSet(sc, t string) string {
	if sc != "conc" {
		return coarse(t)
	}
	seen := map[string]bool{}
	var entries []string
	var cur strings.Builder
	flush := func() {
		if e := coarse(cur.String()); cur.Len() > 0 && !seen[e] {
			seen[e] = true
			entries = append(entries, e)
		}
		cur.Reset()
	}
	for _, l := range strings.SplitAfter(t, "\n") {
		if strings.HasPrefix(l, "req ") {
			flush()
		}
		cur.WriteString(l)
	}
	flush()
	sort.Strings(entries)
	return strings.Join(entries, "")
}

// extraVerdict compares the host of mode with native. "race" (conc only) is
// a data race in the engine (markEngineRaces): it replaces "ok" and "diff",
// since the output of a racing engine is right only by chance.
func extraVerdict(sc string, r map[string]outcome, mode string) string {
	if r[mode].engineRace && r[mode].stage == "ok" {
		return "race"
	}
	if extraMatches(sc, r, mode) {
		return "ok"
	}
	if r[mode].stage == "ok" {
		return "diff"
	}
	return r[mode].stage
}

// enginePkgs are the package paths of the engines, by host mode.
var enginePkgs = map[string]string{
	"yaegi": "github.com/traefik/yaegi/",
	"govm":  "github.com/traefik/yaegi/v2/",
	"aot":   "github.com/traefik/yaegi/v2/",
}

// markEngineRaces sets engineRace on the conc outcomes of the engines whose
// stderr (<out>/conc-<mode>.err) has a data race report with a frame of the
// engine, when neither native run (same plugin, same requests, built with
// -race too) reported a race.
//
// A frame of the engine alone does not tell an engine race from a race of
// the plugin's own code: the engine runs that code, so the plugin's own
// races also have engine frames (vm.(*ctx).exec for yaegi v2). The native runs
// tell them apart: a plugin that races by itself races natively too. When
// native races, the reports of the engines stay counted (races columns of
// results/scenarios.tsv) but do not change the verdict. When native did not
// run (build failure, crash), nothing is marked.
func markEngineRaces(out string, r map[string]outcome) {
	for _, n := range []string{"native", "native2"} {
		if st := r[n].stage; st == "build" || st == "crash" {
			return
		}
		if races(out, extraMode("conc", n)) > 0 {
			return
		}
	}
	for mode, pkg := range enginePkgs {
		o, ok := r[mode]
		if !ok {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(out, extraMode("conc", mode)+".err"))
		o.engineRace = hasEngineRace(string(b), pkg)
		r[mode] = o
	}
}

// hasEngineRace tells whether stderr has a data race report with a function
// frame in the package path pkg. Only the function lines of the stacks are
// read: the file lines hold the local path of the sources, which can contain
// the same path (a checkout under GOPATH).
func hasEngineRace(stderr, pkg string) bool {
	in := false
	for _, l := range strings.Split(stderr, "\n") {
		switch {
		case strings.HasPrefix(l, "WARNING: DATA RACE"):
			in = true
		case strings.HasPrefix(l, "=================="):
			in = false
		case in && !strings.HasPrefix(l, "      ") && framePkg(strings.TrimSpace(l)) == pkg:
			return true
		}
	}
	return false
}

// framePkg returns the engine package path of a function frame: the longest
// path of enginePkgs that prefixes it, or "". The longest match matters
// because the v2 path (github.com/traefik/yaegi/v2/) starts with the v0 path
// (github.com/traefik/yaegi/).
func framePkg(frame string) string {
	best := ""
	for _, pkg := range enginePkgs {
		if strings.HasPrefix(frame, pkg) && len(pkg) > len(best) {
			best = pkg
		}
	}
	return best
}

// extraClassify compares yaegi with mode, as classify does for the base
// scenario.
func extraClassify(sc string, r map[string]outcome, mode string) string {
	y, g := r["yaegi"], r[mode]
	vy, vg := extraVerdict(sc, r, "yaegi"), extraVerdict(sc, r, mode)
	switch {
	case vy == "ok" && vg == "ok":
		return "same"
	case vy == "race" && vg == "race":
		return "both fail"
	case vg == "race":
		return "yaegi v2 fails"
	case vy == "race":
		return "yaegi fails"
	case y.stage != "crash" && g.stage != "crash" && norm(canon(sc, y.transcript)) == norm(canon(sc, g.transcript)):
		if vy == "load" {
			return "both fail"
		}
		return "same"
	case vy == "ok":
		return "yaegi v2 fails"
	case vg == "ok":
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

// applies tells whether the scenario tests something for this plugin, from
// the native transcript: the scenario still runs and is compared, but a
// plugin that never calls next, say, does not test flush or header echo.
func applies(sc string, r map[string]outcome) string {
	n := r["native"]
	if n.stage == "build" || n.stage == "crash" || n.stage == "load" || n.stage == "config" || n.stage == "new" {
		return "n/a: native stops at " + n.stage
	}
	switch sc {
	case "headers", "conc":
		if !regexp.MustCompile(`(?m)^  next: [A-Z]`).MatchString(n.transcript) {
			return "n/a: native never reaches next"
		}
	case "stream":
		sse, ws := streamStep(n.transcript, "sse"), streamStep(n.transcript, "ws")
		var why []string
		if !strings.Contains(sse, "  next called") {
			why = append(why, "sse")
		}
		if !strings.Contains(ws, "  next called") {
			why = append(why, "ws")
		}
		if len(why) > 0 {
			return "n/a: native does not reach next for " + strings.Join(why, " and ")
		}
	}
	return "yes"
}

// streamStep is the part of a stream transcript for one step (sse, ws, plain).
func streamStep(t, step string) string {
	var b strings.Builder
	in := false
	for _, l := range strings.SplitAfter(t, "\n") {
		switch {
		case strings.HasPrefix(l, step+" "):
			in = true
		case !strings.HasPrefix(l, "  "):
			in = false
		}
		if in {
			b.WriteString(l)
		}
	}
	return b.String()
}

// streamFacts is what survives the plugin in the stream scenario for one
// engine: flush (the client saw the first SSE chunk before the end) and
// hijack (the upgrade answered 101 and echoed), "-" when next was not called.
func streamFacts(t string) (flush, hijack string) {
	sse, ws := streamStep(t, "sse"), streamStep(t, "ws")
	switch {
	case !strings.Contains(sse, "  next called"):
		flush = "-"
	case strings.Contains(sse, "early-first-chunk=yes"):
		flush = "yes"
	default:
		flush = "no"
	}
	switch {
	case !strings.Contains(ws, "  next called"):
		hijack = "-"
	case strings.Contains(ws, "  status 101") && strings.Contains(ws, `echo "echo ping\n"`):
		hijack = "yes"
	default:
		hijack = "no"
	}
	return flush, hijack
}

// races counts the data race reports in the stderr of a conc host.
func races(out, name string) int {
	b, _ := os.ReadFile(filepath.Join(out, name+".err"))
	return strings.Count(string(b), "WARNING: DATA RACE")
}

func writeExtraResults(ps []*plugin) {
	if len(extraList()) == 0 {
		return
	}
	rd := filepath.Join(root, "results")
	var b strings.Builder
	b.WriteString("repo\tversion\tscenario\tapplies\tnative\tyaegi\tgovm\taot\tclass\tclass_aot\tflush_native\tflush_yaegi\tflush_govm\tflush_aot\thijack_native\thijack_yaegi\thijack_govm\thijack_aot\traces_native\traces_yaegi\traces_govm\traces_aot\tgovm_error\taot_error\tyaegi_error\n")
	type key struct{ sc, what, v string }
	count := map[key]int{}
	for _, p := range ps {
		res := extraRes[p]
		if p.scope != "" || res == nil {
			continue
		}
		out := filepath.Join(root, "out", p.name)
		for _, sc := range extraList() {
			r := res[sc]
			if r == nil {
				continue
			}
			app := applies(sc, r)
			cl, clA := extraClassify(sc, r, "govm"), extraClassify(sc, r, "aot")
			count[key{sc, "class", cl}]++
			count[key{sc, "class_aot", clA}]++
			count[key{sc, "applies", strings.SplitN(app, ":", 2)[0]}]++
			for _, m := range []string{"yaegi", "govm", "aot"} {
				count[key{sc, "verdict_" + m, extraVerdict(sc, r, m)}]++
			}
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s", p.repo, p.version, sc, app, r["native"].stage,
				extraVerdict(sc, r, "yaegi"), extraVerdict(sc, r, "govm"), extraVerdict(sc, r, "aot"), cl, clA)
			for _, what := range []string{"flush", "hijack"} {
				for _, m := range []string{"native", "yaegi", "govm", "aot"} {
					v := ""
					if sc == "stream" && r[m].stage != "build" {
						f, h := streamFacts(r[m].transcript)
						v = map[string]string{"flush": f, "hijack": h}[what]
						count[key{sc, what + "_" + m, v}]++
					}
					fmt.Fprintf(&b, "\t%s", v)
				}
			}
			for _, m := range []string{"native", "yaegi", "govm", "aot"} {
				v := ""
				if sc == "conc" && r[m].stage != "build" {
					n := races(out, extraMode(sc, m))
					v = strconv.Itoa(n)
					if n > 0 {
						count[key{sc, "races_" + m, "plugins with races"}]++
					}
				}
				fmt.Fprintf(&b, "\t%s", v)
			}
			for _, m := range []string{"govm", "aot", "yaegi"} {
				e := ""
				if extraVerdict(sc, r, m) != "ok" {
					e = strings.ReplaceAll(r[m].errLine, "\t", " ")
				}
				fmt.Fprintf(&b, "\t%s", e)
			}
			b.WriteString("\n")
		}
	}
	os.WriteFile(filepath.Join(rd, "scenarios.tsv"), []byte(b.String()), 0o644)
	var s strings.Builder
	fmt.Fprintf(&s, "Traefik plugin catalog, extra scenarios, %s (conc %s)\n", time.Now().Format("2006-01-02"), *concShape)
	var keys []key
	for k := range count {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.sc != b.sc {
			return a.sc < b.sc
		}
		if a.what != b.what {
			return a.what < b.what
		}
		return a.v < b.v
	})
	last := ""
	for _, k := range keys {
		if h := k.sc + " " + k.what; h != last {
			fmt.Fprintf(&s, "\n%s:\n", h)
			last = h
		}
		fmt.Fprintf(&s, "  %-20s %d\n", k.v, count[k])
	}
	os.WriteFile(filepath.Join(rd, "scenarios-summary.txt"), []byte(s.String()), 0o644)
	fmt.Print(s.String())
}

// runSoakPhase loads every plugin that passes the base scenario in all the
// engines into one process per engine (yaegi, govm, aot), sends traffic for
// -soak, and writes results/soak-<engine>.tsv (samples) and results/soak.txt.
// It runs under -reuse too: "-reuse -soak 10m" runs only the soak.
// There is no native soak: one native binary with all the plugins would need
// one module with all their dependencies.
func runSoakPhase(ps []*plugin) {
	if *soakDur <= 0 {
		return
	}
	var re *regexp.Regexp
	if *soakOnly != "" {
		re = regexp.MustCompile(*soakOnly)
	}
	sd := filepath.Join(root, "out", "soak")
	os.MkdirAll(sd, 0o755)
	var lists = map[string]*strings.Builder{"yaegi": {}, "govm": {}, "aot": {}}
	var names, excluded []string
	for _, p := range ps {
		if p.scope != "" || p.res["native"].stage != "ok" || (re != nil && !re.MatchString(p.repo)) {
			continue
		}
		if verdict(p, "yaegi") != "ok" || verdict(p, "govm") != "ok" || verdict(p, "aot") != "ok" {
			continue
		}
		// A fatal error (concurrent map writes) cannot be recovered and
		// would end the soak process of every plugin.
		if f := fatalInConc(p); f != "" {
			excluded = append(excluded, fmt.Sprintf("%s: fatal error under concurrency in the conc scenario (%s)", p.repo, f))
			continue
		}
		names = append(names, p.repo)
		td := filepath.Join(root, "out", p.name, "testdata.json")
		for m, l := range lists {
			dir := p.modDir
			if m == "yaegi" {
				dir = p.dir
			}
			fmt.Fprintf(l, "%s\t%s\t%s\t%s\t%s\t%v\n", p.repo, dir, strings.Trim(p.m.Import, `"`), p.m.BasePkg, td, p.m.UseUnsafe)
		}
	}
	var wg sync.WaitGroup
	for m, l := range lists {
		list := filepath.Join(sd, m+".list.tsv")
		os.WriteFile(list, []byte(l.String()), 0o644)
		wg.Add(1)
		go func(m, list string) {
			defer wg.Done()
			exe := filepath.Join(root, "bin/vmhost")
			if m == "yaegi" {
				exe = filepath.Join(root, "bin/yhost")
			}
			args := []string{"-soak", list, "-soakdur", soakDur.String(), "-out", filepath.Join(sd, m+".txt"), "-times", filepath.Join(sd, m+".samples.tsv")}
			if *soakSample > 0 {
				args = append(args, "-soaksample", soakSample.String())
			}
			if m == "aot" {
				args = append(args, "-aot")
			}
			ctx, cancel := context.WithTimeout(context.Background(), *soakDur*2+20*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, args...)
			tmp := filepath.Join(sd, m+".tmp")
			os.MkdirAll(tmp, 0o755)
			defer os.RemoveAll(tmp)
			cmd.Env = append(os.Environ(), "GOPROXY=off", "GOWORK=off", "TMPDIR="+tmp, "GOMODCACHE="+hostModCache())
			if m != "yaegi" {
				var env []string
				for _, kv := range cmd.Env {
					if !strings.HasPrefix(kv, "PATH=") && !strings.HasPrefix(kv, "GOROOT=") {
						env = append(env, kv)
					}
				}
				cmd.Env = append(env, "PATH=/usr/bin:/bin")
			}
			cmd.Env = append(cmd.Env, noNetEnv(extraMode("soak", m))...)
			ef, _ := os.Create(filepath.Join(sd, m+".err"))
			cmd.Stdout, cmd.Stderr = ef, ef
			err := cmd.Run()
			ef.Close()
			rc := "0"
			if err != nil {
				rc = err.Error()
			}
			os.WriteFile(filepath.Join(sd, m+".rc"), []byte(rc+"\n"), 0o644)
		}(m, list)
	}
	wg.Wait()
	rd := filepath.Join(root, "results")
	var s strings.Builder
	fmt.Fprintf(&s, "Traefik plugin catalog, soak, %s: %s of traffic, %d plugins (base scenario ok in every engine) in one process per engine\n",
		time.Now().Format("2006-01-02"), *soakDur, len(names))
	for _, e := range excluded {
		fmt.Fprintf(&s, "  excluded %s\n", e)
	}
	for _, m := range []string{"yaegi", "govm", "aot"} {
		samples, _ := os.ReadFile(filepath.Join(sd, m+".samples.tsv"))
		os.WriteFile(filepath.Join(rd, "soak-"+m+".tsv"), samples, 0o644)
		rc, _ := os.ReadFile(filepath.Join(sd, m+".rc"))
		fmt.Fprintf(&s, "\n%s (exit %s):\n", m, strings.TrimSpace(string(rc)))
		s.WriteString(soakTrend(string(samples)))
		t, _ := os.ReadFile(filepath.Join(sd, m+".txt"))
		for _, l := range strings.Split(string(t), "\n") {
			// Only the lines that need a look: load failures, panics, timeouts.
			if strings.HasPrefix(l, "soak ") || strings.Contains(l, " error") ||
				(strings.HasPrefix(l, "plugin ") && !strings.Contains(l, "panics 0 timeouts 0")) {
				fmt.Fprintf(&s, "  %s\n", l)
			}
		}
	}
	os.WriteFile(filepath.Join(rd, "soak.txt"), []byte(s.String()), 0o644)
	fmt.Print(s.String())
}

// fatalInConc lists the conc hosts of p that ended on a fatal error.
func fatalInConc(p *plugin) string {
	var l []string
	for _, m := range extraModes {
		b, _ := os.ReadFile(filepath.Join(root, "out", p.name, extraMode("conc", m)+".err"))
		if strings.Contains(string(b), "\nfatal error: ") || strings.HasPrefix(string(b), "fatal error: ") {
			l = append(l, m)
		}
	}
	return strings.Join(l, ", ")
}

// soakTrend summarizes the samples: the values after the load, at the first
// quarter of the traffic (warm), at the end, and after the context of New is
// canceled.
func soakTrend(samples string) string {
	var rows [][]string
	for _, l := range strings.Split(strings.TrimSpace(samples), "\n")[1:] {
		rows = append(rows, strings.Split(l, "\t"))
	}
	if len(rows) == 0 {
		return "  no samples\n"
	}
	pick := func(phase string, quarter bool) []string {
		var tr [][]string
		for _, r := range rows {
			if r[0] == phase {
				tr = append(tr, r)
			}
		}
		if len(tr) == 0 {
			return nil
		}
		if quarter {
			return tr[len(tr)/4]
		}
		return tr[len(tr)-1]
	}
	var b strings.Builder
	b.WriteString("  phase          s      heap MB  inuse MB  sys MB   rss MB   goroutines  requests\n")
	for _, ph := range []struct {
		label, phase string
		quarter      bool
	}{{"loaded", "loaded", false}, {"warm (1/4)", "traffic", true}, {"end", "end", false}, {"after cancel", "after-cancel", false}} {
		r := pick(ph.phase, ph.quarter)
		if r == nil {
			continue
		}
		fmt.Fprintf(&b, "  %-13s %6s %9s %9s %8s %8s %10s %9s\n", ph.label, r[1], r[2], r[3], r[4], r[5], r[6], r[7])
	}
	return b.String()
}
