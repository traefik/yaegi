package vm

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

// TestNativeRecoversNames checks that the frames of nativeRecovers are on
// the Go stack of a native handler, for the Go release of the test: the
// names come from the standard library and change with it. In Go 1.27 the
// HTTP/2 server moved to net/http/internal/http2, and
// net/http.(*http2serverConn).runHandler was not found any more: a panic
// of a plugin handler under HTTP/2 went to GoPanic, and the client got a
// 200 instead of the aborted response of go run.
func TestNativeRecoversNames(t *testing.T) {
	// catcher returns the catcher that covers the handler that calls it.
	catcher := func() string {
		pcs := make([]uintptr, 64)
		frames := runtime.CallersFrames(pcs[:runtime.Callers(1, pcs)])
		callee := ""
		for {
			f, more := frames.Next()
			for _, c := range nativeRecovers {
				if f.Function == c.name && c.covers(callee) {
					return c.name
				}
			}
			if !more {
				return ""
			}
			callee = f.Function
		}
	}
	found := make(chan string, 1)
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { found <- catcher() })

	for _, c := range []struct {
		name  string
		serve func()
	}{
		{"HTTP/1", func() {
			srv := httptest.NewServer(h)
			defer srv.Close()
			resp, err := srv.Client().Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
		}},
		{"HTTP/2", func() {
			srv := httptest.NewUnstartedServer(h)
			srv.EnableHTTP2 = true
			srv.StartTLS()
			defer srv.Close()
			resp, err := srv.Client().Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.ProtoMajor != 2 {
				t.Fatalf("proto %s, want HTTP/2", resp.Proto)
			}
		}},
		{"TimeoutHandler", func() {
			th := http.TimeoutHandler(h, 10*time.Second, "timeout")
			th.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		}},
	} {
		c.serve()
		if got := <-found; got == "" {
			t.Errorf("%s: no frame of nativeRecovers covers the handler", c.name)
		} else {
			t.Logf("%s: %s", c.name, got)
		}
	}
}
