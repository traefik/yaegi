package vm_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

// rwHandler loads vm/testdata/rwplug with the wrapper mode, in default or
// AOT mode, in front of the native handler next.
func rwHandler(t *testing.T, aot bool, mode string, next http.Handler) http.Handler {
	t.Helper()
	p, err := vm.Load(vm.Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/rwplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pluginhost.CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetFields(map[string]any{"Mode": mode}); err != nil {
		t.Fatal(err)
	}
	h, err := pluginhost.NewHandler(context.Background(), p, next, cfg, "rw")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func forModes(t *testing.T, f func(t *testing.T, aot bool)) {
	for _, aot := range []bool{false, true} {
		t.Run(fmt.Sprintf("aot=%v", aot), func(t *testing.T) { f(t, aot) })
	}
}

// TestPluginRWFlush checks that a native handler behind a plugin that wraps
// the ResponseWriter can stream: the client reads the first chunk before the
// handler ends (SSE, gRPC streaming), with w.(http.Flusher) and with
// http.ResponseController.
func TestPluginRWFlush(t *testing.T) {
	forModes(t, func(t *testing.T, aot bool) {
		for _, how := range []string{"assert", "controller"} {
			read := make(chan struct{})
			srv := httptest.NewServer(rwHandler(t, aot, "full", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "first\n")
				if how == "assert" {
					f, ok := w.(http.Flusher)
					if !ok {
						t.Errorf("%s: %T is not an http.Flusher", how, w)
						return
					}
					f.Flush()
				} else if err := http.NewResponseController(w).Flush(); err != nil {
					t.Errorf("%s: Flush: %v", how, err)
					return
				}
				select {
				case <-read:
				case <-time.After(5 * time.Second):
					t.Errorf("%s: the client did not get the first chunk before the end of the handler", how)
				}
				io.WriteString(w, "second\n")
			})))
			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			br := bufio.NewReader(resp.Body)
			if line, err := br.ReadString('\n'); line != "first\n" {
				t.Errorf("%s: first chunk %q, %v", how, line, err)
			}
			close(read)
			if rest, _ := io.ReadAll(br); string(rest) != "second\n" {
				t.Errorf("%s: rest %q", how, rest)
			}
			resp.Body.Close()
			srv.Close()
		}
	})
}

// TestPluginRWHijack checks a websocket-style upgrade through a plugin that
// wraps the ResponseWriter: httputil.ReverseProxy hijacks the connection
// (http.ResponseController) and the backend hijacks it with
// w.(http.Hijacker).
func TestPluginRWHijack(t *testing.T) {
	forModes(t, func(t *testing.T, aot bool) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Errorf("backend: %T is not an http.Hijacker", w)
				return
			}
			conn, brw, err := hj.Hijack()
			if err != nil {
				t.Errorf("backend: Hijack: %v", err)
				return
			}
			defer conn.Close()
			brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
			brw.Flush()
			line, _ := brw.ReadString('\n')
			brw.WriteString("echo " + line)
			brw.Flush()
		}))
		defer backend.Close()
		u, _ := url.Parse(backend.URL)
		for _, name := range []string{"proxy", "direct"} {
			var next http.Handler = httputil.NewSingleHostReverseProxy(u)
			if name == "direct" {
				next = backend.Config.Handler
			}
			srv := httptest.NewServer(rwHandler(t, aot, "full", next))
			conn, err := net.Dial("tcp", srv.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			io.WriteString(conn, "GET / HTTP/1.1\r\nHost: x\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
			br := bufio.NewReader(conn)
			resp, err := http.ReadResponse(br, nil)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if resp.StatusCode != http.StatusSwitchingProtocols {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("%s: status %d (%s), want 101", name, resp.StatusCode, strings.TrimSpace(string(body)))
			} else {
				io.WriteString(conn, "ping\n")
				if line, err := br.ReadString('\n'); line != "echo ping\n" {
					t.Errorf("%s: after the upgrade: %q, %v", name, line, err)
				}
			}
			conn.Close()
			srv.Close()
		}
	})
}

