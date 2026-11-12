package vm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// chainOuterSrc wraps the ResponseWriter with a type that has its own
// WriteHeader, Write, Flush and Hijack, as compression or cache plugins do.
const chainOuterSrc = `package p

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type rw struct{ http.ResponseWriter }

func (w rw) WriteHeader(code int) {
	w.ResponseWriter.Header().Set("X-A", "1")
	w.ResponseWriter.WriteHeader(code)
}

func (w rw) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(append([]byte("A:"), b...))
}

func (w rw) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w rw) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("A: not a hijacker")
}

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.next.ServeHTTP(rw{w}, r)
}
`

// chainMiddleSrc wraps the ResponseWriter with a type that has only Write
// and Unwrap: it is not a Flusher nor a Hijacker, but
// http.ResponseController finds them behind Unwrap.
const chainMiddleSrc = `package p

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type rw struct{ http.ResponseWriter }

func (w rw) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(append([]byte("B:"), b...))
}

func (w rw) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type handler struct{ next http.Handler }

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{next}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.next.ServeHTTP(rw{w}, r)
}
`

// chainCheckLeafSrc asserts Flusher and Hijacker on the writer it gets,
// then flushes and hijacks it, directly or with http.ResponseController.
const chainCheckLeafSrc = `package p

import (
	"context"
	"fmt"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return handler{}, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, isF := w.(http.Flusher)
	hj, isH := w.(http.Hijacker)
	w.Header().Set("X-Leaf", fmt.Sprintf("flusher=%v hijacker=%v", isF, isH))
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("leaf"))
	rc := http.NewResponseController(w)
	if isF {
		f.Flush()
	} else if err := rc.Flush(); err != nil {
		w.Write([]byte(" flush:" + err.Error()))
	}
	var err error
	if isH {
		_, _, err = hj.Hijack()
	} else {
		_, _, err = rc.Hijack()
	}
	w.Write([]byte(" hijack:" + fmt.Sprint(err)))
}
`

// hostRW is the ResponseWriter of the host: a Flusher (from
// httptest.ResponseRecorder) and a Hijacker.
type hostRW struct {
	*httptest.ResponseRecorder
	hijacks int
}

func (w *hostRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacks++
	return nil, nil, errors.New("host hijack")
}

// TestChainedPluginsWrappedWriterMethods chains plugins loaded with
// separate Loads (one machine each, as Traefik does). Each machine gets a
// wrapper of a value of the machine before it, with the same type name
// (example.com/p.rw). When the wrapper had Flush or Hijack (the
// ResponseWriter wrapper of rwwriter.go), ifaceVMs unwrapped it as a value
// of the next machine: each request panicked with "native type
// example.com/p.rw not in registry". It checks the methods of the wrappers
// (WriteHeader, Write, Flush, Hijack), the Flusher and Hijacker assertions,
// http.ResponseController through Unwrap, a chain of three plugins, and
// concurrent requests (run it with -race).
func TestChainedPluginsWrappedWriterMethods(t *testing.T) {
	needSource(t)
	for _, aot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vm", true: "aot"}[aot], func(t *testing.T) {
			opts := Options{AOT: aot}
			t.Run("two", func(t *testing.T) {
				// the leaf gets the rw of the outer plugin: Flusher and Hijacker
				h := loadChain(t, opts, chainOuterSrc, chainCheckLeafSrc)
				checkChain(t, h, "flusher=true hijacker=true", "A:leafA: hijack:host hijack")
			})
			t.Run("three", func(t *testing.T) {
				// the leaf gets the rw of the middle plugin: no Flusher, no
				// Hijacker; http.ResponseController finds them behind Unwrap
				h := loadChain(t, opts, chainOuterSrc, chainMiddleSrc, chainCheckLeafSrc)
				checkChain(t, h, "flusher=false hijacker=false", "A:B:leafA:B: hijack:host hijack")
			})
		})
	}
}

// loadChain loads each source with its own Load and chains them: the first
// source is the outer handler, the last one is the leaf.
func loadChain(t *testing.T, opts Options, srcs ...string) http.Handler {
	t.Helper()
	ctx := context.Background()
	var next http.Handler = http.NotFoundHandler()
	for i := len(srcs) - 1; i >= 0; i-- {
		dir, pkgPath := writePlugin(t, srcs[i])
		o := opts
		o.Dir = dir
		p, err := Load(o, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		next, err = newHandler(ctx, p, next, cfg, fmt.Sprint("chain", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	return next
}

func checkChain(t *testing.T, h http.Handler, wantLeaf, wantBody string) {
	t.Helper()
	const workers, reqs = 4, 5
	errs := make(chan error, workers*reqs)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqs {
				errs <- serveChain(h, wantLeaf, wantBody)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func serveChain(h http.Handler, wantLeaf, wantBody string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	w := &hostRW{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	switch {
	case w.Code != http.StatusCreated:
		return fmt.Errorf("code %d, want %d", w.Code, http.StatusCreated)
	case w.Header().Get("X-A") != "1":
		return fmt.Errorf("X-A header %q, want 1 (WriteHeader of the outer rw)", w.Header().Get("X-A"))
	case w.Header().Get("X-Leaf") != wantLeaf:
		return fmt.Errorf("X-Leaf header %q, want %q", w.Header().Get("X-Leaf"), wantLeaf)
	case w.Body.String() != wantBody:
		return fmt.Errorf("body %q, want %q", w.Body.String(), wantBody)
	case !w.Flushed:
		return errors.New("the host writer was not flushed")
	case w.hijacks != 1:
		return fmt.Errorf("%d hijacks of the host writer, want 1", w.hijacks)
	}
	return nil
}
