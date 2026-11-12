package vm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestGoListEnv loads with Options.GoList two plugins whose go.mod asks the
// go command for a download: a newer Go release (the toolchain) and a
// module that is not vendored nor in the module cache. The host
// environment allows downloads (GOTOOLCHAIN=auto, GOFLAGS=-mod=mod, a
// GOPROXY): go list inherited it, and asked the proxy. Now go list runs
// with a minimal environment (pluginGoListEnv): the proxy gets no request,
// and the load fails with the error of the go command.
func TestGoListEnv(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	var mu sync.Mutex
	var reqs []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r.URL.Path)
		mu.Unlock()
		http.NotFound(w, r)
	}))
	defer proxy.Close()
	t.Setenv("GOTOOLCHAIN", "auto")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", proxy.URL)
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GONOSUMDB", "")
	// The go env file of the user (go env -w) says the same: go list keeps
	// it (it holds GOMODCACHE, GOPATH... of the host), but the forced
	// variables win over it.
	goenv := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(goenv, []byte("GOTOOLCHAIN=auto\nGOFLAGS=-mod=mod\nGOPROXY="+proxy.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", goenv)

	for _, c := range []struct{ name, gomod, src, want string }{
		{"toolchain", "module example.com/p\n\ngo 1.99\n", "package p\n", "GOTOOLCHAIN=local"},
		{"module", "module example.com/p\n\ngo 1.22\n\nrequire example.com/missingdep v1.0.0\n",
			"package p\n\nimport _ \"example.com/missingdep\"\n", "package errors"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range map[string]string{"go.mod": c.gomod, "p.go": c.src} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			mu.Lock()
			reqs = nil
			mu.Unlock()
			_, err := Load(Options{GoList: true, Dir: dir, Root: dir}, "example.com/p")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Load: %v, want an error with %q", err, c.want)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(reqs) > 0 {
				t.Errorf("go list asked the proxy: %q", reqs)
			}
		})
	}
}

// TestPluginGoListEnv checks the variables that pluginGoListEnv keeps and
// forces.
func TestPluginGoListEnv(t *testing.T) {
	dir := t.TempDir()
	host := []string{"PATH=/bin", "HOME=/h", "GOFLAGS=-mod=mod", "GOPROXY=https://proxy", "GOTOOLCHAIN=auto",
		"GOPRIVATE=x", "HTTPS_PROXY=http://p", "SECRET=1", "GOMODCACHE=/m", "CGO_ENABLED=0", "GOENV=/h/env", "GOTMPDIR=/t"}
	env := strings.Join(pluginGoListEnv(host, dir), " ")
	for _, want := range []string{"PATH=/bin", "HOME=/h", "GOMODCACHE=/m", "CGO_ENABLED=0", "GOENV=/h/env", "GOTMPDIR=/t",
		"GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=readonly"} {
		if !strings.Contains(" "+env+" ", " "+want+" ") {
			t.Errorf("env %q: no %s", env, want)
		}
	}
	for _, bad := range []string{"GOFLAGS=-mod=mod", "https://proxy", "GOTOOLCHAIN=auto", "GOPRIVATE", "HTTPS_PROXY", "SECRET"} {
		if strings.Contains(env, bad) {
			t.Errorf("env %q has %s", env, bad)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor", "modules.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if env := strings.Join(pluginGoListEnv(host, dir), " "); !strings.Contains(env, "GOFLAGS=-mod=vendor") {
		t.Errorf("with a vendor directory: %q", env)
	}
}