// TestPluginRWOptionalIfaces checks what native code sees of the wrapper of
// each plugin type: exactly the optional interfaces that the type has in Go.
func TestPluginRWOptionalIfaces(t *testing.T) {
	type seen struct {
		flusher, hijacker, unwrap bool
		flushErr, deadlineErr     string
		header                    string
	}
	tests := []struct {
		mode string
		want seen
	}{
		{"full", seen{flusher: true, hijacker: true, unwrap: true, flushErr: "<nil>", deadlineErr: "<nil>"}},
		// only Write: no optional method, as in Go
		{"plain", seen{flushErr: "not supported", deadlineErr: "not supported"}},
		// http.ResponseController goes through Unwrap
		{"unwrap", seen{unwrap: true, flushErr: "<nil>", deadlineErr: "<nil>"}},
		// Flush() error is not http.Flusher
		{"badflush", seen{flushErr: "not supported", deadlineErr: "not supported"}},
		// SetWriteDeadline of the plugin, nothing for Flush
		{"deadline", seen{flushErr: "not supported", deadlineErr: "<nil>", header: "set"}},
	}
	errText := func(err error) string {
		if errors.Is(err, http.ErrNotSupported) {
			return "not supported"
		}
		return fmt.Sprint(err)
	}
	forModes(t, func(t *testing.T, aot bool) {
		for _, tt := range tests {
			var got seen
			h := rwHandler(t, aot, tt.mode, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, got.flusher = w.(http.Flusher)
				_, got.hijacker = w.(http.Hijacker)
				_, got.unwrap = w.(interface{ Unwrap() http.ResponseWriter })
				rc := http.NewResponseController(w)
				got.deadlineErr = errText(rc.SetWriteDeadline(time.Now().Add(time.Minute)))
				got.flushErr = errText(rc.Flush())
				got.header = w.Header().Get("X-Deadline")
			}))
			srv := httptest.NewServer(h)
			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			srv.Close()
			if got != tt.want {
				t.Errorf("%s: native handler sees %+v, want %+v", tt.mode, got, tt.want)
			}
		}
	})
}

// rwIfaces lists the optional interfaces that native code finds on w with a
// type assertion, the methods of http.ResponseController included.
func rwIfaces(w http.ResponseWriter) string {
	var s []string
	if _, ok := w.(http.Flusher); ok {
		s = append(s, "Flusher")
	}
	if _, ok := w.(http.Hijacker); ok {
		s = append(s, "Hijacker")
	}
	if _, ok := w.(io.ReaderFrom); ok {
		s = append(s, "ReaderFrom")
	}
	if _, ok := w.(http.Pusher); ok {
		s = append(s, "Pusher")
	}
	//lint:ignore SA1019 yaegi v2 still forwards CloseNotify of plugin types, as Go does
	if _, ok := w.(http.CloseNotifier); ok {
		s = append(s, "CloseNotifier")
	}
	if _, ok := w.(interface{ Unwrap() http.ResponseWriter }); ok {
		s = append(s, "Unwrap")
	}
	if _, ok := w.(interface{ FlushError() error }); ok {
		s = append(s, "FlushError")
	}
	if _, ok := w.(interface{ SetReadDeadline(time.Time) error }); ok {
		s = append(s, "SetReadDeadline")
	}
	if _, ok := w.(interface{ SetWriteDeadline(time.Time) error }); ok {
		s = append(s, "SetWriteDeadline")
	}
	if _, ok := w.(interface{ EnableFullDuplex() error }); ok {
		s = append(s, "EnableFullDuplex")
	}
	return strings.Join(s, ",")
}

