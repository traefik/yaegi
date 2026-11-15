package vm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// Tests of Options.BudgetRate (the budget of the calls from the host is a
// token bucket: Budget steps of burst, BudgetRate steps back per second),
// of the weights of native calls and of select, and of the quiet end of a
// ServeHTTP that the budget stops (budget.go).

const ratePlugSrc = `package p

import (
	"bytes"
	"context"
	"net/http"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// wrapper is a wrapper of the ResponseWriter that reads each byte, as a
// plugin that rewrites the body does.
type wrapper struct {
	http.ResponseWriter
	n int
}

func (w *wrapper) Write(b []byte) (int, error) {
	for i := range b {
		if b[i] == 'x' {
			w.n++
		}
	}
	return w.ResponseWriter.Write(b)
}

type wrap struct{ next http.Handler }

func (h wrap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.next.ServeHTTP(&wrapper{ResponseWriter: w}, r)
}

var (
	buf     = bytes.Repeat([]byte("abcdefgh"), 1<<17) // 1 MiB
	big     = bytes.Repeat([]byte("abcdefgh"), 1<<20) // 8 MiB
	sink    int
	total   atomic.Int64 // sink of the goroutines of /bgcount
	count   atomic.Int64
	stopped atomic.Bool
)

func work(n int) int64 {
	var s int64
	for i := 0; i < n; i++ {
		s += int64(i)
	}
	return s
}

type h struct{}

func (h) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/spin":
		for {
		}
	case "/heavy":
		m := map[int]int{}
		for i := 0; ; i++ {
			m[i&1023]++
		}
	case "/sleepspin":
		for {
			sink += int(work(100000))
			time.Sleep(0)
		}
	case "/select":
		c := make(chan int)
		for {
			select {
			case <-c:
			default:
			}
		}
	case "/count":
		for {
			sink += bytes.Count(buf, []byte("a"))
		}
	case "/deferregexp":
		// A deferred call that does far more work than the grace of a
		// canceled call.
		defer func() {
			re := regexp.MustCompile("a+b")
			sink += len(re.FindAllIndex(big, -1))
			count.Add(1)
		}()
		for {
		}
	case "/regexp":
		re := regexp.MustCompile("a+b")
		w.Write([]byte(strconv.Itoa(len(re.FindAllIndex(big, -1)))))
	case "/bgselect":
		for i := 0; i < 8; i++ {
			go func() {
				c := make(chan int)
				for !stopped.Load() {
					select {
					case <-c:
					default:
					}
					count.Add(1)
				}
			}()
		}
	case "/bgcount":
		for i := 0; i < 4; i++ {
			go func() {
				for !stopped.Load() {
					total.Add(int64(bytes.Count(buf, []byte("a"))))
					count.Add(1)
				}
			}()
		}
	case "/copy":
		// A plugin that copies a stream by itself: it waits in native
		// code (Read), never at a blocking point of the VM.
		b := make([]byte, 16)
		for {
			n, err := r.Body.Read(b)
			if n > 0 {
				w.Write(b[:n])
			}
			if err != nil {
				return
			}
		}
	case "/n":
		w.Write([]byte(strconv.FormatInt(count.Load(), 10)))
	case "/stop":
		stopped.Store(true)
	}
}

func New(_ context.Context, next http.Handler, _ *Config, name string) (http.Handler, error) {
	if name == "wrap" {
		return wrap{next}, nil
	}
	return h{}, nil
}
`

// loadRatePlug loads ratePlugSrc with opts and returns the plugin and the
// handler that New returns for name, with next as its next handler.
func loadRatePlug(t *testing.T, opts Options) *Plugin {
	t.Helper()
	needSource(t)
	dir, pkg := writePlugin(t, ratePlugSrc)
	opts.Dir = dir
	p, err := Load(opts, pkg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func rateHandler(t *testing.T, p *Plugin, next http.Handler, name string) http.Handler {
	t.Helper()
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, next, cfg, name)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// isBudgetStop reports whether r is the panic of a ServeHTTP that the
// budget stopped (without QuietBudget).
func isBudgetStop(r any) bool { return isBudget(r) }

// TestBudgetRateStream: a response written one byte at a time through 1, 2
// and 3 wrappers of the ResponseWriter runs for far longer than the total
// that Budget gave before (each wrapper uses about 25 steps per byte, 5
// without the weights: 10000 bytes are more than 50000 steps, Budget is
// 20000), since its account gets
// BudgetRate steps back per second. Before, Budget was a total per call:
// a stream of 1-byte writes through one wrapper died after about Budget
// writes (an SSE or gRPC stream at 1000 messages per second, after about a
// day with the host defaults), with "http: panic serving".
func TestBudgetRateStream(t *testing.T) {
	const n = 10000
	p := loadRatePlug(t, Options{Budget: 20000, BudgetRate: 1e6, GoPanic: func(r any) { t.Errorf("GoPanic: %v", r) }})
	src := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := range n {
			w.Write([]byte{'x'})
			if i%10 == 9 {
				time.Sleep(time.Millisecond) // at most 10000 bytes per second
			}
		}
	})
	for wrappers := 1; wrappers <= 3; wrappers++ {
		var h http.Handler = src
		for range wrappers {
			h = rateHandler(t, p, h, "wrap")
		}
		start := time.Now()
		body, r := serveBG(h, "/")
		if r != nil || len(body) != n {
			t.Fatalf("%d wrappers: %d bytes, panic %v; want %d bytes", wrappers, len(body), r, n)
		}
		t.Logf("%d wrappers: %d bytes in %v", wrappers, n, time.Since(start))
	}
	if s := p.M.BudgetStops(); s != 0 {
		t.Fatalf("%d budget stops", s)
	}
	// Without BudgetRate, Budget is still a total per call.
	p0 := loadRatePlug(t, Options{Budget: 20000, GoPanic: func(any) {}})
	if _, r := serveBG(rateHandler(t, p0, src, "wrap"), "/"); !isBudgetStop(r) {
		t.Fatalf("no BudgetRate: panic %v, want the budget stop", r)
	}
}

