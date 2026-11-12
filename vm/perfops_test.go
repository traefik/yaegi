package vm

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// perfPlugin runs one small operation per request, chosen by Config.Mode:
// the operations that plugins do often and that the VM made faster or made
// allocate less. TestPerfOpsAllocs bounds their allocations (a regression
// test of each optimization); BenchmarkPerfOps measures their time:
//
//	go test ./vm -run '^$' -bench BenchmarkPerfOps -benchmem
const perfPlugin = `package p

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Config struct{ Mode string }

func CreateConfig() *Config { return &Config{} }

type h struct {
	mode string
	n    int
	m    map[string]int
	keys []string
	buf  []byte
	ints []int
}

func New(ctx context.Context, next http.Handler, c *Config, name string) (http.Handler, error) {
	x := &h{mode: c.Mode, m: map[string]int{}, buf: []byte("0123456789abcdef"), ints: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
	for _, k := range []string{"alpha", "beta", "gamma", "delta"} {
		x.m[k] = len(k)
		x.keys = append(x.keys, k)
	}
	return x, nil
}

func (x *h) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch x.mode {
	case "field": // fields read through a native pointer
		x.n += len(req.Method) + len(req.Host) + req.ProtoMajor + len(req.RequestURI)
	case "mapkey": // lookups and stores of string keys in a map of the plugin
		for _, k := range x.keys {
			x.n += x.m[k]
			x.m[k]++
		}
	case "mapnew": // a new map of the plugin, filled, read and ranged over
		m := map[string]int{}
		for i, k := range x.keys {
			m[k] = i
		}
		for k, v := range m {
			x.n += len(k) + v + m[k]
		}
	case "range": // a range over a native map
		for k, v := range req.Header {
			x.n += len(k) + len(v)
		}
	// native calls with the parameters that the generated trampolines
	// take since TODO.md section 5 ("Trampolines still missing")
	case "join": // []string
		x.n += len(strings.Join(x.keys, ","))
	case "hex": // []byte
		x.n += len(hex.EncodeToString(x.buf))
	case "copy": // io.Writer and io.Reader
		n, _ := io.Copy(io.Discard, strings.NewReader("abcdef"))
		x.n += int(n)
	// funcs of the plugin called back by native code (callbacks.go)
	case "search": // sort.Search over 16 ints: 5 calls
		x.n += sort.Search(len(x.ints), func(i int) bool { return x.ints[i] >= 9 })
	case "fields": // strings.FieldsFunc: one call per rune
		x.n += len(strings.FieldsFunc("a,b;c d,e;f", func(r rune) bool { return r == ',' || r == ';' }))
	case "date": // 8 parameters
		x.n += time.Date(2026, 10, 23, 5, 30, x.n%60, 0, time.UTC).Second()
	}
}
`

// loadPerfPlugin loads perfPlugin and returns its handler for mode.
func loadPerfPlugin(tb testing.TB, mode string) http.Handler {
	tb.Helper()
	if !stdtypes.Available() {
		tb.Skip("no embedded stdlib types for this platform")
	}
	dir := tb.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(perfPlugin), 0o644); err != nil {
		tb.Fatal(err)
	}
	p, err := Load(Options{Dir: dir}, "example.com/p")
	if err != nil {
		tb.Fatal(err)
	}
	c, err := createConfig(p)
	if err != nil {
		tb.Fatal(err)
	}
	if err := c.SetFields(map[string]any{"Mode": mode}); err != nil {
		tb.Fatal(err)
	}
	hd, err := newHandler(context.Background(), p, http.NotFoundHandler(), c, "p")
	if err != nil {
		tb.Fatal(err)
	}
	return hd
}

func perfRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "http://x/a/42", nil)
	for i := range 8 {
		req.Header.Set(fmt.Sprintf("X-H%d", i), "v")
	}
	return req
}

var perfModes = []struct {
	mode string
	max  float64 // allocations more than an empty request
}{
	// The 3 strings; before: 7 (each field read made a boxed address).
	{"field", 3},
	// Before: 12 (each string key was boxed again for the Go map).
	{"mapkey", 0},
	// The map and its table; before: 6 (a second table for the keys).
	{"mapnew", 4},
	// 8 entries, 3 each (the key string, the view of the value); before:
	// 31 (a new iterator, its reflect.MapIter and its slots at each range).
	{"range", 25},
	// The generated trampolines; before, the general path: 26, 7, 17, 15.
	{"join", 3},
	{"hex", 2},
	{"copy", 7},
	{"date", 3},
	// The callback trampolines; before, reflect.MakeFunc: 43, 87.
	{"search", 22},
	{"fields", 31},
}

func TestPerfOpsAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates on its own: the counts mean nothing")
	}
	req := perfRequest()
	allocs := func(mode string) float64 {
		hd := loadPerfPlugin(t, mode)
		rec := httptest.NewRecorder()
		hd.ServeHTTP(rec, req) // compiles the code
		// AllocsPerRun counts the allocations of the whole process: the
		// smallest of a few measures is the cost of the call.
		n := math.Inf(1)
		for range 3 {
			n = min(n, testing.AllocsPerRun(200, func() { hd.ServeHTTP(rec, req) }))
		}
		return n
	}
	base := allocs("")
	for _, c := range perfModes {
		if got := allocs(c.mode) - base; got > c.max {
			t.Errorf("%s: %.1f allocations more than an empty request, want at most %.0f", c.mode, got, c.max)
		}
	}
}

func BenchmarkPerfOps(b *testing.B) {
	req := perfRequest()
	for _, c := range append([]struct {
		mode string
		max  float64
	}{{"", 0}}, perfModes...) {
		name := c.mode
		if name == "" {
			name = "empty"
		}
		b.Run(name, func(b *testing.B) {
			hd := loadPerfPlugin(b, c.mode)
			rec := httptest.NewRecorder()
			hd.ServeHTTP(rec, req)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				hd.ServeHTTP(rec, req)
			}
		})
	}
}