// TestPluginRWFeatures checks each optional method of a plugin
// ResponseWriter alone: native code sees exactly the interfaces of the type,
// and the call reaches the method of the plugin, with its arguments and
// results converted.
func TestPluginRWFeatures(t *testing.T) {
	errText := func(err error) string {
		if errors.Is(err, http.ErrNotSupported) {
			return "not supported"
		}
		return fmt.Sprint(err)
	}
	tests := []struct {
		mode  string
		check func(w http.ResponseWriter) string
		want  string
		body  string
	}{
		{
			mode: "readfrom",
			check: func(w http.ResponseWriter) string {
				n, err := w.(io.ReaderFrom).ReadFrom(strings.NewReader("abc"))
				return fmt.Sprintf("%d %v %s", n, err, w.Header().Get("X-Via"))
			},
			want: "ReaderFrom|3 <nil> readfrom",
			body: "abc",
		},
		{
			mode: "push",
			check: func(w http.ResponseWriter) string {
				p := w.(http.Pusher)
				return fmt.Sprintf("%v; %v", p.Push("/a", nil), p.Push("/b", &http.PushOptions{Method: "HEAD"}))
			},
			want: "Pusher|pushed /a; pushed /b HEAD",
		},
		{
			// FlushError is used by http.ResponseController, but the type is
			// not a Flusher.
			mode: "flusherror",
			check: func(w http.ResponseWriter) string {
				return errText(http.NewResponseController(w).Flush())
			},
			want: "FlushError|plugin flusherror",
		},
		{
			mode: "readdeadline",
			check: func(w http.ResponseWriter) string {
				rc := http.NewResponseController(w)
				return fmt.Sprintf("%s %s %s %s", errText(rc.SetReadDeadline(time.Now().Add(time.Minute))),
					w.Header().Get("X-Read-Deadline"), errText(rc.Flush()), errText(rc.EnableFullDuplex()))
			},
			want: "SetReadDeadline|<nil> set not supported not supported",
		},
		{
			mode: "fullduplex",
			check: func(w http.ResponseWriter) string {
				rc := http.NewResponseController(w)
				return fmt.Sprintf("%s %s", errText(rc.EnableFullDuplex()), errText(rc.SetWriteDeadline(time.Now().Add(time.Minute))))
			},
			want: "EnableFullDuplex|plugin fullduplex not supported",
		},
		{
			// ReadFrom and FlushError only: the wrapper has the four control
			// methods (rwComboMask, the one exception), but the three that
			// the type does not have give the results of Go through
			// http.ResponseController: not supported, without Unwrap.
			mode: "rarectl",
			check: func(w http.ResponseWriter) string {
				rc := http.NewResponseController(w)
				n, err := w.(io.ReaderFrom).ReadFrom(strings.NewReader("abc"))
				return fmt.Sprintf("%d %v %s %s %s %s %s", n, err, w.Header().Get("X-Via"), errText(rc.Flush()),
					errText(rc.SetReadDeadline(time.Now().Add(time.Minute))),
					errText(rc.SetWriteDeadline(time.Now().Add(time.Minute))), errText(rc.EnableFullDuplex()))
			},
			want: "ReaderFrom,FlushError,SetReadDeadline,SetWriteDeadline,EnableFullDuplex|3 <nil> readfrom plugin flusherror not supported not supported not supported",
			body: "abc",
		},
		{
			// The same with Unwrap: http.ResponseController reaches the
			// writer of the server for the missing control methods.
			mode: "rarectlunwrap",
			check: func(w http.ResponseWriter) string {
				rc := http.NewResponseController(w)
				return fmt.Sprintf("%s %s %s %s", errText(rc.Flush()),
					errText(rc.SetReadDeadline(time.Now().Add(time.Minute))),
					errText(rc.SetWriteDeadline(time.Now().Add(time.Minute))), errText(rc.EnableFullDuplex()))
			},
			want: "ReaderFrom,Unwrap,FlushError,SetReadDeadline,SetWriteDeadline,EnableFullDuplex|plugin flusherror <nil> <nil> <nil>",
		},
		{
			// Flush has a pointer receiver and the plugin passes a value: not
			// a Flusher, as in Go.
			mode: "valflush",
			check: func(w http.ResponseWriter) string {
				return errText(http.NewResponseController(w).Flush()) + " " + w.Header().Get("X-Flush")
			},
			want: "|not supported ",
		},
		{
			mode: "ptrflush",
			check: func(w http.ResponseWriter) string {
				return errText(http.NewResponseController(w).Flush()) + " " + w.Header().Get("X-Flush")
			},
			want: "Flusher|<nil> plugin",
		},
	}
	forModes(t, func(t *testing.T, aot bool) {
		for _, tt := range tests {
			var got string
			h := rwHandler(t, aot, tt.mode, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = rwIfaces(w) + "|" + tt.check(w)
			}))
			srv := httptest.NewServer(h)
			resp, err := http.Get(srv.URL)
			if err != nil {
				srv.Close()
				t.Errorf("%s: %v", tt.mode, err)
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			srv.Close()
			if got != tt.want {
				t.Errorf("%s: native handler sees %q, want %q", tt.mode, got, tt.want)
			}
			if string(body) != tt.body {
				t.Errorf("%s: body %q, want %q", tt.mode, body, tt.body)
			}
		}
	})
}