// TestBudgetRateCopyLoop: a loop of the plugin that copies a slow stream
// by itself (r.Body.Read, then Write) never reaches a blocking point of
// the VM, but runs few steps per second: the steps since its last blocking
// point go down with time (BudgetRate/100 per second), so it runs past
// Budget steps (about 40 steps per round, 1000 rounds; Budget is 20000).
// Without the credit, it stopped after Budget steps.
func TestBudgetRateCopyLoop(t *testing.T) {
	const n = 1000
	p := loadRatePlug(t, Options{Budget: 20000, BudgetRate: 1e7, GoPanic: func(r any) { t.Errorf("GoPanic: %v", r) }})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	pr, pw := io.Pipe()
	go func() {
		for range n {
			pw.Write([]byte{'x'})
			time.Sleep(time.Millisecond)
		}
		pw.Close()
	}()
	rec := httptest.NewRecorder()
	start := time.Now()
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic %v after %d bytes", r, rec.Body.Len())
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/copy", pr))
	}()
	if rec.Body.Len() != n {
		t.Fatalf("%d bytes, want %d", rec.Body.Len(), n)
	}
	t.Logf("%d bytes in %v", n, time.Since(start))
}

// TestBudgetRateLoops: with a BudgetRate, a loop still stops with
// ErrBudget. `for {}` empties the bucket (it runs far more steps per
// second than the rate); a loop that runs fewer steps per second than the
// rate stops after Budget steps without a blocking point (less the credit
// of the time, rate/100 per second); a loop that waits after each round of
// work but uses more than the rate stops when the bucket is empty.
func TestBudgetRateLoops(t *testing.T) {
	for _, c := range []struct {
		path         string
		budget, rate int64
	}{
		{"/spin", 1e7, 1e6},
		// The slice rule: the rate (0 here: 8 times the speed of `for {}`
		// measured below) is above the speed of the loop, so the bucket
		// never empties, and the credit of the slice (rate/100) is far
		// below it. The speed is measured, not written: `for {}` runs 4e8
		// steps per second, but about 3e7 with -race, below the credit of
		// a fixed rate of 2e9 (the loop never stopped).
		{"/spin", 1e7, 0},
		// A map write per round: about 1e7 steps per second, below the
		// rate on this machine (else the bucket stops it).
		{"/heavy", 1e6, 3e7},
		{"/sleepspin", 1e6, 1e6},
	} {
		if c.rate == 0 {
			c.rate = 8 * spinSpeed(t, c.budget)
		}
		p := loadRatePlug(t, Options{Budget: c.budget, BudgetRate: c.rate, GoPanic: func(r any) { t.Errorf("GoPanic: %v", r) }})
		h := rateHandler(t, p, http.NotFoundHandler(), "x")
		start := time.Now()
		for range 2 {
			if _, r := serveBG(h, c.path); !isBudgetStop(r) {
				t.Fatalf("%s %+v: panic %v, want the budget stop", c.path, c, r)
			}
		}
		if d := time.Since(start); d > 20*time.Second {
			t.Errorf("%s %+v: stopped after %v", c.path, c, d)
		}
		if s := p.M.BudgetStops(); s != 2 {
			t.Errorf("%s: %d budget stops, want 2", c.path, s)
		}
		t.Logf("%s %+v: 2 stops in %v", c.path, c, time.Since(start))
	}
}

