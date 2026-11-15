package pluginhost

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/vm"
)

// TestBudgetRateByteLoop: with the host defaults, a 256 MiB download
// through a middleware that reads each byte of the response (about one
// step per byte, at full speed of one core: the server and the client are
// local) gets to its end. Before, Budget (1e8 steps) was a total per
// ServeHTTP, and the download was cut at about 95 MiB with
// "http: panic serving: vm: execution budget exhausted". Now the bucket of
// the call gets BudgetRate steps back per second, and the end of each
// Write of the wrapper is a blocking point (vm/budget.go).
func TestBudgetRateByteLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("downloads 256 MiB through the VM")
	}
	const size = 256 << 20
	opts := Options()
	opts.Dir = "../.."
	opts.GoPanic = func(r any) { t.Errorf("GoPanic: %v", r) }
	p, err := vm.Load(opts, "github.com/traefik/yaegi/v2/internal/pluginhost/testdata/byteloop")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("abcdefgh", 32<<10/8))
		for n := 0; n < size; n += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	h, err := NewHandler(context.Background(), p, backend, cfg, "byteloop")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	var logs strings.Builder
	srv.Config.ErrorLog = log.New(&logs, "", 0)
	srv.Start()
	defer srv.Close()
	start := time.Now()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if n != size || err != nil {
		t.Fatalf("got %d bytes (%d MiB), error %v; want %d (log: %q)", n, n>>20, err, size, logs.String())
	}
	if s := p.M.BudgetStops(); s != 0 {
		t.Fatalf("%d budget stops", s)
	}
	t.Logf("%d MiB in %v", n>>20, time.Since(start))
}
