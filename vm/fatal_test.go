package vm

import (
	"context"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// Tests of Options.PanicOnFatal (fatal.go). Plugin:
// testdata/restricted/fatalplug. Without the option, each of these routes
// ends the test process with a fatal error of the runtime, as in Go.

const fatalPlug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/fatalplug"

func loadFatalPlug(t *testing.T, opts Options) http.Handler {
	t.Helper()
	opts.Dir = ".."
	p, err := Load(opts, fatalPlug)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestPanicOnFatal: with PanicOnFatal, an unlock of an unlocked
// sync.Mutex or sync.RWMutex of the plugin, and concurrent writes to a map
// of the plugin (or a read or a range during a write) are panics that the
// plugin recovers, instead of fatal errors that end the host. The correct
// uses of locks and maps give no panic.
func TestPanicOnFatal(t *testing.T) {
	h := loadFatalPlug(t, Options{PanicOnFatal: true})
	serve := func(path string) string {
		body, r := serveBG(h, path)
		if r != nil {
			t.Fatalf("%s: panic %v", path, r)
		}
		return body
	}
	if got := serve("/locks"); got != "ok\nok\n" {
		t.Errorf("/locks: %q, want no panic", got)
	}
	want := strings.Join([]string{
		errUnlockMutex.Error(), // Unlock
		errUnlockMutex.Error(), // through sync.Locker
		errUnlockMutex.Error(), // method value
		errUnlockMutex.Error(), // deferred
		errUnlockRWMutex.Error(),
		errRUnlockRW.Error(),
		errUnlockRWMutex.Error(), // read-locked
		errRUnlockRW.Error(),     // write-locked
		"ok",
	}, "\n") + "\n"
	if got := serve("/unlock"); got != want {
		t.Errorf("/unlock:\n%s\nwant:\n%s", got, want)
	}
	if got := serve("/mapok"); got != "ok ok 40000\n" {
		t.Errorf("/mapok: %q, want no panic", got)
	}
	if raceEnabled {
		// These routes race on purpose: the race detector reports them, as
		// it does for the same program under go run -race.
		return
	}
	for path, errs := range map[string][]*FatalError{
		"/mapwrites":    {errMapWrites, errMapReadWrite},
		"/mapreadwrite": {errMapReadWrite, errMapWrites},
		"/maprange":     {errMapIterWrite, errMapReadWrite},
	} {
		got := strings.TrimSpace(serve(path))
		// On one CPU the goroutines seldom overlap, in Go as here: no
		// panic is then right too.
		found := got == "ok" && runtime.GOMAXPROCS(0) < 2
		for _, e := range errs {
			found = found || got == e.Error()
		}
		if !found {
			t.Errorf("%s: %q, want the panic %q", path, got, errs[0].Error())
		}
	}
}

// TestFatalLockLayout: the replacements of Unlock and RUnlock read the state
// words of sync.Mutex and sync.RWMutex. When a Go release changes their
// layout, the replacements keep the Go behaviour; this test fails so that
// fatal.go follows the new layout.
func TestFatalLockLayout(t *testing.T) {
	l := locks()
	if !l.ok {
		t.Fatal("the layout of sync.Mutex or sync.RWMutex is not the expected one")
	}
	var mu sync.Mutex
	p := unsafe.Pointer(&mu)
	if lockWord(p, l.mutexState)&1 != 0 {
		t.Error("a new Mutex looks locked")
	}
	mu.Lock()
	if lockWord(p, l.mutexState)&1 == 0 {
		t.Error("a locked Mutex looks unlocked")
	}
	mu.Unlock()
	var rw sync.RWMutex
	q := unsafe.Pointer(&rw)
	rw.RLock()
	if r := lockWord(q, l.readerCount); r != 1 {
		t.Errorf("reader count %d with one reader, want 1", r)
	}
	rw.RUnlock()
	rw.Lock()
	if r := lockWord(q, l.readerCount); r != -rwmutexMaxReaders {
		t.Errorf("reader count %d while locked for writing, want %d", r, -rwmutexMaxReaders)
	}
	rw.Unlock()
}

// TestPanicOnFatalHost: with PanicOnFatal, the plugin cannot end the host
// with a Wait on a sync.Cond whose lock is not held, an Unlock of an
// RLocker that is not locked, a misuse of runtime.SetFinalizer (also by
// goroutines that set the finalizer of the same object at once), or a nil
// func that native code starts later on a goroutine of its own
// (time.AfterFunc, context.AfterFunc, WaitGroup.Go, RegisterOnShutdown).
// Each of them is a panic that the plugin recovers; without the option,
// each one ends the test process. The correct uses give no panic. Also in
// AOT mode, the mode of internal/pluginhost.
func TestPanicOnFatalHost(t *testing.T) {
	fin := func(msg string) string {
		return (&FatalError{"runtime.SetFinalizer: " + msg}).Error()
	}
	routes := []struct{ path, want string }{
		{"/cond", strings.Join([]string{
			errUnlockMutex.Error(),
			errUnlockRWMutex.Error(),
			errRUnlockRW.Error(), // RLocker
			errRUnlockRW.Error(), // RLocker().Unlock
			"ok",
		}, "\n")},
		{"/nilfunc", strings.Join([]string{
			errGoNilFunc.Error(), // time.AfterFunc
			errGoNilFunc.Error(), // context.AfterFunc
			errWaitGroupGo.Error(),
			errGoNilFunc.Error(), // RegisterOnShutdown
			"ok",
		}, "\n")},
		{"/finalizer", strings.Join([]string{
			fin("first argument is nil"),
			fin("first argument is fatalplug.pair, not pointer"),
			fin("first argument is int, not pointer"),
			fin("pointer not in allocated block"),
			fin("pointer not at beginning of allocated block"),
			fin("second argument is int, not a function"),
			fin("cannot pass *fatalplug.pair to finalizer func(int)"),
			fin("cannot pass *fatalplug.pair to finalizer func(*fatalplug.pair, int)"),
			fin("cannot pass *fatalplug.pair to finalizer func(...*fatalplug.pair) because dotdotdot"),
			fin("finalizer already set"),
			"ok", "ok", "ok", "ok", "ok",
		}, "\n")},
		{"/finalizerinner", strings.Join([]string{
			fin("pointer not at beginning of allocated block"), // struct field
			fin("pointer not at beginning of allocated block"), // array element
			fin("pointer not at beginning of allocated block"), // native field
			fin("pointer not at beginning of allocated block"), // native field, nil
			fin("finalizer already set"),                       // &b[0] twice
		}, "\n")},
		{"/finalizerstart", "ok\nok\nok\nok\nok\nok\nok"},
		{"/finalizerrace", "ok"},
	}
	for _, aot := range []bool{false, true} {
		var goPanics atomic.Int32
		h := loadFatalPlug(t, Options{PanicOnFatal: true, AOT: aot, GoPanic: func(any) { goPanics.Add(1) }})
		for _, r := range routes {
			body, p := serveBG(h, r.path)
			if p != nil {
				t.Fatalf("aot=%v %s: panic %v", aot, r.path, p)
			}
			if body != r.want+"\n" {
				t.Errorf("aot=%v %s:\n%s\nwant:\n%s", aot, r.path, body, r.want)
			}
		}
		runtime.GC() // the finalizers of /finalizer run, and do nothing
		if n := goPanics.Load(); n != 0 {
			t.Errorf("aot=%v: %d panics of plugin goroutines, want none", aot, n)
		}
	}
}

// TestFinalizerStartFallback: when the probe of the layout of
// runtime.mspan fails (a new Go release), objectStart only proves the
// base of an object, and PanicOnFatal refuses a pointer after a malloc
// header instead of calling runtime.SetFinalizer with a pointer that may
// not be the start of its object.
func TestFinalizerStartFallback(t *testing.T) {
	if !spanLayoutOK {
		t.Skip("the probe fails here: TestRuntimeCouplings reports it")
	}
	small := new([4]*int)
	header := new([300]*int)            // larger than minSizeForMallocHeader: a header
	fallbackSink = []any{small, header} // in the heap
	defer func() { fallbackSink = nil }()
	p0, p1 := uintptr(unsafe.Pointer(small)), uintptr(unsafe.Pointer(header))
	for _, c := range []struct {
		name   string
		l      spanLayout
		p      uintptr
		want   bool
		inHeap bool
	}{
		{"base", spans, p0, true, true},
		{"after header", spans, p1, true, true},
		{"inside", spans, p0 + wordSize, false, true},
		{"base, no layout", spanLayout{}, p0, true, true},
		{"after header, no layout", spanLayout{}, p1, false, true},
		// The offsets of the probe, but not proved (ok false): start must
		// not read the span with them. With the zero offsets above, the
		// words read by mistake are often 0, which hides a missing check.
		{"after header, layout not proved", spanLayout{spans.elemOff, spans.classOff, false}, p1, false, true},
	} {
		if got, in := c.l.start(c.p); got != c.want || in != c.inHeap {
			t.Errorf("%s: start %v inHeap %v, want %v %v", c.name, got, in, c.want, c.inHeap)
		}
	}
	saved := spans
	spans = spanLayout{}
	defer func() { spans = saved }()
	m := &Machine{panicOnFatal: true}
	defer func() {
		fe, ok := recover().(*FatalError)
		if !ok || !strings.Contains(fe.Msg, "pointer not at beginning") {
			t.Errorf("no layout: got %v, want a FatalError", fe)
		}
	}()
	m.checkFinalizerStart(unsafe.Pointer(header), reflect.TypeFor[*int]())
}

var fallbackSink []any

// TestFinalizerNativeFunc: a native func (reflect.TypeOf, json.Marshal,
// a method expression, a method value) is a valid finalizer of a native
// pointer, of a cell of the plugin and of a *byte. It runs, and it does
// not end the host: the runtime calls finalizers on a goroutine where
// nothing recovers. In both modes, with and without PanicOnFatal.
func TestFinalizerNativeFunc(t *testing.T) {
	const want = "ok\nclose 0 <nil>\nencode \"{}\\n\" <nil>\n"
	for _, aot := range []bool{false, true} {
		for _, pof := range []bool{false, true} {
			var goPanics atomic.Int32
			h := loadFatalPlug(t, Options{PanicOnFatal: pof, AOT: aot, GoPanic: func(r any) {
				t.Logf("aot=%v PanicOnFatal=%v: GoPanic %v", aot, pof, r)
				goPanics.Add(1)
			}})
			body, p := serveBG(h, "/finalizernative")
			if p != nil {
				t.Fatalf("aot=%v PanicOnFatal=%v: panic %v", aot, pof, p)
			}
			if body != want {
				t.Errorf("aot=%v PanicOnFatal=%v:\n%s\nwant:\n%s", aot, pof, body, want)
			}
			if n := goPanics.Load(); n != 0 {
				t.Errorf("aot=%v PanicOnFatal=%v: %d panics of plugin goroutines, want none", aot, pof, n)
			}
		}
	}
	if !hostFinalizerRuns() {
		t.Error("a finalizer of the host does not run any more")
	}
}

// TestFinalizerBlockGoexit: the runtime runs all the finalizers of the
// process on one goroutine. A finalizer of the plugin that blocks, or
// that calls runtime.Goexit, must not stop the finalizers of the host.
// A Goexit ends only that finalizer: the next one of the plugin runs.
func TestFinalizerBlockGoexit(t *testing.T) {
	for _, aot := range []bool{false, true} {
		var goPanics atomic.Int32
		h := loadFatalPlug(t, Options{AOT: aot, GoPanic: func(r any) {
			t.Logf("aot=%v: GoPanic %v", aot, r)
			goPanics.Add(1)
		}})
		serve := func(path, want string) {
			t.Helper()
			body, p := serveBG(h, path)
			if p != nil {
				t.Fatalf("aot=%v %s: panic %v", aot, path, p)
			}
			if body != want {
				t.Errorf("aot=%v %s: %q, want %q", aot, path, body, want)
			}
		}
		serve("/finalizerexit", "goexit ok\nnext ok\n")
		if !hostFinalizerRuns() {
			t.Fatalf("aot=%v: a finalizer of the host does not run after a Goexit in a finalizer of the plugin", aot)
		}
		serve("/finalizerblock", "blocked\n")
		ok := hostFinalizerRuns()
		serve("/finalizerrelease", "released\n")
		if !ok {
			t.Fatalf("aot=%v: a finalizer of the host does not run while a finalizer of the plugin blocks", aot)
		}
		if n := goPanics.Load(); n != 0 {
			t.Errorf("aot=%v: %d panics of plugin goroutines, want none", aot, n)
		}
	}
}

// hostFinalizerRuns reports whether a finalizer of the host still runs
// (the finalizer goroutine of the runtime is not stuck), within 5 s.
func hostFinalizerRuns() bool {
	ran := make(chan struct{})
	runtime.SetFinalizer(new([4]*int), func(*[4]*int) { close(ran) })
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		runtime.GC()
		select {
		case <-ran:
			return true
		case <-time.After(10 * time.Millisecond):
		}
	}
	return false
}
