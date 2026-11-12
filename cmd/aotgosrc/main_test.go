package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/mod/sumdb/dirhash"
)

// TestPatchReleases checks the versions read from the list of
// golang.org/toolchain: the patch releases of one release, for linux/amd64
// only, without betas and release candidates.
func TestPatchReleases(t *testing.T) {
	list := "v0.0.1-go1.27.3.linux-amd64\nv0.0.1-go1.27rc1.linux-amd64\nv0.0.1-go1.27.4.darwin-arm64\nv0.0.1-go1.26.10.linux-amd64\nv0.0.1-go1.27.0.linux-amd64\n"
	if got := patchReleases(list, "go1.27"); !reflect.DeepEqual(got, []string{"go1.27.3", "go1.27.0"}) {
		t.Errorf("patchReleases = %v", got)
	}
}

// TestFetchChecksum checks that fetch extracts the sources only when the
// hash of the downloaded zip is the one of the checksum database: a wrong
// or tampered response of the proxy must not give hashes for the manifest.
// The proxy is a local server and the checksum database a fake.
func TestFetchChecksum(t *testing.T) {
	mod, ver := toolchainModule("go1.27.4")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"src/fmt/print.go":        "package fmt\n",
		"src/fmt/internal/x/x.go": "package x\n",
		"src/net/http/server.go":  "package http\n",
		"bin/go":                  "binary",
	} {
		w, err := zw.Create(mod + "@" + ver + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zipFile := filepath.Join(t.TempDir(), "t.zip")
	if err := os.WriteFile(zipFile, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	good, err := dirhash.HashZip(zipFile, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+mod+"/@v/"+ver+".zip" {
			http.NotFound(w, r)
			return
		}
		w.Write(buf.Bytes())
	}))
	defer srv.Close()
	sum := func(h string) lookupFunc {
		return func(m, v string) (string, error) {
			if m != mod || v != ver {
				t.Errorf("lookup of %s@%s, want %s@%s", m, v, mod, ver)
			}
			return h, nil
		}
	}

	if root, err := fetch(srv.URL, "go1.27.4", []string{"fmt"}, sum("h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		os.RemoveAll(root)
		t.Fatalf("fetch with a wrong checksum: %v, want a checksum mismatch", err)
	}

	root, err := fetch(srv.URL, "go1.27.4", []string{"fmt"}, sum(good))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if b, err := os.ReadFile(filepath.Join(root, "src", "fmt", "print.go")); err != nil || string(b) != "package fmt\n" {
		t.Errorf("src/fmt/print.go: %q, %v", b, err)
	}
	for _, f := range []string{"src/fmt/internal/x/x.go", "src/net/http/server.go"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			t.Errorf("%s extracted, want only the files of the copied packages", f)
		}
	}
}
