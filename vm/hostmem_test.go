package vm

import (
	"bytes"
	"context"
	"encoding/asn1"
	"encoding/base64"
	"go/doc"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
	"unicode"
	"unsafe"
)

// TestHostMem checks the run-time check of hostmem.go: a restricted plugin
// (default options and Sandbox) that writes to host objects reached from
// standard library variables, without naming the variable, gets a
// recoverable panic, and the host keeps its objects. The plugin's own
// objects stay writable and io.EOF keeps its identity. The elements of the
// host []byte variables (net.IPv4bcast...) are checked too. The known limit
// (writes made by native code) is pinned: when a fix removes it, this test
// must change. It runs in a subprocess, because that case changes the host.
func TestHostMem(t *testing.T) {
	if os.Getenv("YAEGI_HOSTMEM_CHILD") == "1" {
		hostMemChild(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHostMem$", "-test.count=1")
	cmd.Env = append(os.Environ(), "YAEGI_HOSTMEM_CHILD=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil || childStdout(stdout.String()) != "PASS\n" {
		t.Fatalf("subprocess: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
}

func hostMemChild(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostmemplug"
	const want = `location: refused
local copy: refused
utc copy: refused
field: refused
map delete: refused
map update: refused
map clear: refused
field address: refused
slice element: refused
slice append: refused
slice copy: refused
reflect encoding: refused
reflect eof: refused
reflect field: refused
reflect map: refused
bytes element: refused
bytes pointer: refused
bytes append: refused
bytes append slice: refused
bytes copy: refused
bytes copy string: refused
bytes clear: refused
bytes reflect copy: refused
bytes reflect clear: refused
bytes reflect append overflow: done
bytes asn1: refused
bytes reflect index: refused
bytes reflect elem: refused
bytes reflect swapper: refused
bytes sort: refused
bytes own: done
stdout object: done
stderr object: done
own: done
hole json: done
`
	eof := io.EOF
	var hostBytes0 [][]byte
	for _, b := range hostBytes() {
		hostBytes0 = append(hostBytes0, bytes.Clone(b))
	}
	scripts := len(unicode.Scripts)
	prefix := doc.IllegalPrefixes[0]
	// Not 0, 1, 2: on Windows the descriptors are handles.
	stdin, stdout, stderr := os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()
	for _, opts := range []Options{{Dir: ".."}, {Dir: "..", Sandbox: &Sandbox{}}} {
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if got := rec.Body.String(); got != want {
			t.Errorf("sandbox %v: plugin output:\n%s\nwant:\n%s", opts.Sandbox != nil, got, want)
		}

		if got := time.Local.String(); got != "Local" {
			t.Errorf("host time.Local is %q", got)
		}
		if got := time.Now().Location().String(); got != "Local" {
			t.Errorf("host time.Now().Location() is %q", got)
		}
		if got := time.UTC.String(); got != "UTC" {
			t.Errorf("host time.UTC is %q", got)
		}
		if unicode.Upper.LatinOffset == 99 {
			t.Error("host unicode.Upper changed")
		}
		if unicode.Categories["Lu"] == nil || len(unicode.Scripts) != scripts || unicode.Scripts["Latin"] == nil {
			t.Error("host unicode tables changed")
		}
		if got := base64.StdEncoding.EncodeToString([]byte{0xfb, 0xff}); got != "+/8=" {
			t.Errorf("host base64.StdEncoding encodes %q", got)
		}
		if io.EOF != eof || io.EOF.Error() != "EOF" {
			t.Errorf("host io.EOF changed: %v", io.EOF)
		}
		for i, b := range hostBytes() {
			if string(b) != string(hostBytes0[i]) {
				t.Errorf("host []byte %d changed: %v, want %v", i, b, hostBytes0[i])
			}
		}
		if doc.IllegalPrefixes[0] != prefix {
			t.Errorf("host doc.IllegalPrefixes[0] is %q", doc.IllegalPrefixes[0])
		}
		// A changed *os.File makes these panic (nil file) or return
		// another descriptor.
		if os.Stdout.Fd() != stdout || os.Stderr.Fd() != stderr || os.Stdin.Fd() != stdin {
			t.Errorf("host stdio files changed: %d %d %d, want %d %d %d", os.Stdout.Fd(), os.Stderr.Fd(), os.Stdin.Fd(), stdout, stderr, stdin)
		}

		// The known limit changes the host. Undo it for the next round.
		if got := http.ErrNotSupported.ErrorString; got != "pwned" {
			t.Errorf("limit fixed? host http.ErrNotSupported is %q: update hostmem.go and this test", got)
		}
		http.ErrNotSupported.ErrorString = "feature not supported"
	}
	hostMemAOT(t)
}

// hostBytes are the []byte variables of the standard library that
// hostmemplug writes to.
func hostBytes() [][]byte {
	return [][]byte{
		net.IPv4bcast, net.IPv4allsys, net.IPv4allrouter, net.IPv4zero,
		net.IPv6zero, net.IPv6loopback, net.IPv6linklocalallnodes,
		net.IPv6linklocalallrouters, net.IPv6interfacelocalallnodes,
		asn1.NullBytes,
	}
}

// hostMemAOT checks the globals of the AOT copies (aot/all, linked by
// aotstate_test.go): they are shared by all the plugins of the process, so
// the writes of one plugin are refused, and a second plugin sees the
// original values.
func hostMemAOT(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/hostmemaotplug"
	serve := func(opts Options, path string) string {
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if p.M.table("encoding/xml") != aotPkg("encoding/xml") || p.M.table("database/sql") != aotPkg("database/sql") || aotPkg("encoding/xml") == nil {
			t.Fatal("the plugin does not use the AOT copies")
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Body.String()
	}
	for _, opts := range []Options{{Dir: "..", AOT: true}, {Dir: "..", AOT: true, Sandbox: &Sandbox{}}} {
		if got, want := serve(opts, "/write"), "xml map: refused\nsql error: refused\n"; got != want {
			t.Errorf("sandbox %v: first plugin:\n%s\nwant:\n%s", opts.Sandbox != nil, got, want)
		}
		if got, want := serve(opts, "/read"), "entity: false\nerr: sql: no rows in result set\n"; got != want {
			t.Errorf("sandbox %v: second plugin:\n%s\nwant:\n%s", opts.Sandbox != nil, got, want)
		}
	}
}

// TestHostMemHas checks the page table and the binary search of
// hostMemSet.has against a scan of the spans, at the edges of each span
// and at random addresses around them.
func TestHostMemHas(t *testing.T) {
	s := hostMem()
	scan := func(p uintptr) bool {
		for _, sp := range s.spans {
			if sp.lo <= p && p < sp.hi {
				return true
			}
		}
		return false
	}
	check := func(p uintptr) {
		if got, want := s.has(p), scan(p); got != want {
			t.Fatalf("has(%#x) = %v, want %v", p, got, want)
		}
	}
	r := rand.New(rand.NewPCG(1, 2))
	for _, sp := range s.spans {
		for _, p := range []uintptr{sp.lo - 1, sp.lo, sp.lo + 1, sp.hi - 1, sp.hi, sp.hi + 1} {
			check(p)
		}
		for range 20 {
			check(sp.lo - 1<<14 + uintptr(r.IntN(1<<15)))
		}
	}
	check(0)
	check(^uintptr(0))
	if !s.has(uintptr(unsafe.Pointer(time.UTC))) {
		t.Error("time.UTC is not in host memory")
	}
	if s.has(uintptr(unsafe.Pointer(&http.Client{}))) {
		t.Error("a new *http.Client is in host memory")
	}
}

// BenchmarkHostWriteCheck is the cost of the check of a store of a
// restricted machine to a native object of its own (a field store on a
// *http.Client of the plugin).
func BenchmarkHostWriteCheck(b *testing.B) {
	m := &Machine{restricted: true}
	p := reflect.ValueOf(&http.Client{})
	hostMem()
	for b.Loop() {
		m.hostWritePtr(p)
	}
}
