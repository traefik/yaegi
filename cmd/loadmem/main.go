//go:build linux

// Command loadmem loads one plugin with vm.Load, as Traefik does, and
// prints what the load keeps in memory and what the first and the next
// requests cost. One process measures one plugin: run it several times
// and take the median.
//
//	loadmem [-traefik] [-aot] [-unsafe] [-config JSON] [-req get,sqli] [-d 2s] dir import
//
// It prints one line per measure, "name value unit":
//
//	load               time of vm.Load (parse, type check, SSA, check, init)
//	heap_load          heap in use right after vm.Load (no GC)
//	live_load          live heap after vm.Load and a GC
//	rss_load           resident memory after vm.Load and the GC
//	new                CreateConfig, the config, and New
//	live_new           live heap after New and a GC
//	<req>_first        the first request
//	<req>_time         time per request after it, for at least -d
//	<req>_allocs       allocations per request
//	rss_end, rss_peak  resident memory at the end, and its peak
//
// -config is the dynamic configuration of the plugin in JSON (Traefik
// decodes it into the Config of the plugin). The requests are named:
// get, getmany, sqli and post10k are the requests of the Coraza
// benchmark; w1, w2 and w3 the requests of compare/bench.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/internal/pluginhost"
	_ "github.com/traefik/yaegi/v2/natsyms/all"
	"github.com/traefik/yaegi/v2/vm"
)

// setEngine selects another engine than v1 (engine.go of a tree that has
// one); nil here.
var setEngine func(o *vm.Options, name string)

type request struct {
	method, target string
	headers        map[string]string
	body           string
}

func requests() map[string]request {
	browser := map[string]string{
		"User-Agent":      "Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/131.0",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.5",
	}
	many := map[string]string{}
	for k, v := range browser {
		many[k] = v
	}
	var q []string
	for i := 0; i < 20; i++ {
		many[fmt.Sprintf("X-Custom-%02d", i)] = fmt.Sprintf("value-%02d-abcdefghijklmnop", i)
		q = append(q, fmt.Sprintf("arg%02d=value%02d", i, i))
	}
	jsonH := map[string]string{"Content-Type": "application/json"}
	for k, v := range browser {
		jsonH[k] = v
	}
	text := strings.Repeat("The quick brown fox jumps over the lazy dog near the river bank. ", 160)
	return map[string]request{
		"get":     {"GET", "/index.html?q=hello+world&page=2", browser, ""},
		"getmany": {"GET", "/search?" + strings.Join(q, "&"), many, ""},
		"sqli":    {"GET", "/?id=1%27%20OR%20%271%27%3D%271", browser, ""},
		"post10k": {"POST", "/api/reviews", jsonH, `{"title":"A product review","rating":4,"text":"` + text + `"}`},
		"w1": {"GET", "http://example.com/foo/bar?x=1", map[string]string{
			"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			"Accept":     "text/html,application/xhtml+xml", "Accept-Encoding": "gzip, br",
			"X-Forwarded-Proto": "https", "Authorization": "Bearer abc.def.ghi",
			"X-Custom-A": "a", "X-Custom-B": "b", "X-Debug": "1", "Cookie": "session=xyz",
			"X-Real-Ip": "10.0.0.1",
		}, ""},
		"w2": {"POST", "http://example.com/api/cart", map[string]string{"Content-Type": "application/json"}, `{"user":"alice@example.com","level":2,"active":true,"items":[` +
			`{"id":1,"name":"keyboard","price":49.9,"qty":1,"tags":["hw","usb"]},` +
			`{"id":2,"name":"mouse","price":19.5,"qty":2,"tags":["hw"]},` +
			`{"id":3,"name":"monitor 27 inches","price":259,"qty":1,"tags":["hw","display","4k"]},` +
			`{"id":4,"name":"cable","price":5.25,"qty":4},` +
			`{"id":5,"name":"dock","price":129.99,"qty":1,"tags":["hw","usb-c"]},` +
			`{"id":6,"name":"headset","price":79,"qty":1,"tags":["audio"]},` +
			`{"id":7,"name":"webcam","price":64.5,"qty":1,"tags":["video","usb"]},` +
			`{"id":8,"name":"chair","price":349,"qty":1,"tags":["furniture"]}` +
			`],"meta":{"source":"checkout","region":"eu-west-1","cart":"c-42-17","session":"s-3f9a0b"},` +
			`"total":0}`},
		"w3": {"GET", "http://example.com/api/v2/users/123456/profile/settings", nil, ""},
	}
}

// backend is the next handler: it reads the body and answers 200.
func backend(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "ok %s %s body=%d\n", r.Method, r.URL.Path, len(b))
}

