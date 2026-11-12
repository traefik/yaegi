package vm

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
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
