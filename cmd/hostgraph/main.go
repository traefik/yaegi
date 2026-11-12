// Command hostgraph checks that the AOT copies of non-standard modules
// (aot/go1_NN, made by cmd/aotgen) still fit the module graph of a host
// program, today Traefik.
//
// A copy is generated from one version of a module: the one in yaegi v2's
// go.mod. A copy often keeps some packages of its own module native (for
// example the copy of github.com/pelletier/go-toml/v2 imports the native
// github.com/pelletier/go-toml/v2/unstable). When a host links yaegi v2, Go's
// minimal version selection (MVS) picks the highest version required in the
// whole graph. If the host requires a newer version of a copied module, its
// native packages are built at that version while the copy stays at the old
// one: the copy can stop compiling (go-toml v2.2.4 against v2.4.3 did), or,
// worse, compile and mix two versions of one library.
//
// Two modes:
//
//	hostgraph            offline: compare each copy with the versions of
//	                     yaegi v2's go.mod and of the host snapshot
//	                     (testdata/hostgraph/traefik.go.mod.txt). Fails when
//	                     a copy keeps native packages of its own module and
//	                     the host would select another version of it.
//	hostgraph -build     online (module proxy and checksum database): make a
//	                     throwaway module that requires yaegi v2 (replaced by
//	                     this checkout) and the host snapshot, build
//	                     github.com/traefik/yaegi/v2/aot/all with the go command
//	                     in PATH, then check the real selected versions.
//	hostgraph -update    online: replace the snapshot with the go.mod of
//	                     traefik/traefik at -ref (read only: nothing is sent
//	                     there).
//
// When it fails, regenerate the copy at the version of the host: update the
// module in yaegi v2's go.mod (go get module@version), then run
// scripts/aotgen.sh with each Go release of the window (docs/AOTGEN.md).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

const (
	goVMModule   = "github.com/traefik/yaegi/v2"
	snapshotPath = "testdata/hostgraph/traefik.go.mod.txt"
	sourcePath   = "testdata/hostgraph/SOURCE"
	traefikRepo  = "https://github.com/traefik/traefik"
	traefikRaw   = "https://raw.githubusercontent.com/traefik/traefik/"
)