func serve(h http.Handler, r request) int {
	var body io.Reader = http.NoBody
	if r.body != "" {
		body = strings.NewReader(r.body)
	}
	req := httptest.NewRequest(r.method, r.target, body)
	req.RemoteAddr = "192.0.2.10:54321"
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// status returns a field of /proc/self/status in MB.
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

func heapMB(gc bool) float64 {
	if gc {
		runtime.GC()
		runtime.GC()
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc) / (1 << 20)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "loadmem: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	traefik := flag.Bool("traefik", false, "start from pluginhost.Options() (AOT mode, budget, goroutine cap, load timeout, empty environment), the options of the catalog host")
	aot := flag.Bool("aot", false, "AOT mode (Options.AOT)")
	loadOnly := flag.Bool("loadonly", false, "stop after the load")
	unsafe := flag.Bool("unsafe", false, "Options.UseUnsafe")
	engine := flag.String("engine", "", "Options.Engine, when this tree has engines")
	config := flag.String("config", "", "dynamic configuration of the plugin, in JSON")
	reqs := flag.String("req", "get", "comma-separated requests")
	d := flag.Duration("d", 2*time.Second, "minimum time per request kind")
	minN := flag.Int("n", 20, "minimum number of requests per kind")
	heapProf := flag.String("heapprofile", "", "write a heap profile after the load and a GC")
	flag.Parse()
	if flag.NArg() != 2 {
		fail("usage: loadmem [flags] dir import")
	}
	dir, imp := flag.Arg(0), flag.Arg(1)
	var opts vm.Options
	if *traefik {
		opts = pluginhost.Options()
	}
	opts.Dir, opts.UseUnsafe = dir, *unsafe
	opts.AOT = opts.AOT || *aot
	if *engine != "" {
		if setEngine == nil {
			fail("this build has no engine %q", *engine)
		}
		setEngine(&opts, *engine)
	}
	table := requests()
	var names []string
	for _, n := range strings.Split(*reqs, ",") {
		if n == "" {
			continue
		}
		if _, ok := table[n]; !ok {
			fail("unknown request %q", n)
		}
		names = append(names, n)
	}

	fmt.Printf("live_base %.1f MB\n", heapMB(true))
	t := time.Now()
	p, err := vm.Load(opts, imp)
	if err != nil {
		fail("load: %v", err)
	}
	fmt.Printf("load %.1f ms\n", ms(time.Since(t)))
	fmt.Printf("heap_load %.1f MB\n", heapMB(false))
	fmt.Printf("live_load %.1f MB\n", heapMB(true))
	fmt.Printf("rss_load %.1f MB\n", status("VmRSS"))
	if *heapProf != "" {
		f, err := os.Create(*heapProf)
		if err != nil {
			fail("%v", err)
		}
		if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
			fail("%v", err)
		}
		f.Close()
	}
	if *loadOnly {
		fmt.Printf("rss_peak %.1f MB\n", status("VmHWM"))
		runtime.KeepAlive(p)
		return
	}

	t = time.Now()
	cfg, err := pluginhost.CreateConfig(p)
	if err != nil {
		fail("CreateConfig: %v", err)
	}
	if *config != "" {
		err := cfg.Decode(func(ptr any) error { return json.Unmarshal([]byte(*config), ptr) })
		if err != nil {
			fail("config: %v", err)
		}
	}
	h, err := pluginhost.NewHandler(context.Background(), p, http.HandlerFunc(backend), cfg, "loadmem")
	if err != nil {
		fail("New: %v", err)
	}
	fmt.Printf("new %.1f ms\n", ms(time.Since(t)))
	fmt.Printf("live_new %.1f MB\n", heapMB(true))
	fmt.Printf("rss_new %.1f MB\n", status("VmRSS"))

	for _, name := range names {
		r := table[name]
		t := time.Now()
		code := serve(h, r)
		fmt.Printf("%s_first %.3f ms\n", name, ms(time.Since(t)))
		fmt.Printf("%s_status %d code\n", name, code)
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		t = time.Now()
		n := 0
		for n < *minN || time.Since(t) < *d {
			if c := serve(h, r); c != code {
				fail("%s: status %d, then %d", name, code, c)
			}
			n++
		}
		el := time.Since(t)
		runtime.ReadMemStats(&m1)
		fmt.Printf("%s_time %.4f ms\n", name, ms(el)/float64(n))
		fmt.Printf("%s_allocs %.0f allocs\n", name, float64(m1.Mallocs-m0.Mallocs)/float64(n))
	}
	fmt.Printf("live_end %.1f MB\n", heapMB(true))
	fmt.Printf("rss_end %.1f MB\n", status("VmRSS"))
	fmt.Printf("rss_peak %.1f MB\n", status("VmHWM"))
	runtime.KeepAlive(p)
}