// TestBudgetRateSlowClock: `for {}` stops with the slice rule also when
// each read of the clock takes time. The clock here moves 5 ns per read:
// with a rate of 1e12 (the bucket never empties), the credit of one read is
// 50 steps. Before, near the end of the slice the limit came a few steps
// after the steps of the loop, each refill credited about the steps run
// since the last one, and the loop never stopped (linux/386, where the
// clock is slow, hung in TestBudgetRateLoops).
func TestBudgetRateSlowClock(t *testing.T) {
	var clock atomic.Int64
	slow := func() int64 { return clock.Add(5) }
	budgetNowTest.Store(&slow)
	defer budgetNowTest.Store(nil)
	p := loadRatePlug(t, Options{Budget: 1e6, BudgetRate: 1e12, GoPanic: func(any) {}})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	done := make(chan any, 1)
	go func() {
		_, r := serveBG(h, "/spin")
		done <- r
	}()
	select {
	case r := <-done:
		if !isBudgetStop(r) {
			t.Fatalf("panic %v, want the budget stop", r)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("for {} still runs after 30 s")
	}
}

// spinSpeed returns the steps per second of `for {}` on this machine (and
// with -race when it is on): the time that a total of budget steps takes.
func spinSpeed(t *testing.T, budget int64) int64 {
	t.Helper()
	p := loadRatePlug(t, Options{Budget: budget, GoPanic: func(any) {}})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	start := time.Now()
	if _, r := serveBG(h, "/spin"); !isBudgetStop(r) {
		t.Fatalf("/spin without rate: panic %v, want the budget stop", r)
	}
	s := int64(float64(budget) / time.Since(start).Seconds())
	t.Logf("for {}: %d steps per second", s)
	return s
}

// TestBudgetWeights: a select and a native call count more than one step,
// and the bytes that a native function reads count too, so that the
// steps follow the CPU. Before, a select with a default counted nothing
// but the backward jump (a loop of selects took 29 s per call to use 1e8
// steps), a call of bytes.Count on 1 MiB counted one step, and a regexp on
// 200 MiB ran 12.9 s in one step.
func TestBudgetWeights(t *testing.T) {
	p := loadRatePlug(t, Options{Budget: 1e7, GoPanic: func(any) {}})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	for _, path := range []string{"/select", "/count", "/regexp"} {
		start := time.Now()
		body, r := serveBG(h, path)
		if !isBudgetStop(r) {
			t.Fatalf("%s: %q, panic %v; want the budget stop", path, body, r)
		}
		// Before: 1e7 selects (2.4 s), 1e7 calls of bytes.Count (170 s),
		// and the regexp on 8 MiB (1.7e7 steps now) ran to its end. Now
		// about 0.04 s, 0.01 s and at once.
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: stopped after %v", path, d)
		}
		t.Logf("%s: stopped after %v", path, time.Since(start))
	}
}

// TestBudgetBackgroundWeights: 8 background goroutines in a select loop
// with a default, and 4 that call bytes.Count on 1 MiB in a loop, run at
// most BackgroundRate weighted steps per second. Before, a select counted
// one step: at the host BackgroundRate of 5e7, 8 such goroutines used 3.7
// cores of 4 for ever, and the bytes.Count loop 3.95.
func TestBudgetBackgroundWeights(t *testing.T) {
	const rate = 1e6
	for _, c := range []struct {
		start string
		steps int64 // weighted steps of one round, at least
	}{
		{"/bgselect", selectSteps + 2*nativeCallSteps},
		{"/bgcount", (1<<20)/64 + 2*nativeCallSteps},
	} {
		p := loadRatePlug(t, Options{Budget: 1e8, BackgroundRate: rate, GoPanic: func(r any) { t.Errorf("GoPanic: %v", r) }})
		h := rateHandler(t, p, http.NotFoundHandler(), "x")
		rounds := func() int64 {
			body, _ := serveBG(h, "/n")
			n, _ := strconv.ParseInt(body, 10, 64)
			return n
		}
		if _, r := serveBG(h, c.start); r != nil {
			t.Fatal(r)
		}
		time.Sleep(1500 * time.Millisecond) // past the burst of one second
		n1, t1 := rounds(), time.Now()
		time.Sleep(time.Second)
		n2, d := rounds(), time.Since(t1)
		serveBG(h, "/stop")
		if !waitFor(30*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
			t.Errorf("%d live goroutines after /stop", p.M.Goroutines())
		}
		// rate/steps rounds per second, plus one chunk (rate/100) per
		// goroutine.
		want := int64(d.Seconds()*rate)/c.steps + 8*(rate/100)/c.steps + 8
		if n2-n1 > 2*want {
			t.Fatalf("%s: %d rounds in %v, want at most about %d", c.start, n2-n1, d, want)
		}
		if n2 <= n1 {
			t.Fatalf("%s: no round in %v: the goroutines must be slowed, not stopped", c.start, d)
		}
		t.Logf("%s: %d rounds in %v, want at most about %d", c.start, n2-n1, d, want)
	}
}