// TestPluginRWCloseNotify checks http.CloseNotifier on a plugin
// ResponseWriter: the native handler gets the channel that the plugin
// returns (here the one of the server), and it fires when the client goes
// away.
func TestPluginRWCloseNotify(t *testing.T) {
	forModes(t, func(t *testing.T, aot bool) {
		started := make(chan struct{})
		got := make(chan string, 1)
		h := rwHandler(t, aot, "closenotify", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//lint:ignore SA1019 yaegi v2 still forwards CloseNotify of plugin types, as Go does
			cn, ok := w.(http.CloseNotifier)
			if !ok {
				got <- rwIfaces(w) + "|not a CloseNotifier"
				close(started)
				return
			}
			ch := cn.CloseNotify()
			via := w.Header().Get("X-Close-Notify")
			close(started)
			select {
			case v := <-ch:
				got <- fmt.Sprintf("%s|%s %v", rwIfaces(w), via, v)
			case <-time.After(10 * time.Second):
				got <- "no close notification"
			}
		}))
		srv := httptest.NewServer(h)
		defer srv.Close()
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
		go func() {
			<-started
			cancel()
		}()
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
		if s, want := <-got, "CloseNotifier|plugin true"; s != want {
			t.Errorf("native handler sees %q, want %q", s, want)
		}
	})
}

// TestPluginRWFlushPanic checks that a Flush of the plugin that panics, or
// that runs out of budget, cannot end the host when native code calls it on
// a goroutine of its own: httputil.ReverseProxy with a FlushInterval
// (Traefik: 100ms by default) flushes from a timer goroutine
// (maxLatencyWriter.delayedFlush). The panic goes to Options.GoPanic and the
// response ends normally. Each case runs in a child process (this test
// binary), so that a crash is seen.
func TestPluginRWFlushPanic(t *testing.T) {
	if c := os.Getenv("VM_RWFLUSH_CHILD"); c != "" {
		mode, plugMode, _ := strings.Cut(c, " ")
		got := make(chan any, 1)
		p, err := vm.Load(vm.Options{
			Dir:    "..",
			AOT:    mode == "aot",
			Budget: 10_000_000,
			GoPanic: func(r any) {
				select {
				case got <- r: // the first one; a later flush can panic again
				default:
				}
			},
		}, "github.com/traefik/yaegi/v2/vm/testdata/rwplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := pluginhost.CreateConfig(p)
		if err := cfg.SetFields(map[string]any{"Mode": plugMode}); err != nil {
			t.Fatal(err)
		}
		flushed := make(chan struct{})
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "13")
			io.WriteString(w, "first\n")
			w.(http.Flusher).Flush()
			select {
			case <-flushed:
			case <-time.After(10 * time.Second):
			}
			io.WriteString(w, "second\n")
		}))
		defer backend.Close()
		u, _ := url.Parse(backend.URL)
		proxy := httputil.NewSingleHostReverseProxy(u)
		proxy.FlushInterval = 20 * time.Millisecond
		h, err := pluginhost.NewHandler(context.Background(), p, proxy, cfg, "rw")
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(h)
		defer srv.Close()
		type result struct {
			body string
			err  error
		}
		done := make(chan result, 1)
		go func() {
			resp, err := http.Get(srv.URL)
			if err != nil {
				done <- result{err: err}
				return
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			done <- result{string(b), err}
		}()
		var r any
		select {
		case r = <-got:
		case <-time.After(30 * time.Second):
			t.Fatal("the plugin Flush was not called")
		}
		close(flushed)
		res := <-done
		fmt.Printf("body %q %v\ngopanic %v\nhost still alive\n", res.body, res.err, r)
		return
	}
	for _, c := range []struct{ mode, panic string }{
		{"flushpanic", "panic: flush boom"},
		{"flushloop", vm.ErrBudget.Error()},
	} {
		for _, mode := range []string{"default", "aot"} {
			cmd := exec.Command(os.Args[0], "-test.run=^TestPluginRWFlushPanic$", "-test.v")
			cmd.Env = append(os.Environ(), "VM_RWFLUSH_CHILD="+mode+" "+c.mode)
			out, err := cmd.CombinedOutput()
			want := fmt.Sprintf("body %q <nil>\ngopanic %v\nhost still alive\n", "first\nsecond\n", c.panic)
			if err != nil || !strings.Contains(string(out), want) {
				t.Errorf("%s %s: child process: %v, output:\n%s\nwant:\n%s", mode, c.mode, err, out, want)
			}
		}
	}
}
