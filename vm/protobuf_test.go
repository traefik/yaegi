package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
	"github.com/traefik/yaegi/v2/vm/testdata/pbplug"
)

const pbplugPkg = "github.com/traefik/yaegi/v2/vm/testdata/pbplug"

// pbplugModule returns a plugin module as Traefik gets it: the sources of
// testdata/pbplug at the root of a module of the same path, with
// google.golang.org/protobuf in vendor/ (go mod vendor, from the module
// cache that scripts/setup.sh fills: no network).
func pbplugModule(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	for _, f := range []string{"pbplug.go", "pb/event.pb.go"} {
		b, err := os.ReadFile(filepath.Join("testdata", "pbplug", f))
		if err != nil {
			tb.Fatal(err)
		}
		dst := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	gomod := "module " + pbplugPkg + "\n\ngo 1.23\n\nrequire google.golang.org/protobuf v1.36.11\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		tb.Fatal(err)
	}
	// the hashes of yaegi v2's go.sum (protobuf and what its go.mod names)
	sum, err := os.ReadFile(filepath.Join("..", "go.sum"))
	if err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); err != nil {
		tb.Fatal(err)
	}
	cmd := exec.Command("go", "mod", "vendor")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("go mod vendor (the module cache needs google.golang.org/protobuf v1.36.11: scripts/setup.sh): %v\n%s", err, out)
	}
	return dir
}

// loadPbplug loads the protobuf plugin from dir as Traefik does and returns
// its handler.
func loadPbplug(tb testing.TB, dir string, opts Options) http.Handler {
	tb.Helper()
	opts.Dir = dir
	opts.UseUnsafe = true // protobuf uses unsafe: the manifest of such a plugin has useUnsafe
	p, err := Load(opts, pbplugPkg)
	if err != nil {
		tb.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		tb.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "pbplug")
	if err != nil {
		tb.Fatal(err)
	}
	return h
}

func serveBody(h http.Handler, path string) string {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Body.String()
}

// TestProtobufPlugin runs a plugin with its own generated protobuf messages
// (vm/testdata/pbplug: oneof, proto3 optional, maps of messages, repeated
// fields, enums, Any, Timestamp, Struct) through Marshal, Unmarshal, Equal,
// Clone, Merge, Reset, protojson, prototext and protoreflect, and compares
// the result with the same code compiled by gc. testdata/protobuf.go runs
// the same code with yaegi v2.
func TestProtobufPlugin(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := pbplugModule(t)
	want := pbplug.Report()
	t.Setenv("GOFLAGS", "-mod=vendor") // the go list loader reads vendor/
	for _, opts := range []Options{{}, {AOT: true}, {GoList: true}} {
		h := loadPbplug(t, dir, opts)
		if got := serveBody(h, "/report"); got != want {
			t.Errorf("%+v:\ngot:\n%s\nwant:\n%s", opts, got, want)
		}
		// on several goroutines: the regions of unsafe are per goroutine
		done := make(chan string, 8)
		for i := 0; i < 8; i++ {
			go func() { done <- serveBody(h, "/roundtrip") }()
		}
		for i := 0; i < 8; i++ {
			if got := <-done; got == "" || got == "0" {
				t.Errorf("%+v: roundtrip on a goroutine: %q", opts, got)
			}
		}
	}
}

// BenchmarkProtobuf times 50 round trips of an event with 20 headers
// (Marshal, Unmarshal, Equal), in the VM and natively.
func BenchmarkProtobuf(b *testing.B) {
	if !stdtypes.Available() {
		b.Skip("no embedded standard library types")
	}
	b.Run("vm", func(b *testing.B) {
		h := loadPbplug(b, pbplugModule(b), Options{})
		serveBody(h, "/roundtrip") // compile the functions first
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			serveBody(h, "/roundtrip")
		}
	})
	b.Run("native", func(b *testing.B) {
		cfg := pbplug.CreateConfig()
		for i := 0; i < b.N; i++ {
			for j := 0; j < cfg.N; j++ {
				pbplug.Roundtrip()
			}
		}
	})
}
