//go:build linux

// Command loadmany loads N distinct copies of one plugin in one process, as
// Traefik loads N plugins, and prints what each extra plugin costs in heap
// and resident memory, and the load time. The copies are the .go files of
// one plugin directory, each in its own module (example.com/plugN), so that
// nothing is shared between them but what the VM shares itself.
//
//	loadmany [-traefik] [-aot] [-n 50] [-at 1,10,50] [-heapprofile f] dir
//
// It prints one line per checkpoint of -at:
//
//	plugins=N live=MB rss=MB per_plugin_live=MB per_plugin_rss=MB load=ms
//
// live and rss are measured after a GC; per_plugin_* is the cost from the
// first plugin to the N-th one, divided by N-1; load is the mean load time
// of the copies loaded since the previous checkpoint.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/internal/pluginhost"
	_ "github.com/traefik/yaegi/v2/natsyms/all"
	"github.com/traefik/yaegi/v2/vm"
)

func status(field string) float64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, field+":"); ok {
			var kb float64
			fmt.Sscan(strings.TrimSuffix(strings.TrimSpace(v), " kB"), &kb)
			return kb / 1024
		}
	}
	return 0
}

func liveMB() float64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc) / (1 << 20)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "loadmany: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	traefik := flag.Bool("traefik", false, "start from pluginhost.Options(), the options of the catalog host")
	aot := flag.Bool("aot", false, "AOT mode (Options.AOT)")
	n := flag.Int("n", 50, "number of copies")
	at := flag.String("at", "1,10,50", "comma-separated checkpoints")
	heapProf := flag.String("heapprofile", "", "write a heap profile after the last load and a GC")
	flag.Parse()
	if flag.NArg() != 1 {
		fail("usage: loadmany [flags] dir")
	}
	files, err := filepath.Glob(filepath.Join(flag.Arg(0), "*.go"))
	if err != nil || len(files) == 0 {
		fail("no .go file in %s", flag.Arg(0))
	}
	checks := map[int]bool{}
	for _, s := range strings.Split(*at, ",") {
		if c, err := strconv.Atoi(s); err == nil {
			checks[c] = true
		}
	}
	tmp, err := os.MkdirTemp("", "loadmany")
	if err != nil {
		fail("%v", err)
	}
	defer os.RemoveAll(tmp)
	for i := 1; i <= *n; i++ {
		d := filepath.Join(tmp, fmt.Sprintf("plug%d", i))
		if err := os.MkdirAll(d, 0o755); err != nil {
			fail("%v", err)
		}
		mod := fmt.Sprintf("module example.com/plug%d\n\ngo 1.22\n", i)
		if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte(mod), 0o644); err != nil {
			fail("%v", err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				fail("%v", err)
			}
			if err := os.WriteFile(filepath.Join(d, filepath.Base(f)), b, 0o644); err != nil {
				fail("%v", err)
			}
		}
	}

	base := liveMB()
	fmt.Printf("plugins=0 live=%.1f rss=%.1f\n", base, status("VmRSS"))
	var keep []*vm.Plugin
	var live1, rss1 float64
	var loadTime time.Duration
	loaded := 0
	for i := 1; i <= *n; i++ {
		var opts vm.Options
		if *traefik {
			opts = pluginhost.Options()
		}
		opts.Dir = filepath.Join(tmp, fmt.Sprintf("plug%d", i))
		opts.AOT = opts.AOT || *aot
		t := time.Now()
		p, err := vm.Load(opts, fmt.Sprintf("example.com/plug%d", i))
		if err != nil {
			fail("load %d: %v", i, err)
		}
		loadTime += time.Since(t)
		loaded++
		keep = append(keep, p)
		if !checks[i] && i != *n {
			continue
		}
		live, rss := liveMB(), status("VmRSS")
		if i == 1 {
			live1, rss1 = live, rss
		}
		line := fmt.Sprintf("plugins=%d live=%.1f rss=%.1f", i, live, rss)
		if i > 1 {
			line += fmt.Sprintf(" per_plugin_live=%.2f per_plugin_rss=%.2f", (live-live1)/float64(i-1), (rss-rss1)/float64(i-1))
		}
		line += fmt.Sprintf(" load=%.1fms", float64(loadTime.Microseconds())/1000/float64(loaded))
		fmt.Println(line)
		loadTime, loaded = 0, 0
	}
	if *heapProf != "" {
		runtime.GC()
		f, err := os.Create(*heapProf)
		if err != nil {
			fail("%v", err)
		}
		if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
			fail("%v", err)
		}
		f.Close()
	}
	runtime.KeepAlive(keep)
}