func main() {
	root := flag.String("root", ".", "root of the yaegi v2 checkout")
	build := flag.Bool("build", false, "build aot/all in a throwaway module with the host requirements (needs the network)")
	update := flag.Bool("update", false, "replace the snapshot with the go.mod of traefik/traefik at -ref (needs the network)")
	ref := flag.String("ref", "master", "-update: branch, tag or commit of traefik/traefik")
	keep := flag.Bool("keep", false, "-build: keep the throwaway module (its path is printed)")
	flag.Parse()

	var err error
	switch {
	case *update:
		err = runUpdate(*root, *ref)
	case *build:
		err = runBuild(*root, *keep)
	default:
		var problems []string
		problems, err = runOffline(*root)
		for _, p := range problems {
			fmt.Println("FAIL:", p)
		}
		if err == nil && len(problems) > 0 {
			err = fmt.Errorf("%d copies do not fit the host graph", len(problems))
		}
		if err == nil {
			fmt.Println("hostgraph: every AOT copy fits the host snapshot")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hostgraph:", err)
		os.Exit(1)
	}
}

// Copy is the AOT copy of one non-standard package, read from its glue file.
type Copy struct {
	Dir     string // directory of the copy
	Path    string // import path of the original package
	Module  string // module the copy was generated from
	Version string // version of that module
	// Native lists the packages of Module that the copy (or a package it
	// copied with it) imports natively.
	Native []string
}

var (
	reGluePath    = regexp.MustCompile(`(?m)^\s*Path:\s*"([^"]+)"`)
	reGlueModule  = regexp.MustCompile(`(?m)^\s*Module:\s*"([^"]+)"`)
	reGlueVersion = regexp.MustCompile(`(?m)^\s*Version:\s*"([^"]+)"`)
)

// readCopies reads the copies of non-standard packages of the tree
// aot/go1_NN at dir. Directories that start with "_" (copies that do not
// build) or "." are skipped.
func readCopies(tree string) ([]Copy, error) {
	var out []Copy
	err := filepath.WalkDir(tree, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != tree && (strings.HasPrefix(d.Name(), "_") || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "zz_aotglue.go" {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		mm := reGlueModule.FindSubmatch(src)
		if mm == nil {
			return nil // standard library copy
		}
		c := Copy{Dir: filepath.Dir(p), Module: string(mm[1])}
		if m := reGluePath.FindSubmatch(src); m != nil {
			c.Path = string(m[1])
		}
		if m := reGlueVersion.FindSubmatch(src); m != nil {
			c.Version = string(m[1])
		}
		out = append(out, c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The native imports: every Go file under the directory of the module
	// in the tree (the copy and the internal packages copied with it).
	for i := range out {
		c := &out[i]
		modDir := filepath.Join(tree, filepath.FromSlash(c.Module))
		if !strings.HasPrefix(c.Dir+string(filepath.Separator), modDir+string(filepath.Separator)) {
			modDir = c.Dir
		}
		native, err := nativeImports(modDir, c.Module)
		if err != nil {
			return nil, err
		}
		c.Native = native
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// nativeImports lists the imports of the Go files under dir that are
// packages of module mod (not rewritten to an AOT copy).
func nativeImports(dir, mod string) ([]string, error) {
	seen := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, im := range f.Imports {
			path := strings.Trim(im.Path.Value, "\"")
			if path == mod || strings.HasPrefix(path, mod+"/") {
				seen[path] = true
			}
		}
		return nil
	})
	var out []string
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, err
}

// replacedBy is the version given by requires to a module that a replace
// directive sends to another module or to a directory: no copy fits it.
const replacedBy = "replaced:"

// requires returns the required version of each module of a go.mod file,
// with its replacements applied: a replace that only changes the version
// gives that version, and any other replace gives replacedBy and its target.
func requires(name string, data []byte) (map[string]string, error) {
	f, err := modfile.Parse(name, data, nil) // ParseLax drops replace directives
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range f.Require {
		out[r.Mod.Path] = r.Mod.Version
	}
	for _, r := range f.Replace {
		v, ok := out[r.Old.Path]
		if !ok || (r.Old.Version != "" && r.Old.Version != v) {
			continue
		}
		if r.New.Path == r.Old.Path && r.New.Version != "" {
			out[r.Old.Path] = r.New.Version
		} else {
			out[r.Old.Path] = replacedBy + r.New.String()
		}
	}
	return out, nil
}

// trees lists the aot/go1_NN directories of the checkout.
func trees(root string) ([]string, error) {
	m, err := filepath.Glob(filepath.Join(root, "aot", "go1_*"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range m {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out, nil
}

// runOffline compares the copies with the versions that MVS would select in
// a host that requires the snapshot: the highest of yaegi v2's go.mod and of
// the snapshot (both list every module that provides a package, since Go
// 1.17, so this is the selected version unless a third module raises it;
// -build checks the real graph).
func runOffline(root string) ([]string, error) {
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	own, err := requires("go.mod", gomod)
	if err != nil {
		return nil, err
	}
	snap, err := os.ReadFile(filepath.Join(root, snapshotPath))
	if err != nil {
		return nil, err
	}
	host, err := requires(snapshotPath, snap)
	if err != nil {
		return nil, err
	}
	ts, err := trees(root)
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, tree := range ts {
		copies, err := readCopies(tree)
		if err != nil {
			return nil, err
		}
		problems = append(problems, checkCopies(filepath.Base(tree), copies, own, host)...)
	}
	return problems, nil
}

// checkCopies returns the problems of the copies of one tree, given the
// requirements of yaegi v2 and of the host.
func checkCopies(tree string, copies []Copy, own, host map[string]string) []string {
	var problems []string
	for _, c := range copies {
		mine := own[c.Module]
		if mine != c.Version {
			problems = append(problems, fmt.Sprintf("%s: the copy of %s is from %s@%s, but yaegi v2's go.mod requires %s: run scripts/aotgen.sh",
				tree, c.Path, c.Module, c.Version, orNone(mine)))
			continue
		}
		hv, ok := host[c.Module]
		if target, found := strings.CutPrefix(hv, replacedBy); found {
			problems = append(problems, fmt.Sprintf("%s: the copy of %s is from %s@%s, but the host replaces %s with %s: no copy fits",
				tree, c.Path, c.Module, c.Version, c.Module, target))
			continue
		}
		sel := mine
		if ok && semver.Compare(hv, sel) > 0 {
			sel = hv
		}
		if sel == c.Version {
			continue
		}
		if len(c.Native) > 0 {
			problems = append(problems, fmt.Sprintf("%s: the copy of %s is from %s@%s, but the host selects %s for its native packages %s (two versions of one module): update %s to %s in yaegi v2's go.mod and run scripts/aotgen.sh",
				tree, c.Path, c.Module, c.Version, sel, strings.Join(c.Native, ", "), c.Module, sel))
			continue
		}
		fmt.Printf("note: %s: the copy of %s is from %s, the host selects %s (the copy is complete; a plugin that vendors %s runs it interpreted)\n",
			tree, c.Path, c.Version, sel, sel)
	}
	return problems
}

func orNone(s string) string {
	if s == "" {
		return "nothing"
	}
	return s
}

// hostModFile makes the go.mod of the throwaway module: the snapshot with
// another module line, without its local replacements and tool lines, plus
// a requirement of yaegi v2 replaced by the checkout at root.
func hostModFile(snap []byte, root string) ([]byte, error) {
	f, err := modfile.Parse(snapshotPath, snap, nil)
	if err != nil {
		return nil, err
	}
	if err := f.AddModuleStmt("example.com/hostgraph"); err != nil {
		return nil, err
	}
	for _, r := range f.Replace {
		if modfile.IsDirectoryPath(r.New.Path) {
			old := r.Old // DropReplace clears r
			if err := f.DropReplace(old.Path, old.Version); err != nil {
				return nil, err
			}
			// A module of the host's own repository: it is only
			// reachable through the local replacement.
			if err := f.DropRequire(old.Path); err != nil {
				return nil, err
			}
		}
	}
	for _, t := range f.Tool {
		if err := f.DropTool(t.Path); err != nil {
			return nil, err
		}
	}
	if err := f.AddRequire(goVMModule, "v0.0.0"); err != nil {
		return nil, err
	}
	if err := f.AddReplace(goVMModule, "", root, ""); err != nil {
		return nil, err
	}
	f.Cleanup()
	return f.Format()
}

// onlineEnv is the environment of the go commands of -build and -update:
// the module proxy and the checksum database are needed, whatever the
// offline settings of the session (GOPROXY=off, GOSUMDB=off).
func onlineEnv() []string {
	env := os.Environ()
	set := func(k, v string) {
		for i, e := range env {
			if strings.HasPrefix(e, k+"=") {
				env[i] = k + "=" + v
				return
			}
		}
		env = append(env, k+"="+v)
	}
	if p := os.Getenv("GOPROXY"); p == "" || p == "off" {
		set("GOPROXY", "https://proxy.golang.org,direct")
	}
	if s := os.Getenv("GOSUMDB"); s == "" || s == "off" {
		set("GOSUMDB", "sum.golang.org")
	}
	// The snapshot is not tidy for this module: let the go command add
	// what it needs, but never remove a requirement (no go mod tidy, which
	// would lower the versions MVS selects).
	set("GOFLAGS", "-mod=mod")
	set("GOWORK", "off")
	return env
}

func goCmd(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = onlineEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

func runBuild(root string, keep bool) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	snap, err := os.ReadFile(filepath.Join(abs, snapshotPath))
	if err != nil {
		return err
	}
	mod, err := hostModFile(snap, abs)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "hostgraph")
	if err != nil {
		return err
	}
	if keep {
		fmt.Println("module:", dir)
	} else {
		defer os.RemoveAll(dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), mod, 0o644); err != nil {
		return err
	}
	main := "package main\n\nimport _ \"" + goVMModule + "/aot/all\"\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		return err
	}
	if _, err := goCmd(dir, "build", "-o", os.DevNull, "."); err != nil {
		return fmt.Errorf("aot/all does not build in the host graph: %v", err)
	}
	// The real versions: each package of the build with its module.
	out, err := goCmd(dir, "list", "-deps", "-f", "{{.ImportPath}} {{.Dir}} {{with .Module}}{{.Path}} {{.Version}}{{end}}", ".")
	if err != nil {
		return err
	}
	type pkg struct{ path, dir, mod, ver string }
	var pkgs []pkg
	trees := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		p := pkg{path: f[0], dir: f[1]}
		if len(f) >= 4 {
			p.mod, p.ver = f[2], f[3]
		}
		pkgs = append(pkgs, p)
		if rest, ok := strings.CutPrefix(p.path, goVMModule+"/aot/"); ok {
			if t, _, ok := strings.Cut(rest, "/"); ok && strings.HasPrefix(t, "go1_") {
				trees[filepath.Join(abs, "aot", t)] = true
			}
		}
	}
	selected := map[string]string{}
	for _, p := range pkgs {
		if p.mod != "" && p.mod != goVMModule {
			selected[p.mod] = p.ver
		}
	}
	var problems []string
	for tree := range trees {
		copies, err := readCopies(tree)
		if err != nil {
			return err
		}
		for _, c := range copies {
			sel, ok := selected[c.Module]
			if !ok {
				out, err := goCmd(dir, "list", "-m", "-f", "{{.Version}}", c.Module)
				if err != nil {
					return err
				}
				sel = strings.TrimSpace(string(out))
			}
			if sel == c.Version {
				continue
			}
			if len(c.Native) > 0 {
				problems = append(problems, fmt.Sprintf("%s: the copy of %s is from %s@%s, but the host builds %s at %s (two versions of one module)",
					filepath.Base(tree), c.Path, c.Module, c.Version, strings.Join(c.Native, ", "), sel))
				continue
			}
			fmt.Printf("note: %s: the copy of %s is from %s, the host selects %s\n", filepath.Base(tree), c.Path, c.Version, sel)
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		fmt.Println("FAIL:", p)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d copies do not fit the host graph", len(problems))
	}
	fmt.Println("hostgraph: aot/all builds in the host graph, every copy fits")
	return nil
}

// lsRemoteCommit returns the commit of ref in the output of git ls-remote:
// the branch refs/heads/<ref>, else the commit of the tag refs/tags/<ref>
// (its peeled line for an annotated tag). git ls-remote matches patterns by
// their tail, so refs/heads/backport/<ref> is skipped. It returns ref itself
// when no line matches (a commit hash).
func lsRemoteCommit(out, ref string) string {
	got := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			got[f[1]] = f[0]
		}
	}
	for _, name := range []string{"refs/heads/" + ref, "refs/tags/" + ref + "^{}", "refs/tags/" + ref} {
		if sha, ok := got[name]; ok {
			return sha
		}
	}
	return ref
}

// runUpdate replaces the snapshot with the go.mod of traefik/traefik at ref,
// and records the commit and the date in testdata/hostgraph/SOURCE.
func runUpdate(root, ref string) error {
	out, err := exec.Command("git", "ls-remote", traefikRepo, "refs/heads/"+ref, "refs/tags/"+ref, "refs/tags/"+ref+"^{}").Output()
	if err != nil {
		return fmt.Errorf("git ls-remote %s %s: %v", traefikRepo, ref, err)
	}
	sha := lsRemoteCommit(string(out), ref)
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sha) {
		return fmt.Errorf("%s: not a branch, a tag or a full commit hash of %s", ref, traefikRepo)
	}
	client := &http.Client{Timeout: time.Minute}
	resp, err := client.Get(traefikRaw + sha + "/go.mod")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET go.mod at %s: %s", sha, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if _, err := modfile.Parse("go.mod", data, nil); err != nil {
		return fmt.Errorf("go.mod at %s: %v", sha, err)
	}
	if err := os.WriteFile(filepath.Join(root, snapshotPath), data, 0o644); err != nil {
		return err
	}
	src := fmt.Sprintf("traefik/traefik go.mod, commit %s (ref %s), fetched %s\n", sha, ref, time.Now().UTC().Format("2006-01-02"))
	if err := os.WriteFile(filepath.Join(root, sourcePath), []byte(src), 0o644); err != nil {
		return err
	}
	fmt.Print(src)
	return nil
}
