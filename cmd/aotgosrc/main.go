// Command aotgosrc keeps aot/all/gosources.txt up to date: the manifest of
// the GOROOT sources of the standard packages that yaegi v2 copies (the AOT
// copies of aot/go1_NN), for each Go patch release (internal/gosources).
//
// With the manifest, a host built with a newer patch release than the AOT
// copies keeps the copies when that patch release did not change their
// sources (vm/aotversion.go), with no regeneration. cmd/aotgen writes the
// entry of the Go version it runs with; this command adds the newer patch
// releases:
//
//	go run ./cmd/aotgosrc -update
//
// lists the patch releases of each release of aot/ on the Go module proxy
// (golang.org/toolchain), downloads the sources of the ones that are newer
// than the copies and not in the manifest yet (about 70 MB each), checks
// each download against the Go checksum database (sum.golang.org), and adds
// their hashes. It prints, for each new patch release, the copied packages
// it changed: none means that the copies stay usable; otherwise the copies
// must be regenerated (sh scripts/aotgen.sh) with that Go version, and
// until then the hosts built with it fall back (Options.StaleAOTFallback)
// or refuse AOT mode.
//
//	go run ./cmd/aotgosrc -goroot DIR -version go1.27.4
//
// adds the entry of one version from a GOROOT on disk instead.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/mod/sumdb"
	"golang.org/x/mod/sumdb/dirhash"

	"github.com/traefik/yaegi/v2/internal/gorelease"
	"github.com/traefik/yaegi/v2/internal/gosources"
)

func main() {
	out := flag.String("o", "aot", "directory of the AOT copies (the manifest is <o>/all/gosources.txt)")
	proxy := flag.String("proxy", "https://proxy.golang.org", "Go module proxy to download the Go sources from")
	sumURL := flag.String("sumdb", "https://sum.golang.org", "URL of the Go checksum database (key of sum.golang.org) that checks the downloads")
	update := flag.Bool("update", false, "add the patch releases of the proxy that are newer than the copies")
	goroot := flag.String("goroot", "", "add the entry of -version from this GOROOT")
	version := flag.String("version", "", "with -goroot: the Go version of the GOROOT (go1.27.4)")
	flag.Parse()
	manifest := filepath.Join(*out, "all", "gosources.txt")
	switch {
	case *goroot != "" && *version != "":
		pkgs, err := gosources.StdCopies(filepath.Join(*out, gorelease.Prefix(*version)))
		check(err)
		check(gosources.Record(manifest, *version, *goroot, pkgs, false))
		report(manifest, copiesVersion(filepath.Join(*out, gorelease.Prefix(*version))), *version)
	case *update:
		check(runUpdate(*out, manifest, *proxy, sumDBLookup(*sumURL)))
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "aotgosrc:", err)
		os.Exit(1)
	}
}

// runUpdate adds the missing patch releases of each release of the window
// that has AOT copies in out.
func runUpdate(out, manifest, proxy string, sum lookupFunc) error {
	list, err := get(proxy + "/golang.org/toolchain/@v/list")
	if err != nil {
		return err
	}
	for _, w := range gorelease.Window {
		rel := fmt.Sprintf("go1.%d", w)
		tree := filepath.Join(out, gorelease.Prefix(rel))
		if _, err := os.Stat(tree); err != nil {
			continue // no copies for this release
		}
		copies := copiesVersion(tree)
		if copies == "" {
			return fmt.Errorf("%s: no GoVersion in the AOT glue", tree)
		}
		pkgs, err := gosources.StdCopies(tree)
		if err != nil {
			return err
		}
		m, err := gosources.ReadFile(manifest)
		if err != nil {
			return err
		}
		for _, v := range patchReleases(string(list), rel) {
			if m[v] != nil || gosources.Less(v, copies) {
				continue
			}
			fmt.Printf("%s: downloading the sources\n", v)
			root, err := fetch(proxy, v, pkgs, sum)
			if err != nil {
				return err
			}
			err = gosources.Record(manifest, v, root, pkgs, false)
			os.RemoveAll(root)
			if err != nil {
				return err
			}
			report(manifest, copies, v)
		}
	}
	return nil
}

// report prints whether Go version v changed the copied packages of the
// copies made with Go version copies.
func report(manifest, copies, v string) {
	m, err := gosources.ReadFile(manifest)
	check(err)
	if v == copies {
		fmt.Printf("%s: the version of the copies\n", v)
		return
	}
	same, changed, known := m.Same(copies, v)
	switch {
	case !known:
		fmt.Printf("%s: the copies (%s) are not in the manifest\n", v, copies)
	case same:
		fmt.Printf("%s: no copied package changed since %s: the copies stay usable\n", v, copies)
	default:
		fmt.Printf("%s: changed since %s: %s: regenerate the copies (sh scripts/aotgen.sh with %s)\n", v, copies, strings.Join(changed, ", "), v)
	}
}

var goVersionRE = regexp.MustCompile(`GoVersion:\s+"([^"]+)"`)

// copiesVersion returns the Go version the AOT copies of tree were made
// with (Package.GoVersion of the glue of the copy of fmt), or "".
func copiesVersion(tree string) string {
	b, err := os.ReadFile(filepath.Join(tree, "fmt", "zz_aotglue.go"))
	if err != nil {
		return ""
	}
	if m := goVersionRE.FindSubmatch(b); m != nil {
		return gorelease.StripExperiments(string(m[1]))
	}
	return ""
}

