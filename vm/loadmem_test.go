package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadConstsAcrossPackages checks that the bound of the string
// constants (maxConstBytes) is a bound of the load, not of one package.
// Each package below keeps an 8 MiB string constant and is far under the
// limit alone; 64 such packages kept 1 GiB of heap from 86 KiB of source
// when the limit was per package. The load is refused before go/types
// checks the package that passes the limit.
func TestLoadConstsAcrossPackages(t *testing.T) {
	needSource(t)
	files := map[string]string{"go.mod": "module example.com/p\n\ngo 1.22\n"}
	var imps, uses strings.Builder
	for i := 0; i < 4; i++ {
		fmt.Fprintf(&imps, "\t\"example.com/p/c%d\"\n", i)
		fmt.Fprintf(&uses, "var _ = c%d.N\n", i)
		var b strings.Builder
		fmt.Fprintf(&b, "package c%d\n\nconst S0 = %q\n", i, strings.Repeat("a", 1024))
		for j := 1; j <= 13; j++ {
			fmt.Fprintf(&b, "const S%d = S%d + S%d\n", j, j-1, j-1)
		}
		b.WriteString("const N = len(S13)\n")
		files[fmt.Sprintf("c%d/c.go", i)] = b.String()
	}
	files["p.go"] = "package p\n\nimport (\n" + imps.String() + ")\n\n" + uses.String()
	dir := writeModule(t, files)

	// One package alone loads.
	if _, err := Load(Options{Dir: dir}, "example.com/p/c0"); err != nil {
		t.Fatalf("one package: %v", err)
	}
	_, err := Load(Options{Dir: dir}, "example.com/p")
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), "string constants of the plugin and its packages are too large") {
		t.Fatalf("want a *LoadError on the string constants of the load, got %v", err)
	}
}

// TestLoadOnePerPlugin checks that a plugin is loaded once at a time
// (loadSlots). The first load is abandoned after its LoadTimeout while its
// SSA build has not returned (the "built" hook blocks, as go/types or the
// SSA builder on a crafted package, which cannot be canceled). A second
// load of the same plugin waits for it and fails on its own deadline
// instead of starting one more load; a load of another plugin is not
// delayed; and once the abandoned load has ended, the plugin loads.
func TestLoadOnePerPlugin(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, "package p\n\nfunc F() int { return 1 }\n")
	other, otherPath := writePlugin(t, "package p\n\nfunc F() int { return 2 }\n")

	block := make(chan struct{})
	entered := make(chan struct{}, 4)
	var builds int
	hooks := &loadHooks{stage: func(s string) {
		if s == "built" {
			builds++
			entered <- struct{}{}
			<-block
		}
	}}
	opts := Options{Dir: dir, LoadTimeout: 200 * time.Millisecond, hooks: hooks}
	if _, err := Load(opts, pkgPath); err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("first load: want the deadline error, got %v", err)
	}
	<-entered // the abandoned load is blocked after its SSA build

	// The same plugin, from the same directory written another way.
	opts.Dir = filepath.Join(dir, ".")
	_, err := Load(opts, pkgPath)
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), "a previous load of example.com/p has not ended yet") {
		t.Fatalf("second load: want a *LoadError on the previous load, got %v", err)
	}
	if builds != 1 {
		t.Fatalf("the second load started a load of its own (%d builds)", builds)
	}

	// Another plugin loads at once.
	if _, err := Load(Options{Dir: other, LoadTimeout: 10 * time.Second}, otherPath); err != nil {
		t.Fatalf("other plugin: %v", err)
	}

	// A load with no deadline waits until the abandoned load ends.
	done := make(chan error, 1)
	go func() {
		_, err := LoadContext(context.Background(), Options{Dir: dir}, pkgPath)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("a load without deadline did not wait for the abandoned load: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(block)
	if err := <-done; err != nil {
		t.Fatalf("load after the abandoned load ended: %v", err)
	}
}

// TestLoadDefaultSourceLimit checks the default bound of the source read by
// a load (DefaultLoadLimits.MaxTotalSourceBytes, 64 MiB): about 60 to 150
// MiB of heap per MiB of source at the peak of the load.
func TestLoadDefaultSourceLimit(t *testing.T) {
	needSource(t)
	if DefaultLoadLimits.MaxTotalSourceBytes != 64<<20 {
		t.Fatalf("DefaultLoadLimits.MaxTotalSourceBytes = %d, want 64 MiB", DefaultLoadLimits.MaxTotalSourceBytes)
	}
	dir, pkgPath := writePlugin(t, "package p\n\nfunc F() int { return 1 }\n")
	big := "package p\n\n// " + strings.Repeat("a", 65<<20) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(Options{Dir: dir}, pkgPath)
	var le *LoadError
	if !errors.As(err, &le) || !strings.Contains(err.Error(), "source too large") {
		t.Fatalf("want a source too large *LoadError, got %v", err)
	}
}
