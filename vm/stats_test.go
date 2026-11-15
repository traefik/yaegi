package vm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const statsPlugSrc = `package p

import (
	"context"
	"net/http"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

func deep(n int) int { return deep(n+1) + 1 }

func Boom() { panic("boom") }

var stop = make(chan struct{})

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/ok":
		w.Write([]byte("ok"))
	case "/panic":
		panic("boom")
	case "/spin":
		for {
		}
	case "/deep":
		deep(0)
	case "/bg":
		// A background goroutine that works without end, with short
		// sleeps: the BackgroundRate slows it down.
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				for i := 0; i < 1000; i++ {
				}
				time.Sleep(time.Microsecond)
			}
		}()
	case "/stop":
		close(stop)
	}
}
`

// TestStats checks Machine.Stats: the requests and calls from the host,
// the panics and the stops of each kind, the steps, and the time that
// the BackgroundRate made the background goroutines wait. Before, a host
// had BudgetStops (ServeHTTP only) and nothing for the panics, the steps,
// the other stops or the background throttling (round 3 review).
func TestStats(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, statsPlugSrc)
	var mu sync.Mutex
	var goPanics []any
	p, err := Load(Options{
		Dir: dir, Budget: 1e6, BudgetRate: 1e6, BackgroundRate: 1e5, MaxStack: 1 << 20,
		RecoverHandler: true, QuietBudget: true,
		GoPanic: func(r any) { mu.Lock(); goPanics = append(goPanics, r); mu.Unlock() },
	}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	if s := p.M.Stats(); s != (Stats{}) {
		t.Errorf("Stats after the load: %+v, want zero", s)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		t.Fatal(err)
	}
	serve := func(path string) {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	for _, path := range []string{"/ok", "/ok", "/panic", "/spin", "/spin", "/deep", "/bg"} {
		serve(path)
	}
	if !waitFor(10*time.Second, func() bool { return p.M.Stats().BackgroundWait > 0 }) {
		t.Errorf("BackgroundWait is 0 after 10 s of a background loop over its BackgroundRate")
	}
	serve("/stop")
	s := p.M.Stats()
	want := Stats{Requests: 8, Calls: 2, Panics: 1, BudgetStops: 2, StackStops: 1}
	got := Stats{Requests: s.Requests, Calls: s.Calls, Panics: s.Panics, BudgetStops: s.BudgetStops, MemoryStops: s.MemoryStops, StackStops: s.StackStops, LostMethodErrors: s.LostMethodErrors}
	if got != want {
		t.Errorf("Stats %+v, want %+v", got, want)
	}
	// Two budget stops of 1e6 steps each, at least.
	if s.Steps < 2e6 {
		t.Errorf("Steps %d, want at least 2e6 (two budget stops)", s.Steps)
	}
	if s.BudgetStops != p.M.BudgetStops() {
		t.Errorf("Stats.BudgetStops %d, Machine.BudgetStops %d", s.BudgetStops, p.M.BudgetStops())
	}
	// Plugin.Call: a panic of the function is counted too.
	if _, err := p.Call(context.Background(), "CreateConfig"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call(context.Background(), "Boom"); err == nil {
		t.Fatal("Boom: no error")
	}
	if s := p.M.Stats(); s.Calls != 4 || s.Panics != 2 {
		t.Errorf("after 2 calls: Calls %d, Panics %d, want 4, 2", s.Calls, s.Panics)
	}
}

// TestBudgetReportAgain checks that with QuietBudget the budget stops of a
// machine go to GoPanic again, at most once per BudgetReportInterval and
// with the number of new stops (*BudgetStopsError). Before, only the first
// stop of the machine was reported: the Machine of a plugin lives as long
// as the host, so a plugin that started to loop weeks after its first stop
// was not seen.
func TestBudgetReportAgain(t *testing.T) {
	var now atomic.Int64
	now.Store(int64(time.Hour))
	clock := func() int64 { return now.Load() }
	budgetNowTest.Store(&clock)
	defer budgetNowTest.Store(nil)

	var goPanics []any
	p := loadRatePlug(t, Options{Budget: 1e6, RecoverHandler: true, QuietBudget: true, GoPanic: func(r any) { goPanics = append(goPanics, r) }})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	spin := func(n int) {
		for range n {
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/spin", nil))
		}
	}
	spin(3)
	if len(goPanics) != 1 || goPanics[0] != ErrBudgetStopped {
		t.Fatalf("GoPanic got %v, want one ErrBudgetStopped", goPanics)
	}
	now.Add(int64(BudgetReportInterval) - 1)
	spin(1)
	if len(goPanics) != 1 {
		t.Fatalf("GoPanic got %v before BudgetReportInterval, want one report", goPanics)
	}
	now.Add(1)
	spin(2)
	if len(goPanics) != 2 {
		t.Fatalf("GoPanic got %v after BudgetReportInterval, want two reports", goPanics)
	}
	var e *BudgetStopsError
	if !errors.As(goPanics[1].(error), &e) || !errors.Is(e, ErrBudget) || e.Stops != 4 || e.Total != 5 || e.Since != BudgetReportInterval {
		t.Errorf("second report %v, want 4 new stops, 5 in total, since %v", goPanics[1], BudgetReportInterval)
	}
	if !strings.Contains(e.Error(), "4 more requests") {
		t.Errorf("message %q", e.Error())
	}
	// No new stop: no report.
	now.Add(int64(10 * BudgetReportInterval))
	if len(goPanics) != 2 || p.M.BudgetStops() != 6 {
		t.Errorf("GoPanic got %v, %d stops", goPanics, p.M.BudgetStops())
	}
}

// TestCallErrorPrefix checks that the errors of Plugin.Call do not repeat
// the prefix of the errors of the VM: "vm: execution budget exhausted",
// not "vm: vm: execution budget exhausted".
func TestCallErrorPrefix(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, "package p\n\nfunc Spin() { for {} }\n\nfunc Boom() { panic(\"boom\") }\n")
	p, err := Load(Options{Dir: dir, Budget: 1e6}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Call(context.Background(), "Spin")
	if !errors.Is(err, ErrBudget) || err.Error() != ErrBudget.Error() {
		t.Errorf("Spin: %v, want %q", err, ErrBudget)
	}
	if _, err := p.Call(context.Background(), "Boom"); err == nil || !strings.HasPrefix(err.Error(), "vm: panic: boom") {
		t.Errorf("Boom: %v, want %q", err, "vm: panic: boom")
	}
	p.M.Close()
	if _, err := p.Call(context.Background(), "Boom"); !errors.Is(err, ErrClosed) || strings.Contains(err.Error(), "vm: vm:") {
		t.Errorf("after Close: %v, want %q", err, ErrClosed)
	}
}