// patchReleases returns the patch releases of release rel ("go1.27") in the
// version list of golang.org/toolchain: "v0.0.1-go1.27.4.linux-amd64"
// gives "go1.27.4". Betas and release candidates are left out.
func patchReleases(list, rel string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(list))
	for sc.Scan() {
		v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "v0.0.1-")
		if !ok {
			continue
		}
		v, ok = strings.CutSuffix(v, ".linux-amd64")
		if !ok || gorelease.Release(v) != rel || strings.Count(v, ".") != 2 {
			continue
		}
		if _, ok := gorelease.Patch(v); ok {
			out = append(out, v)
		}
	}
	return out
}

func get(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// toolchainModule returns the module path and version of the toolchain of
// Go version v for linux/amd64 on the module proxy.
func toolchainModule(v string) (mod, ver string) {
	return "golang.org/toolchain", "v0.0.1-" + v + ".linux-amd64"
}

// fetch downloads the toolchain of Go version v from the proxy, checks the
// hash of the zip against sum (the Go checksum database), and extracts the
// Go files of the packages pkgs (not their subdirectories) into a new
// temporary GOROOT, which it returns. The hashes of the manifest decide that
// a host keeps old AOT copies: they must come from the real Go sources, not
// from a wrong or tampered response of the proxy.
func fetch(proxy, v string, pkgs []string, sum lookupFunc) (string, error) {
	mod, ver := toolchainModule(v)
	url := fmt.Sprintf("%s/%s/@v/%s.zip", proxy, mod, ver)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	tmp, err := os.CreateTemp("", "aotgosrc-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	n, err := io.Copy(tmp, resp.Body)
	if err != nil {
		return "", err
	}
	want, err := sum(mod, ver)
	if err != nil {
		return "", fmt.Errorf("checksum of %s@%s: %w", mod, ver, err)
	}
	got, err := dirhash.HashZip(tmp.Name(), dirhash.Hash1)
	if err != nil {
		return "", fmt.Errorf("%s: %w", url, err)
	}
	if got != want {
		return "", fmt.Errorf("%s: checksum mismatch: downloaded %s, checksum database %s", url, got, want)
	}
	zr, err := zip.NewReader(tmp, n)
	if err != nil {
		return "", err
	}
	copied := map[string]bool{}
	for _, p := range pkgs {
		copied[p] = true
	}
	root, err := os.MkdirTemp("", "aotgosrc-goroot-")
	if err != nil {
		return "", err
	}
	prefix := mod + "@" + ver + "/src/"
	for _, f := range zr.File {
		rel, ok := strings.CutPrefix(f.Name, prefix)
		if !ok || !strings.HasSuffix(rel, ".go") || !copied[path.Dir(rel)] || !f.Mode().IsRegular() {
			continue
		}
		dst := filepath.Join(root, "src", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return "", err
		}
	}
	return root, nil
}

// A lookupFunc returns the hash ("h1:...") of the zip of module mod at
// version ver.
type lookupFunc func(mod, ver string) (string, error)

// sumGolangOrgKey is the verifier key of sum.golang.org (the one of the go
// command, cmd/go/internal/modfetch/key.go).
const sumGolangOrgKey = "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8"

// sumDBLookup returns a lookupFunc that asks the checksum database at url,
// which must sign with the key of sum.golang.org. The client checks the
// signed tree and the inclusion proof of each record, like the go command.
func sumDBLookup(url string) lookupFunc {
	client := sumdb.NewClient(&sumOps{url: strings.TrimSuffix(url, "/"), config: map[string][]byte{}})
	return func(mod, ver string) (string, error) {
		lines, err := client.Lookup(mod, ver)
		if err != nil {
			return "", err
		}
		for _, line := range lines {
			if f := strings.Fields(line); len(f) == 3 && f[0] == mod && f[1] == ver {
				return f[2], nil
			}
		}
		return "", fmt.Errorf("no hash of %s@%s in %q", mod, ver, lines)
	}
}

// sumOps are the operations of sumdb.Client: HTTP reads, and a
// configuration and a cache in memory (one run of the command).
type sumOps struct {
	url    string
	mu     sync.Mutex
	config map[string][]byte
}

func (o *sumOps) ReadRemote(path string) ([]byte, error) { return get(o.url + path) }

func (o *sumOps) ReadConfig(file string) ([]byte, error) {
	if file == "key" {
		return []byte(sumGolangOrgKey), nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.config[file], nil
}

func (o *sumOps) WriteConfig(file string, old, new []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !bytes.Equal(o.config[file], old) {
		return sumdb.ErrWriteConflict
	}
	o.config[file] = new
	return nil
}

func (o *sumOps) ReadCache(string) ([]byte, error) { return nil, errors.New("no cache") }

func (o *sumOps) WriteCache(string, []byte) {}

func (o *sumOps) Log(msg string) { fmt.Fprintln(os.Stderr, "aotgosrc:", msg) }

func (o *sumOps) SecurityError(msg string) {
	fmt.Fprintln(os.Stderr, "aotgosrc: checksum database:", msg)
	os.Exit(1)
}
