package vm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// xxhashFiles is a vendored package with the layout of cespare/xxhash v2:
// the file with the {string, cap} header idiom is guarded by !appengine only,
// so the purego tag the loader sets does not remove it, and a plugin with
// useUnsafe runs it.
var xxhashFiles = map[string]string{
	"go.mod":             "module example.com/plug\n\ngo 1.22\n\nrequire example.com/xxh v1.0.0\n",
	"vendor/modules.txt": "# example.com/xxh v1.0.0\n## explicit; go 1.11\nexample.com/xxh\n",
	"vendor/example.com/xxh/xxh.go": `package xxh

// Digest keeps the bytes it was given in a fixed buffer, as xxhash does.
type Digest struct {
	mem [32]byte
	n   int
	h   uint64
}

func New() *Digest { return &Digest{h: 14695981039346656037} }

func (d *Digest) Write(b []byte) (int, error) {
	for len(b) > 0 {
		c := copy(d.mem[d.n:], b)
		d.n += c
		b = b[c:]
		if d.n == len(d.mem) {
			d.flush()
		}
	}
	return 0, nil
}

func (d *Digest) flush() {
	for _, c := range d.mem[:d.n] {
		d.h ^= uint64(c)
		d.h *= 1099511628211
	}
	d.n = 0
}

func (d *Digest) Sum64() uint64 {
	d.flush()
	return d.h
}

func Sum64(b []byte) uint64 {
	d := New()
	d.Write(b)
	return d.Sum64()
}
`,
	"vendor/example.com/xxh/xxh_safe.go": `//go:build appengine

package xxh

func Sum64String(s string) uint64 { return Sum64([]byte(s)) }

func (d *Digest) WriteString(s string) (int, error) { return d.Write([]byte(s)) }
`,
	"vendor/example.com/xxh/xxh_unsafe.go": `//go:build !appengine

package xxh

import "unsafe"

func Sum64String(s string) uint64 {
	b := *(*[]byte)(unsafe.Pointer(&sliceHeader{s, len(s)}))
	return Sum64(b)
}

func (d *Digest) WriteString(s string) (n int, err error) {
	d.Write(*(*[]byte)(unsafe.Pointer(&sliceHeader{s, len(s)})))
	return len(s), nil
}

type sliceHeader struct {
	s   string
	cap int
}
`,
	"plug.go": `package plug

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"example.com/xxh"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bad := 0
		long := strings.Repeat("0123456789abcdef", 5)
		for _, base := range []string{r.URL.Path, r.Method, http.StatusText(404), long, r.URL.Path + long} {
			for n := 0; n <= len(base); n++ {
				s := base[n:]
				if xxh.Sum64String(s) != xxh.Sum64([]byte(s)) {
					bad++
				}
				d := xxh.New()
				d.WriteString(s)
				d.WriteString(r.URL.Path)
				if d.Sum64() != xxh.Sum64([]byte(s+r.URL.Path)) {
					bad++
				}
			}
		}
		fmt.Fprint(w, "bad ", bad)
	}), nil
}
`,
}

// TestXXHashUnsafe checks the unsafe file of cespare/xxhash v2 in a plugin
// with Options.UseUnsafe, on concurrent requests, in VM and AOT modes: it
// panicked at request time before the {string, cap} header idiom was
// supported (testdata/unsafeidioms.go has the idiom alone).
func TestXXHashUnsafe(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := writeModule(t, xxhashFiles)
	for _, aot := range []bool{false, true} {
		p, err := Load(Options{Dir: dir, UseUnsafe: true, AOT: aot}, "example.com/plug")
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
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				for j := range 5 {
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, httptest.NewRequest("GET", fmt.Sprintf("/p%d/%d", i, j), nil))
					if got := rec.Body.String(); got != "bad 0" {
						t.Errorf("aot=%v: status %d, body %q, want \"bad 0\"", aot, rec.Code, got)
					}
				}
			})
		}
		wg.Wait()
	}
	// Without UseUnsafe, the plugin is refused at load time, not at
	// request time.
	if _, err := Load(Options{Dir: dir}, "example.com/plug"); err == nil {
		t.Error("loaded without UseUnsafe")
	}
}
