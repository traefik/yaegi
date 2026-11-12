package vm

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestHostGlobals checks that a plugin changes only its own copies of
// http.DefaultClient, http.DefaultTransport, http.DefaultServeMux and
// net.DefaultResolver (hostregs.go) and of the default logger of log/slog,
// and not GOMAXPROCS: the host keeps its values, and the plugin uses its
// copies (its insecure transport reaches a TLS server that the host does
// not trust). A new &http.Client{} with a nil Transport uses the plugin's
// DefaultTransport, as in Go, and the plugin's DefaultClient has a nil
// Transport, as in Go. Two plugins run at the same time, one with an
// insecure transport and one without; run it with -race.
func TestHostGlobals(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) }))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0) // the handshakes the secure plugin refuses
	srv.StartTLS()
	defer srv.Close()
	procs := runtime.GOMAXPROCS(0)
	preferGo := net.DefaultResolver.PreferGo

	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostvarsplug"
	handler := func(name string) http.Handler {
		p, err := Load(Options{Dir: ".."}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, name)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	serve := func(h http.Handler, path string) string {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Target", srv.URL)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Body.String()
	}
	hostLogger := slog.Default()
	insecure, secure := handler("insecure"), handler("secure")
	for _, name := range []string{"insecure", "secure"} {
		h := map[string]http.Handler{"insecure": insecure, "secure": secure}[name]
		if got, want := serve(h, "/slog"), "level=INFO msg=new name="+name+"\n"; got != want {
			t.Errorf("%s plugin slog: %q, want %q", name, got, want)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			if got, want := serve(insecure, "/"), "ok ok ok true &{<nil> <nil> <nil> 7s}"; got != want {
				t.Errorf("insecure plugin: %q, want %q", got, want)
			}
			if got, want := serve(secure, "/"), "error error error true &{<nil> <nil> <nil> 7s}"; got != want {
				t.Errorf("secure plugin: %q, want %q", got, want)
			}
			if got, want := serve(insecure, "/evil"), "evil insecure"; got != want {
				t.Errorf("insecure plugin mux: %q, want %q", got, want)
			}
			if got, want := serve(secure, "/evil"), "evil secure"; got != want {
				t.Errorf("secure plugin mux: %q, want %q", got, want)
			}
		})
	}
	wg.Wait()

	if tr := http.DefaultTransport.(*http.Transport); tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("the plugin turned off the TLS verification of the host")
	}
	if _, err := http.Get(srv.URL); err == nil {
		t.Error("the host trusts the TLS server of the test")
	}
	if http.DefaultClient.Timeout != 0 {
		t.Errorf("host http.DefaultClient.Timeout = %v", http.DefaultClient.Timeout)
	}
	for _, path := range []string{"/evil", "/evil2"} {
		if _, pat := http.DefaultServeMux.Handler(httptest.NewRequest("GET", path, nil)); pat != "" {
			t.Errorf("the plugin added %q to the host http.DefaultServeMux", pat)
		}
	}
	if got := runtime.GOMAXPROCS(0); got != procs {
		t.Errorf("GOMAXPROCS = %d, want %d", got, procs)
	}
	if slog.Default() != hostLogger {
		t.Error("the plugin changed the default logger of log/slog of the host")
	}
	if net.DefaultResolver.PreferGo != preferGo {
		t.Error("the plugin changed net.DefaultResolver of the host")
	}
}

// TestHostWrites checks that the writes that name a variable of the
// standard library, and the functions that change the signal handling of
// the process, are refused at load time with their position, and that the
// writes to the plugin's own copies are allowed. The writes that do not
// name the variable are checked at run time (TestHostMem).
func TestHostWrites(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostwriteplug"
	for _, golist := range []bool{false, true} {
		_, err := Load(Options{Dir: "..", GoList: golist}, pkg)
		if err == nil {
			t.Fatalf("go list %v: no error", golist)
		}
		msg := err.Error()
		for _, want := range []string{
			"hostwriteplug.go:21:2: assigning the host variable time.Local is not allowed in a plugin",
			"hostwriteplug.go:22:2: assigning the host variable time.UTC",
			"hostwriteplug.go:23:2: assigning the host variable io.EOF",
			"hostwriteplug.go:24:2: assigning the host variable unicode.Categories",
			"hostwriteplug.go:25:9: changing the host variable unicode.Scripts",
			"hostwriteplug.go:26:8: taking the address of the host variable io.ErrUnexpectedEOF",
			"hostwriteplug.go:28:2: assigning the host variable net/http.ErrAbortHandler",
			"hostwriteplug.go:28:24: assigning the host variable net/http.ErrBodyNotAllowed",
			"hostwriteplug.go:30:2: assigning the host variable os.Stdout",
			"os/signal.Ignore is not allowed",
			"os/signal.Reset is not allowed",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("go list %v: missing %q in\n%s", golist, want, msg)
			}
		}
		if n := strings.Count(msg, "\n") + 1; n != 11 {
			t.Errorf("go list %v: %d errors, want 11 (Allowed and the call result must load):\n%s", golist, n, msg)
		}
	}
	if _, err := Load(Options{Dir: "..", Unrestricted: true}, pkg); err != nil {
		t.Errorf("unrestricted: %v", err)
	}
}

// TestDefaultClient runs testdata/defclient with the standard library of
// plugins (RestrictedStd, as cmd/runtests does) and with the restrictions
// of plugins: http.DefaultClient prints as in Go (a nil Transport), and a
// client with a nil Transport sends its requests through the program's
// http.DefaultTransport, which the program replaces. The host keeps its
// DefaultClient and DefaultTransport.
func TestDefaultClient(t *testing.T) {
	hostTransport, hostClient := http.DefaultTransport, http.DefaultClient
	for _, opts := range []RunOptions{{Dir: "..", RestrictedStd: true}, {Dir: "..", Restricted: true}} {
		if _, err := RunMain(opts, "./vm/testdata/defclient"); err != nil {
			t.Errorf("restricted %v: %v", opts.Restricted, err)
		}
	}
	if http.DefaultTransport != hostTransport {
		t.Error("the program replaced the host http.DefaultTransport")
	}
	if c := http.DefaultClient; c != hostClient || c.Transport != nil || c.Timeout != 0 {
		t.Errorf("the program changed the host http.DefaultClient: %v", c)
	}
}
