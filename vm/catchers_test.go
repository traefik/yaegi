package vm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

// The probes of the stack-walk heuristics of nativego.go.
// underCatcher, below and afterPanic decide what to do with a panic by
// matching the names of functions of the standard library and of the
// runtime on the Go stack (nativeRecovers, deferFrames, noGoexit). A new Go
// release can rename these functions, or inline them in a way that changes
// the frames: the heuristics then give the wrong answer without an error.
// These probes run the real code paths of the standard library (an HTTP/1
// and an HTTP/2 server, http.TimeoutHandler, a finalizer, a cleanup, the
// deferred calls of a panic, of runtime.Goexit and of a normal return) and
// ask the heuristics, from inside, what they see. They run in every test
// run, so also in the "next Go" job of CI.

// probeCatcher reports what underCatcher says for a panic of the calls
// below it.
//
//go:noinline
func probeCatcher() bool { return underCatcher(probeCatcherName) }

// probeBelow reports whether a function of names is on the stack below it.
//
//go:noinline
func probeBelow(names []string) bool { return below(probeBelowName, names) }

var probeCatcherName, probeBelowName string

func init() { probeCatcherName, probeBelowName = goFuncName(probeCatcher), goFuncName(probeBelow) }

// TestCatcherProbes: the frames that nativeRecovers lists recover the panics
// of the handlers of the HTTP/1 and HTTP/2 servers and of
// http.TimeoutHandler, and the frames of deferFrames and noGoexit are on the
// stack where afterPanic and underCatcher expect them.
func TestCatcherProbes(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	record := func(name string, ok bool) {
		mu.Lock()
		seen[name] = ok
		mu.Unlock()
	}
	handler := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record(name, probeCatcher())
			if r.Body != nil {
				io.Copy(io.Discard, r.Body)
			}
		})
	}

	// HTTP/1: (*conn).serve covers serverHandler.ServeHTTP.
	srv := httptest.NewServer(handler("http1"))
	probeGet(t, srv.Client(), srv.URL)
	srv.Close()

	// http.TimeoutHandler: its ServeHTTP.func1 covers the handler.
	srv = httptest.NewServer(http.TimeoutHandler(handler("timeout"), time.Minute, ""))
	probeGet(t, srv.Client(), srv.URL)
	srv.Close()

	// HTTP/2: runHandler covers the handler.
	srv = httptest.NewUnstartedServer(handler("http2"))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	c := srv.Client()
	c.Transport.(*http.Transport).ForceAttemptHTTP2 = true
	c.Transport.(*http.Transport).TLSClientConfig.NextProtos = []string{"h2"}
	if resp := probeGet(t, c, srv.URL); resp != nil && resp.ProtoMajor != 2 {
		t.Errorf("the HTTP/2 probe got %s", resp.Proto)
	}
	srv.Close()

	for _, name := range []string{"http1", "timeout", "http2"} {
		if ok, ran := seen[name]; !ran || !ok {
			t.Errorf("%s: underCatcher sees no catcher (ran %v): a frame of nativeRecovers (nativego.go) is gone or renamed", name, ran)
		}
	}
	// A goroutine of the program (no catcher) must not see one.
	done := make(chan bool)
	go func() { done <- probeCatcher() }()
	if <-done {
		t.Error("underCatcher sees a catcher on a plain goroutine")
	}

	// deferFrames: the deferred calls of a panic, of runtime.Goexit and of a
	// normal return with a defer that is not open-coded.
	func() {
		defer func() {
			if !probeBelow(deferFrames) {
				t.Error("a deferred call during a panic: no frame of deferFrames below it")
			}
			recover()
		}()
		panic("probe")
	}()
	go func() {
		defer close(done)
		defer func() {
			if !probeBelow(deferFrames) {
				t.Error("a deferred call during runtime.Goexit: no frame of deferFrames below it")
			}
		}()
		runtime.Goexit()
	}()
	<-done
	// At a normal return, a deferred call is called by its function, with
	// no frame of the runtime between (open-coded or not: runtime.deferreturn
	// is not on the stack with Go 1.26). So the covers func of a catcher must
	// leave out its deferred closures by name (calleeNotIn), also the ones of
	// a closure, which are named outer.1, not outer.func1, as in
	// (*timeoutHandler).ServeHTTP.func1.
	var outer, deferred, caller string
	func() {
		outer = callerName(0)
		for i := 0; i < 1; i++ {
			defer func() { deferred, caller = callerName(0), callerName(1) }() // in a loop: not open-coded
		}
		defer func() {}()
	}()
	if caller != outer {
		t.Errorf("the deferred call %s at a normal return is called by %s, not by its function %s", deferred, caller, outer)
	}
	if calleeNotIn(outer)(deferred) {
		t.Errorf("calleeNotIn(%q) covers its own deferred closure %s", outer, deferred)
	}

	// noGoexit: the finalizer and the cleanup goroutines of the runtime.
	fin, clean := make(chan bool, 1), make(chan bool, 1)
	func() {
		p := new([64]byte)
		runtime.SetFinalizer(p, func(*[64]byte) { fin <- probeBelow(noGoexit) })
		q := new([64]byte)
		runtime.AddCleanup(q, func(int) { clean <- probeBelow(noGoexit) }, 0)
	}()
	for name, ch := range map[string]chan bool{"finalizer": fin, "cleanup": clean} {
		var ok, ran bool
		for i := 0; i < 50 && !ran; i++ {
			runtime.GC()
			select {
			case ok = <-ch:
				ran = true
			case <-time.After(20 * time.Millisecond):
			}
		}
		if !ran || !ok {
			t.Errorf("%s: no frame of noGoexit below it (ran %v)", name, ran)
		}
	}

	// Each catcher of nativeRecovers has a probe above, except the names of
	// another Go release (the HTTP/2 server moved in Go 1.27).
	probed := []string{
		"net/http.(*conn).serve",
		"net/http.(*timeoutHandler).ServeHTTP.func1",
		"net/http.(*http2serverConn).runHandler",
		"net/http/internal/http2.(*serverConn).runHandler",
	}
	for _, c := range nativeRecovers {
		if !slices.Contains(probed, c.name) {
			t.Errorf("no probe for the catcher %s: add one to TestCatcherProbes", c.name)
		}
	}
}

// callerName returns the name of the function skip frames above its
// caller (0: the caller).
func callerName(skip int) string {
	pc, _, _, _ := runtime.Caller(skip + 1)
	return runtime.FuncForPC(pc).Name()
}

func probeGet(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Error(err)
		return nil
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}
