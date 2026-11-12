package vm

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

const natfastPlugin = `package p

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Config struct{ Mode string }

func CreateConfig() *Config { return &Config{} }

type h struct {
	mode string
	re   *regexp.Regexp
	n    int
	in   string
	s    string
}

func New(ctx context.Context, next http.Handler, c *Config, name string) (http.Handler, error) {
	return &h{mode: c.Mode, in: "/a/42", re: regexp.MustCompile("^/a/(\\d+)$")}, nil
}

func (x *h) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch x.mode {
	case "range":
		for k := range req.Header {
			x.n += len(k)
		}
	case "index":
		x.n += strings.Index(x.in, "/")
	case "sprintf":
		x.s = fmt.Sprintf("%d-%d", x.n, 7)
	case "method":
		x.n += len(x.re.FindStringSubmatch(x.in))
	case "duration":
		x.s = (3 * time.Second).String()
	}
}
`

// TestNatfastAllocs checks the allocations of native calls that have a
// direct path (natfast.go, zz_natsigs.go) and of the range over a native
// map (features.go), against an empty request. The general path allocated
// about 4 times more for each of them.
func TestNatfastAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates on its own: the counts mean nothing")
	}
	needSource(t)
	dir, pkg := writePlugin(t, natfastPlugin)
	p, err := Load(Options{Dir: dir}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://x/a/42", nil)
	for i := range 20 {
		req.Header.Set(fmt.Sprintf("X-H%d", i), "v")
	}
	allocs := func(mode string) float64 {
		c, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.SetFields(map[string]any{"Mode": mode}); err != nil {
			t.Fatal(err)
		}
		hd, err := newHandler(context.Background(), p, http.NotFoundHandler(), c, "p")
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		hd.ServeHTTP(rec, req) // compiles the code
		// AllocsPerRun counts the allocations of the whole process: in a
		// full run, goroutines that other tests left add to it. They only
		// add, so the smallest of a few measures is the cost of the call.
		n := math.Inf(1)
		for range 3 {
			n = min(n, testing.AllocsPerRun(200, func() { hd.ServeHTTP(rec, req) }))
		}
		return n
	}
	base := allocs("")
	for _, c := range []struct {
		mode string
		max  float64
	}{
		{"range", 30}, // 20 keys: one string each, plus the iterator
		{"index", 1},
		{"sprintf", 12},
		{"method", 4},
		{"duration", 3},
	} {
		if got := allocs(c.mode) - base; got > c.max {
			t.Errorf("%s: %.1f allocations more than an empty request, want at most %.0f", c.mode, got, c.max)
		}
	}
}