// TestBudgetCancelGraceWeights: the weight of a native call made in the
// deferred calls of a canceled call is taken from the grace (cancelGrace
// steps), so a deferred regexp on 8 MiB (1.7e7 steps) stops before it
// starts. Before, a refill took only minChunk steps from the grace,
// whatever the weight that had passed the limit: the regexp ran to its end
// after ErrCanceled (1.8 s on 64 MiB).
func TestBudgetCancelGraceWeights(t *testing.T) {
	p := loadRatePlug(t, Options{Cancel: true, GoPanic: func(r any) { t.Errorf("GoPanic: %v", r) }})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, r := serveCtx(h, ctx, "/deferregexp"); r != http.ErrAbortHandler {
		t.Fatalf("/deferregexp: panic %v, want %v", r, http.ErrAbortHandler)
	}
	if body, _ := serveBG(h, "/n"); body != "0" {
		t.Fatalf("the deferred regexp ran to its end after the cancel (count %s)", body)
	}
}

// TestBudgetFaultGraceWeights is TestBudgetCancelGraceWeights for
// ErrBudget: the deferred calls that run after the budget is spent share a
// grace, and the weight of a native call counts in it. Before, a refill
// took only minChunk from the grace, so a deferred regexp on a large input
// ran to its end after ErrBudget.
func TestBudgetFaultGraceWeights(t *testing.T) {
	p := loadRatePlug(t, Options{Budget: 1e6, GoPanic: func(any) {}})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	if _, r := serveBG(h, "/deferregexp"); r == nil {
		t.Fatal("/deferregexp: no panic, want the stop of the budget")
	}
	if body, _ := serveBG(h, "/n"); body != "0" {
		t.Fatalf("the deferred regexp ran to its end after ErrBudget (count %s)", body)
	}
}

// TestBudgetQuietServeHTTP: with QuietBudget (host.Options sets it), a
// ServeHTTP that the budget stops panics with http.ErrAbortHandler, so net/http aborts the response without a log; the
// first stop of the machine goes to GoPanic (ErrBudgetStopped, which is
// ErrBudget for errors.Is), and Machine.BudgetStops counts them all.
// Before, net/http logged "http: panic serving" and a full stack for each
// request.
func TestBudgetQuietServeHTTP(t *testing.T) {
	var goPanics []any
	p := loadRatePlug(t, Options{Budget: 1e6, BudgetRate: 1e6, QuietBudget: true, GoPanic: func(r any) { goPanics = append(goPanics, r) }})
	srv := httptest.NewUnstartedServer(rateHandler(t, p, http.NotFoundHandler(), "x"))
	var logs bytes.Buffer
	srv.Config.ErrorLog = log.New(&logs, "", 0)
	srv.Start()
	defer srv.Close()
	for range 3 {
		if resp, err := http.Get(srv.URL + "/spin"); err == nil {
			resp.Body.Close()
			t.Fatalf("status %s, want an aborted response", resp.Status)
		}
	}
	srv.Close() // the log of the last request is written
	if logs.Len() != 0 {
		t.Errorf("server log:\n%s", logs.String())
	}
	if len(goPanics) != 1 || goPanics[0] != ErrBudgetStopped || !errors.Is(ErrBudgetStopped, ErrBudget) {
		t.Errorf("GoPanic got %v, want one ErrBudgetStopped", goPanics)
	}
	if s := p.M.BudgetStops(); s != 3 {
		t.Errorf("%d budget stops, want 3", s)
	}

	// With RecoverHandler, the response gets the status 500, and GoPanic
	// gets only the first stop too.
	goPanics = nil
	p = loadRatePlug(t, Options{Budget: 1e6, RecoverHandler: true, QuietBudget: true, GoPanic: func(r any) { goPanics = append(goPanics, r) }})
	h := rateHandler(t, p, http.NotFoundHandler(), "x")
	for range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/spin", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("RecoverHandler: status %d, want 500", rec.Code)
		}
	}
	if len(goPanics) != 1 || goPanics[0] != ErrBudgetStopped || p.M.BudgetStops() != 2 {
		t.Errorf("RecoverHandler: GoPanic got %v, %d stops; want one ErrBudgetStopped, 2 stops", goPanics, p.M.BudgetStops())
	}

	// Without QuietBudget, the host gets ErrBudget, as for any panic.
	p = loadRatePlug(t, Options{Budget: 1e6, GoPanic: func(r any) { t.Errorf("GoPanic: %v", r) }})
	if _, r := serveBG(rateHandler(t, p, http.NotFoundHandler(), "x"), "/spin"); r != ErrBudget || p.M.BudgetStops() != 1 {
		t.Errorf("no QuietBudget: panic %v, %d stops; want ErrBudget, 1 stop", r, p.M.BudgetStops())
	}
}
