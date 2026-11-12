package vm

// Loading a plugin without the Go toolchain: the plugin's packages, the
// other packages of its module and its vendored modules are parsed from
// source and type-checked against the standard library types embedded by
// package stdtypes. Nothing runs go list or reads GOROOT: a host may ship
// only the plugin archive and a binary.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/tools/go/ssa"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// srcLoader resolves and type-checks the source packages of one plugin.
type srcLoader struct {
	lctx     context.Context // load deadline, checked between packages
	budget   *loadBudget
	fset     *token.FileSet
	std      *stdtypes.Universe
	ctx      build.Context
	modRoot  string // root of the main module (the module of the plugin package)
	modPath  string
	archRoot string // root of the module holding Options.Dir (the plugin archive)
	archPath string
	dir      string             // Options.Dir, absolute: archRoot or a directory below it
	modAbove bool               // archRoot is above dir: the plugin directory has no go.mod
	realRoot string             // archRoot without symbolic links: nothing is read outside of it
	nested   map[string]bool    // directory -> it has a go.mod (dirs below modRoot)
	gomod    *modFile           // go.mod of the main module (nil in GOPATH mode)
	goVer    string             // go directive of the main module
	vendGo   map[string]string  // vendored module -> its go directive (vendor/modules.txt), "" without one
	vendMod  map[string]vendMod // vendored module -> its version (vendor/modules.txt)
	// vendor is vendor/modules.txt as the go command reads it, for the
	// consistency check (checkVendor): the modules in their order, with their
	// explicit mark and replacement. nil when the module has no modules.txt.
	vendor *vendorList
	// newerGo lists the go directives that capVersion lowered to the release
	// the VM was built with ("go.mod (go 1.27)"), refused without
	// Options.AllowNewerGo.
	newerGo []string
	pkgs    map[string]*srcPkg
	order   []*srcPkg                 // dependencies first
	bps     map[string]*build.Package // importDir results, by directory
	parsed  map[string]*ast.File      // parseFile results, by file name
	// pol says what the loader accepts beyond go build (the options of
	// Options for plugins written for yaegi).
	pol loadPolicy
	// goPathSrc is <Options.GoPath>/src in GOPATH mode (gopath.go), "" in
	// module mode. modRoot and archRoot are then this directory too, and
	// modPath and archPath are "".
	goPathSrc string
	// purego holds the directories that importDir selected again with the
	// purego and noasm tags.
	purego map[string]bool
	// pluginPath is the package path of the plugin, pluginTags the build
	// tags of its "// yaegi:tags" lines, and notes the notes of Plugin.Notes
	// (yaegifiles.go).
	pluginPath string
	pluginTags []string
	notes      []string
}

// vendMod is the version of a vendored module, and whether go.mod replaces
// it ("# path version => replacement" in vendor/modules.txt).
type vendMod struct {
	version  string
	replaced bool
}

// loadPolicy says what the default loader accepts beyond what go build
// accepts: the options of Options for plugins written for yaegi (see the
// comment above Options.IgnoreSoftErrors). The zero policy is go build.
type loadPolicy struct {
	ignore            typeErrPolicy // IgnoreSoftErrors, IgnoreUnusedExprs (softerr.go)
	ignoreTests       bool          // IgnoreTestFiles (testfiles.go)
	dupDecls          bool          // IgnoreDuplicateDecls (dupdecls.go)
	yaegiTags         bool          // YaegiTags (yaegiTags, run.go)
	allowNewerGo      bool          // AllowNewerGo (capVersion)
	importFromArchive bool          // ImportFromArchive (dirOf)
	ignoreVendor      bool          // IgnoreInconsistentVendoring (checkVendor)
}

// loadPolicy returns the policy of the default loader for opts.
func (opts Options) loadPolicy() loadPolicy {
	return loadPolicy{
		ignore:            opts.typeErrPolicy(),
		ignoreTests:       opts.IgnoreTestFiles,
		dupDecls:          opts.IgnoreDuplicateDecls,
		yaegiTags:         opts.YaegiTags,
		allowNewerGo:      opts.AllowNewerGo,
		importFromArchive: opts.ImportFromArchive,
		ignoreVendor:      opts.IgnoreInconsistentVendoring,
	}
}

type srcPkg struct {
	path  string
	files []*ast.File
	info  *types.Info
	types *types.Package
	err   error
	done  bool
}

// errNoGoMod is wrapped by the error of findModule when it finds no go.mod.
// A caller that can load a directory without a module (a GOPATH mode)
// tests it with errors.Is; the other errors (a go.mod that does not parse,
// dir outside of limit) must refuse the load.
var errNoGoMod = errors.New("no go.mod")

// findModule returns the root directory, the module path and the parsed
// go.mod of the module holding dir: the go.mod of dir, or else of the
// nearest parent of dir that has one, as the go command finds it.
//
// limit is Options.Root. When it is empty, the search goes up to the root of
// the file system, as the go command does. When it is set, dir must be limit
// or a directory below it (see underRoot), and the search stops at limit:
// the go.mod of limit is read, the go.mod files above it are not. This keeps
// a plugin without a go.mod of its own from taking the go.mod of an unrelated
// project above it.
//
// Each go.mod is read with the rule of the module files (openModuleFile): a
// go.mod that is a symbolic link is followed only when its target is a
// regular file below the directory (without symbolic links), as in a
// Kubernetes ConfigMap volume.
//
// When no go.mod is found, the error wraps errNoGoMod. root is an absolute
// path (without symbolic links when underRoot had to resolve them).
func findModule(dir, limit string) (root, path string, gomod *modFile, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", nil, err
	}
	// steps is the number of parents of abs the search may visit: -1 without
	// a limit. It comes from the check of underRoot, not from a comparison
	// of d with top at each step, so the search stops at Root even when the
	// two paths differ in case (Windows).
	var top string
	steps := -1
	if limit != "" {
		if abs, top, steps, err = underRoot(abs, limit); err != nil {
			return "", "", nil, err
		}
	}
	for d := abs; ; d = filepath.Dir(d) {
		// A directory that does not exist has no go.mod.
		if real, err := filepath.EvalSymlinks(d); err == nil {
			f, ok, err := readGoMod(real, d)
			if err != nil {
				return "", "", nil, err
			}
			if ok {
				return d, f.Module.Mod.Path, f, nil
			}
		}
		if steps == 0 {
			return "", "", nil, fmt.Errorf("%w in %s or its parents up to %s (Options.Root): the plugin needs a go.mod file", errNoGoMod, abs, top)
		}
		steps--
		if p := filepath.Dir(d); p == d {
			return "", "", nil, fmt.Errorf("%w in %s or its parents", errNoGoMod, abs)
		}
	}
}

// maxModFileBytes is the largest go.mod the loader reads into memory: the
// Go files have the load budget (LoadLimits), a go.mod has this fixed limit,
// far above any real one (the go.mod of a plugin has a few lines).
const maxModFileBytes = 16 << 20

// underRoot checks that dir (an absolute path, Options.Dir) is limit
// (Options.Root) or a directory below it. It returns the two paths to walk
// and the number of parents of dir up to limit (0 when they are the same
// directory).
//
// The paths are compared as given first. When that fails, they are compared
// without their symbolic links: a host may give Root through a link and Dir
// resolved, or the reverse (/tmp and /private/tmp on macOS). The search for
// go.mod then walks the resolved paths, so that it stops at Root.
func underRoot(dir, limit string) (abs, top string, steps int, err error) {
	top, err = filepath.Abs(limit)
	if err != nil {
		return "", "", 0, err
	}
	if rel, ok := relBelow(top, dir); ok {
		return dir, top, depthOf(rel), nil
	}
	realDir, err1 := filepath.EvalSymlinks(dir)
	realTop, err2 := filepath.EvalSymlinks(top)
	if err1 == nil && err2 == nil {
		if rel, ok := relBelow(realTop, realDir); ok {
			return realDir, realTop, depthOf(rel), nil
		}
	}
	return "", "", 0, fmt.Errorf("the plugin directory %s is not in the root directory %s (Options.Root)", dir, top)
}

// relBelow returns the path of p relative to base, and whether p is base or
// a directory below it ("/a/bc" is not below "/a/b"). The paths are absolute
// and clean. On Windows, the comparison ignores the case, as filepath.Rel
// does.
func relBelow(base, p string) (rel string, ok bool) {
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// depthOf returns the number of elements of a relative path below a
// directory: 0 for ".".
func depthOf(rel string) int {
	if rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// readGoMod parses the go.mod file of directory d, below the plugin archive
// whose root without symbolic links is realRoot. ok is false when d has no
// go.mod file. A go.mod that is not a regular file in the archive is refused
// (see openModuleFile). The file is parsed as the go.mod of a main module
// (modfile.Parse), as the go command parses it: an unknown directive is an
// error, and the replace directives are read (checkVendor compares them
// with vendor/modules.txt; modfile.ParseLax drops them).
func readGoMod(realRoot, d string) (f *modFile, ok bool, err error) {
	name := filepath.Join(d, "go.mod")
	file, err := openModuleFile(realRoot, name)
	if file == nil || err != nil {
		return nil, err != nil, err
	}
	b, err := io.ReadAll(io.LimitReader(file, maxModFileBytes+1))
	file.Close()
	if err != nil {
		return nil, true, err
	}
	if len(b) > maxModFileBytes {
		return nil, true, loadErrorf("vm load: %s is larger than %d bytes", name, maxModFileBytes)
	}
	mf, strictErr := modfile.Parse(name, b, nil)
	lax := false
	if strictErr != nil {
		// As the go command (ReadModFile of cmd/go): a go.mod that asks
		// for a Go release newer than the host and that the host does not
		// parse is a newer go.mod (the new release may have a directive
		// the host does not know), not a bad one. It is read as
		// modfile.ParseLax reads it: checkNewerGo refuses it unless
		// Options.AllowNewerGo is set.
		mf, err = modfile.ParseLax(name, b, nil)
		if err != nil || mf.Go == nil || !versionLess(stdtypes.Release(runtime.Version()), minorVersion("go"+mf.Go.Version)) {
			return nil, true, strictErr
		}
		lax = true
	}
	if mf.Module == nil {
		return nil, true, fmt.Errorf("%s: no module directive", name)
	}
	return &modFile{File: mf, lax: lax}, true, nil
}

// modFile is a parsed go.mod. lax says that modfile.Parse refused it and
// that modfile.ParseLax read it, because it asks for a Go release newer
// than the host (see readGoMod): it then has no replace directives.
type modFile struct {
	*modfile.File
	lax bool
}

// openModuleFile opens a file of the plugin archive (a Go file, a nested
// go.mod, vendor/modules.txt) for reading. It returns a nil file and no error
// when name does not exist or is a directory.
//
// Nothing may be read outside the archive (realRoot, without symbolic links).
// A symbolic link is followed, as Go follows it, when its final target (after
// every link, in the name and in its directories) is a regular file below
// realRoot. A Kubernetes ConfigMap volume makes every file a link
// (main.go -> ..data/main.go, and ..data -> ..2026_10_22_xxx/): such a plugin
// loads. A link that leaves realRoot, a dangling link, a link loop and a
// target that is not a regular file (a FIFO, a device, a directory) are
// refused.
//
// The file is opened through an os.Root at realRoot, on the resolved path. If
// a link is changed between the check and the open, the open still cannot
// leave realRoot (os.Root refuses it), and the opened file is checked again
// (see openRegular): a file swapped for a FIFO or a device after the check is
// refused, without blocking on it.
func openModuleFile(realRoot, name string) (*os.File, error) {
	lst, err := os.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) || err == nil && lst.IsDir() {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(name)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot resolve the symbolic link: %v", name, err)
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("%s is outside the module %s", name, realRoot)
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	st, err := root.Lstat(rel)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	f, err := openRegular(root, rel)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	if fst, err := f.Stat(); err != nil || !os.SameFile(st, fst) {
		f.Close()
		return nil, fmt.Errorf("%s changed while it was opened", name)
	}
	return f, nil
}

// openRegular opens the regular file rel of root for reading, and refuses
// any other kind of file after the open (fstat). The open never blocks: on
// Unix the file is opened with O_NONBLOCK (openNoBlock), so the open of a
// FIFO without a writer returns at once instead of waiting for one, and the
// FIFO is then refused. A regular file reads the same with the flag. Without
// it, a regular file swapped for a FIFO between the check of the caller and
// the open blocked the load for ever.
func openRegular(root *os.Root, rel string) (*os.File, error) {
	f, err := root.OpenFile(rel, os.O_RDONLY|openNoBlock, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("not a regular file")
	}
	return f, nil
}

// limitedFile is a module file whose reads stop at a size (see the OpenFile
// of newSrcLoader).
type limitedFile struct {
	io.Reader
	f *os.File
}

func (l limitedFile) Close() error { return l.f.Close() }

// mainModule returns the module that holds package pkgPath, given the module
// of the plugin archive (root, path): the innermost module, below root, whose
// directory holds the directory of pkgPath and whose module path is the
// import path of that directory. Go builds a package with the go.mod and the
// vendor directory of its own module: a nested module (a directory with its
// own go.mod, as plugins/traefik of darkweak/souin) is not part of the
// module around it. Without such a nested module, it returns root itself.
func mainModule(root, path string, gomod *modFile, pkgPath string) (string, string, *modFile, error) {
	rel, ok := strings.CutPrefix(pkgPath, path+"/")
	if !ok || module.CheckImportPath(pkgPath) != nil {
		return root, path, gomod, nil
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", nil, err
	}
	elems := strings.Split(rel, "/")
	for i := len(elems); i > 0; i-- {
		sub := strings.Join(elems[:i], "/")
		d := filepath.Join(root, filepath.FromSlash(sub))
		f, ok, err := readGoMod(realRoot, d)
		if err != nil {
			return "", "", nil, err
		}
		if ok && f.Module.Mod.Path == path+"/"+sub {
			return d, f.Module.Mod.Path, f, nil
		}
	}
	return root, path, gomod, nil
}

// pluginModuleDir returns the directory go list runs in for package pkgPath
// of the plugin in dir: the root of its nested module when it has one (see
// mainModule), dir otherwise. limit is Options.Root (see findModule).
func pluginModuleDir(dir, limit, pkgPath string) string {
	root, path, gomod, err := findModule(dir, limit)
	if err != nil {
		return dir
	}
	if d, _, _, err := mainModule(root, path, gomod, pkgPath); err == nil && d != root {
		return d
	}
	return dir
}

// newSrcLoader returns the loader of package pkgPath of the plugin in dir.
// limit is Options.Root (see findModule). When no go.mod is found and
// goPath (Options.GoPath) is set, the loader reads the plugin in GOPATH mode
// (gopath.go).
func newSrcLoader(ctx context.Context, dir, limit, goPath, pkgPath string, std *stdtypes.Universe, budget *loadBudget) (*srcLoader, error) {
	archRoot, archPath, gomod, err := findModule(dir, limit)
	if errors.Is(err, errNoGoMod) && goPath != "" {
		return newGoPathLoader(ctx, dir, goPath, pkgPath, std, budget)
	}
	if err != nil {
		return nil, err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	root, path, gomod, err := mainModule(archRoot, archPath, gomod, pkgPath)
	if err != nil {
		return nil, err
	}
	bctx := build.Default
	bctx.GOOS, bctx.GOARCH = runtime.GOOS, runtime.GOARCH
	bctx.GOROOT, bctx.GOPATH = "", ""
	bctx.CgoEnabled = false // the VM cannot run cgo
	// No purego or noasm tag here: importDir adds them only for the
	// packages that need them, so the other packages keep the files of go
	// run.
	bctx.Dir = root
	realRoot, err := filepath.EvalSymlinks(archRoot)
	if err != nil {
		return nil, err
	}
	bctx.OpenFile = moduleOpenFile(realRoot, budget)
	// The go.mod is above Options.Dir when the two directories differ
	// without their symbolic links: findModule may have resolved them (see
	// underRoot).
	modAbove := archRoot != absDir
	if realDir, err := filepath.EvalSymlinks(absDir); err == nil {
		modAbove = realRoot != realDir
	}
	l := &srcLoader{realRoot: realRoot, budget: budget, lctx: ctx,
		fset: std.Fset, std: std, ctx: bctx, modRoot: root, modPath: path,
		archRoot: archRoot, archPath: archPath, dir: absDir, modAbove: modAbove, nested: map[string]bool{},
		gomod: gomod, goVer: std.GoVersion, vendGo: map[string]string{}, vendMod: map[string]vendMod{}, pkgs: map[string]*srcPkg{},
		bps: map[string]*build.Package{}, parsed: map[string]*ast.File{}, purego: map[string]bool{},
	}
	// go1.16 without a go directive, as the go command builds such a
	// module (goModVersion).
	l.goVer = l.capVersion("go.mod", goModVersion(gomod.File))
	if err := l.readVendorVersions(); err != nil {
		return nil, err
	}
	return l, nil
}

// moduleOpenFile returns the OpenFile of the build.Context of a loader whose
// files are below realRoot (the module root, or <GoPath>/src in GOPATH mode,
// without symbolic links). go/build reads the start of each file (package
// clause, build tags): with the same checks as parseFile (openModuleFile), it
// reads nothing outside realRoot and never blocks on a FIFO. It keeps the
// header of the file in memory (the comments, the package clause and the
// imports), and yaegiTags reads the whole file: both read at most the total
// source budget, so a huge file is not held in memory before parseFile
// refuses it (the budget counts its size).
func moduleOpenFile(realRoot string, budget *loadBudget) func(string) (io.ReadCloser, error) {
	return func(name string) (io.ReadCloser, error) {
		f, err := openModuleFile(realRoot, name)
		if f == nil && err == nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		if err != nil {
			return nil, err
		}
		if max := budget.limits.MaxTotalSourceBytes; max > 0 {
			return limitedFile{io.LimitReader(f, max), f}, nil
		}
		return f, nil
	}
}

// capVersion limits a go directive to the release the VM was built with:
// go/types refuses newer versions, and code that compiles under them without
// using their new features runs the same (yaegi ignores go directives). A
// directive of a newer release (major.minor: the patch level does not change
// the language) is recorded in newerGo, with who holds it ("go.mod", or
// "vendor/modules.txt (module M)"): the load refuses it unless
// Options.AllowNewerGo is set (checkNewerGo), as the go command refuses to
// build with an older release than the directive asks for.
func (l *srcLoader) capVersion(who, v string) string {
	if versionLess(l.std.GoVersion, v) {
		if versionLess(l.std.GoVersion, minorVersion(v)) {
			l.newerGo = append(l.newerGo, fmt.Sprintf("%s asks for go %s", who, strings.TrimPrefix(v, "go")))
		}
		return l.std.GoVersion
	}
	return v
}

// minorVersion returns the major.minor part of a "go1.21.3" style version.
func minorVersion(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) > 2 {
		return strings.Join(parts[:2], ".")
	}
	return v
}

// checkNewerGo refuses the load when a go directive asks for a release newer
// than the one the VM was built with (see capVersion), unless the policy
// allows it. The message follows the go command ("go: go.mod requires go >=
// 1.27 (running go 1.26; GOTOOLCHAIN=local)").
func (l *srcLoader) checkNewerGo() error {
	if l.pol.allowNewerGo || len(l.newerGo) == 0 {
		return nil
	}
	return loadErrorf("vm load: %s, newer than the Go release of the host (%s): go build would refuse it with this release (or download the newer one); set Options.AllowNewerGo to type check the plugin as %s anyway",
		strings.Join(l.newerGo, ", "), l.std.GoVersion, l.std.GoVersion)
}

// versionLess compares "go1.21" style versions (major.minor[.patch]).
func versionLess(a, b string) bool {
	pa := strings.Split(strings.TrimPrefix(a, "go"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "go"), ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscan(pa[i], &x)
		}
		if i < len(pb) {
			fmt.Sscan(pb[i], &y)
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// readVendorVersions reads the go directives of the vendored modules
// ("## explicit; go 1.16" lines of vendor/modules.txt): they decide language
// semantics such as per-iteration loop variables. It also reads the module
// versions ("# path version [=> replacement]"), for the AOT version check.
// A modules.txt that is not a regular file in the archive is refused (see
// openModuleFile).
func (l *srcLoader) readVendorVersions() error {
	name := filepath.Join(l.modRoot, "vendor", "modules.txt")
	f, err := openModuleFile(l.realRoot, name)
	if f == nil || err != nil {
		return err
	}
	defer f.Close()
	l.vendor = &vendorList{meta: map[module.Version]vendorMeta{}, listed: map[module.Version]bool{}}
	sc := bufio.NewScanner(f)
	mod := ""
	var cur module.Version // the module of the last "# " line, for the lines below it
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# "):
			f := strings.Fields(line[2:])
			mod = ""
			cur = module.Version{}
			if len(f) >= 1 {
				mod = f[0]
				if _, ok := l.vendGo[mod]; !ok {
					l.vendGo[mod] = ""
				}
				vm := vendMod{replaced: strings.Contains(line, "=>")}
				if len(f) >= 2 && f[1] != "=>" {
					vm.version = f[1]
				}
				l.vendMod[mod] = vm
				cur = l.vendor.add(f)
			}
		case strings.HasPrefix(line, "## ") && mod != "":
			for _, part := range strings.Split(line[3:], ";") {
				part = strings.TrimSpace(part)
				if v, ok := strings.CutPrefix(part, "go "); ok {
					l.vendGo[mod] = l.capVersion("vendor/modules.txt (module "+mod+")", "go"+v)
				}
				if part == "explicit" && cur.Path != "" {
					m := l.vendor.meta[cur]
					m.explicit = true
					l.vendor.meta[cur] = m
				}
			}
		case cur.Path != "":
			// A package of the module: the module is in the build list.
			if f := strings.Fields(line); len(f) == 1 && module.CheckImportPath(f[0]) == nil {
				l.vendor.addPackage(cur)
			}
		}
	}
	return nil
}

// vendorList is vendor/modules.txt as the go command reads it (readVendorList
// of cmd/go): each "# path version [=> new [version]]" or "# path => new
// [version]" line is a module, with the "## explicit" mark of the next line,
// and the lines below it name its packages.
type vendorList struct {
	// mods are the modules with at least one package, in their order: the
	// build list (vendorList of cmd/go). A module line without a package
	// (a module that go.mod requires for its go.mod only) is not in it.
	mods []module.Version
	// replaced are the modules whose line has a replacement, in their
	// order (vendorReplaced of cmd/go).
	replaced []module.Version
	meta     map[module.Version]vendorMeta
	listed   map[module.Version]bool // in mods
}

// vendorMeta is what modules.txt says of a module: whether go.mod requires
// it ("## explicit"), and the module that replaces it, if any.
type vendorMeta struct {
	explicit    bool
	replacement module.Version
}

// add records the module of a "# " line, from its fields, and returns it:
// the zero module when the line is not one the go command understands (a
// path alone, or a version that is not a semantic version). "# path => new
// [version]" is a replacement of every version of path (a replace without a
// version in go.mod).
func (v *vendorList) add(f []string) module.Version {
	if len(f) < 2 {
		return module.Version{}
	}
	mod := module.Version{Path: f[0]}
	switch {
	case semver.IsValid(f[1]):
		mod.Version = f[1]
		f = f[2:]
	case f[1] == "=>":
		f = f[1:]
	default:
		return module.Version{}
	}
	if len(f) >= 2 && f[0] == "=>" {
		meta := v.meta[mod]
		switch {
		case len(f) == 2: // a directory
			meta.replacement = module.Version{Path: f[1]}
			v.replaced = append(v.replaced, mod)
		case len(f) == 3 && semver.IsValid(f[2]):
			meta.replacement = module.Version{Path: f[1], Version: f[2]}
			v.replaced = append(v.replaced, mod)
		}
		v.meta[mod] = meta
	}
	return mod
}

// addPackage records that module mod has a package: it is in the build list.
func (v *vendorList) addPackage(mod module.Version) {
	if !v.listed[mod] {
		v.listed[mod] = true
		v.mods = append(v.mods, mod)
	}
}

// checkVendor refuses the load when go.mod and vendor/modules.txt of the
// main module do not agree, as go build does ("inconsistent vendoring",
// checkVendorConsistency of cmd/go), unless the policy ignores it. The
// checks are the ones of the go command: every module required in go.mod is
// marked explicit in modules.txt, every explicit module of modules.txt that
// has a package is required in go.mod, and the replacements match. A go.mod
// older than go 1.14 is only checked for a version that differs (modules.txt
// had no explicit marks then); a go.mod without a go directive is read as go
// 1.16, as the go command reads it. A module without a vendor directory is
// not checked (the go command would then read the module cache, which yaegi v2
// does not have: a missing package is refused when it is imported).
func (l *srcLoader) checkVendor() error {
	if l.pol.ignoreVendor || l.gomod == nil {
		return nil
	}
	if st, err := os.Stat(filepath.Join(l.modRoot, "vendor")); err != nil || !st.IsDir() {
		return nil
	}
	v := l.vendor
	if v == nil {
		v = &vendorList{meta: map[module.Version]vendorMeta{}}
	}
	pre114 := versionLess(goModVersion(l.gomod.File), "go1.14")
	version := map[string]string{} // path -> vendored version
	for _, m := range v.mods {
		if m.Version != "" {
			version[m.Path] = m.Version
		}
	}
	var errs []string
	errorf := func(m module.Version, format string, args ...any) {
		errs = append(errs, "\t"+describeModule(m)+": "+fmt.Sprintf(format, args...))
	}
	required := map[module.Version]bool{}
	for _, r := range l.gomod.Require {
		required[r.Mod] = true
		if v.meta[r.Mod].explicit {
			continue
		}
		if pre114 {
			if vv, ok := version[r.Mod.Path]; ok && vv != r.Mod.Version {
				errorf(r.Mod, "is explicitly required in go.mod, but vendor/modules.txt indicates %s@%s", r.Mod.Path, vv)
			}
			continue
		}
		errorf(r.Mod, "is explicitly required in go.mod, but not marked as explicit in vendor/modules.txt")
	}
	if l.gomod.lax {
		// The replace directives are not read (see modFile): they cannot
		// be compared.
		if len(errs) == 0 {
			return nil
		}
		return l.vendorError(errs)
	}
	// Every replacement of go.mod is checked, also one that applies to no
	// vendored module: the go.mod of the replacement changes the versions of
	// the other modules.
	replaced := map[module.Version]module.Version{}
	for _, r := range l.gomod.Replace {
		if _, seen := replaced[r.Old]; seen {
			continue
		}
		replaced[r.Old] = r.New
		if r.Old.Path == l.modPath {
			continue // the main module is never replaced
		}
		vr := v.meta[r.Old].replacement
		switch {
		case vr == (module.Version{}):
			if pre114 && (r.Old.Version == "" || version[r.Old.Path] != r.Old.Version) {
				// Before go 1.14, modules.txt left out the replacements of
				// every version and of the modules outside the build list.
				continue
			}
			errorf(r.Old, "is replaced in go.mod, but not marked as replaced in vendor/modules.txt")
		case vr != r.New:
			errorf(r.Old, "is replaced by %s in go.mod, but marked as replaced by %s in vendor/modules.txt", describeModule(r.New), describeModule(vr))
		}
	}
	for _, m := range v.mods {
		if v.meta[m].explicit && !required[m] {
			errorf(m, "is marked as explicit in vendor/modules.txt, but not explicitly required in go.mod")
		}
	}
	for _, m := range v.replaced {
		// A replacement of this version, or of every version of the path.
		// When go.mod has one, the loop above has compared the two.
		if _, ok := replaced[m]; ok {
			continue
		}
		if _, ok := replaced[module.Version{Path: m.Path}]; ok {
			continue
		}
		errorf(m, "is marked as replaced in vendor/modules.txt, but not replaced in go.mod")
	}
	if len(errs) == 0 {
		return nil
	}
	return l.vendorError(errs)
}

// vendorError is the error of checkVendor, with its list of differences.
func (l *srcLoader) vendorError(errs []string) error {
	return loadErrorf("vm load: inconsistent vendoring in %s:\n%s\n\tgo build refuses it the same way: run go mod vendor, or set Options.IgnoreInconsistentVendoring to read the vendor directory as it is", l.modRoot, strings.Join(errs, "\n"))
}

// describeModule returns "path@version", or the path alone without a
// version.
func describeModule(m module.Version) string {
	if m.Version == "" {
		return m.Path
	}
	return m.Path + "@" + m.Version
}

// goModVersion returns the go directive of a go.mod as a "go1.21" version:
// go1.16 without a directive, as the go command reads such a file
// (gover.DefaultGoModVersion).
func goModVersion(f *modfile.File) string {
	if f.Go == nil {
		return "go1.16"
	}
	return "go" + f.Go.Version
}

// goVersionOf returns the go directive that applies to package path.
func (l *srcLoader) goVersionOf(path string) string {
	best, v := "", ""
	for mod, gv := range l.vendGo {
		if (path == mod || strings.HasPrefix(path, mod+"/")) && len(mod) > len(best) {
			best, v = mod, gv
		}
	}
	if best == "" || l.isOwn(path) {
		return l.goVer
	}
	if v == "" {
		// Modules older than go 1.17 have no version line: gc uses go1.16.
		return "go1.16"
	}
	return v
}

// info is the srcInfo of package path for the AOT version check: the
// vendored module of the longest matching path in vendor/modules.txt, the
// directory of the package and the files check compiles. A package of the
// plugin's own module has no module.
func (l *srcLoader) info(path string) (srcInfo, bool) {
	dir, err := l.dirOf(path)
	if err != nil {
		return srcInfo{}, false
	}
	si := srcInfo{dir: dir}
	if bp, err := l.importDir(dir); err == nil {
		si.files = bp.GoFiles
	}
	if l.goPathSrc != "" || l.isOwn(path) {
		// No module (GOPATH mode, or the plugin's own module): the
		// package is interpreted, never run as an AOT copy.
		return si, true
	}
	for mod, vm := range l.vendMod {
		if (path == mod || strings.HasPrefix(path, mod+"/")) && len(mod) > len(si.module) {
			si.module, si.version, si.replaced = mod, vm.version, vm.replaced
		}
	}
	return si, true
}

// isOwn reports whether package path is in the main module: its path is in
// the module path, and its directory is not in a nested module (a directory
// with its own go.mod below the module root, which Go excludes from the
// module).
func (l *srcLoader) isOwn(path string) bool {
	rel, ok := strings.CutPrefix(path, l.modPath)
	if !ok || rel != "" && rel[0] != '/' {
		return false
	}
	d := filepath.Join(l.modRoot, filepath.FromSlash(rel))
	for ; d != l.modRoot && len(d) > len(l.modRoot); d = filepath.Dir(d) {
		has, ok := l.nested[d]
		if !ok {
			st, err := os.Stat(filepath.Join(d, "go.mod"))
			has = err == nil && !st.IsDir()
			l.nested[d] = has
		}
		if has {
			return false
		}
	}
	return true
}

// dirOf returns the directory of a non-standard package: in the main module,
// or in its vendor directory. With Options.ImportFromArchive, like yaegi
// (and yaegi v2 before nested modules), a package that is in neither but whose
// path is in the module of the plugin archive is read from its directory in
// the archive: a nested plugin module that does not vendor a package of the
// module around it. The go command refuses that layout, and so does the
// loader without the option.
// The import path must be valid (no ".." elements, as go list requires) and
// the directory must not leave the plugin archive through a symbolic link.
func (l *srcLoader) dirOf(path string) (string, error) {
	if err := module.CheckImportPath(path); err != nil {
		return "", err
	}
	var d string
	if l.goPathSrc != "" {
		d = l.goPathDir(path)
	} else if l.isOwn(path) {
		d = filepath.Join(l.modRoot, filepath.FromSlash(strings.TrimPrefix(path, l.modPath)))
	} else {
		d = filepath.Join(l.modRoot, "vendor", filepath.FromSlash(path))
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			rel, ok := strings.CutPrefix(path, l.archPath)
			if ok && (rel == "" || rel[0] == '/') && !l.pol.importFromArchive {
				return "", fmt.Errorf("cannot find package %s: not in module %s nor in its vendor directory (it is in the module %s of the plugin archive, around this one: go build refuses this layout, Options.ImportFromArchive reads it from %s)",
					path, l.modPath, l.archPath, filepath.Join(l.archRoot, filepath.FromSlash(rel)))
			}
			if !ok || rel != "" && rel[0] != '/' {
				if l.modAbove {
					// The go.mod is above Options.Dir: often the go.mod
					// of another project, and the plugin has none.
					return "", fmt.Errorf("cannot find package %s: not in module %s nor in its vendor directory (the go.mod used is %s, above the plugin directory %s: does the plugin miss its own go.mod? Options.Root stops the search for go.mod)",
						path, l.modPath, filepath.Join(l.archRoot, "go.mod"), l.dir)
				}
				return "", fmt.Errorf("cannot find package %s: not in module %s nor in its vendor directory", path, l.modPath)
			}
			d = filepath.Join(l.archRoot, filepath.FromSlash(rel))
		}
	}
	real, err := filepath.EvalSymlinks(d)
	if err != nil {
		return "", fmt.Errorf("cannot find package %s: %v", path, err)
	}
	if rel, err := filepath.Rel(l.realRoot, real); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		if l.goPathSrc != "" {
			return "", fmt.Errorf("package %s: %s is outside of %s (Options.GoPath)", path, real, l.goPathSrc)
		}
		return "", fmt.Errorf("package %s: %s is outside the module %s", path, real, l.archRoot)
	}
	return d, nil
}

// Import implements types.Importer.
func (l *srcLoader) Import(path string) (*types.Package, error) {
	return l.ImportFrom(path, "", 0)
}

// ImportFrom implements types.ImporterFrom: go/types calls it with the
// directory of the file that imports path. Only GOPATH mode uses it: there,
// the vendor directories that apply depend on that directory.
func (l *srcLoader) ImportFrom(path, dir string, _ types.ImportMode) (*types.Package, error) {
	if l.std.Has(path) {
		return l.std.Import(path)
	}
	if l.goPathSrc != "" {
		var err error
		if path, err = l.goPathResolve(path, dir); err != nil {
			return nil, err
		}
	}
	p, err := l.load(path)
	if err != nil {
		return nil, err
	}
	return p.types, nil
}

func (l *srcLoader) load(path string) (*srcPkg, error) {
	if p := l.pkgs[path]; p != nil {
		if !p.done {
			return nil, fmt.Errorf("import cycle through %s", path)
		}
		return p, p.err
	}
	if err := ctxErr(l.lctx); err != nil {
		return nil, err
	}
	if err := l.budget.addPackage(); err != nil {
		return nil, err
	}
	p := &srcPkg{path: path}
	l.pkgs[path] = p
	p.err = l.check(p)
	p.done = true
	if p.err == nil {
		l.order = append(l.order, p)
	}
	return p, p.err
}

func (l *srcLoader) check(p *srcPkg) error {
	dir, err := l.dirOf(p.path)
	if err != nil {
		return err
	}
	bp, err := l.importDir(dir)
	if err != nil {
		var multi *build.MultiplePackageError
		if errors.As(err, &multi) && !l.pol.ignoreTests && (strings.HasSuffix(multi.Files[0], "_test.go") || strings.HasSuffix(multi.Files[1], "_test.go")) {
			return fmt.Errorf("%s: %v\n\tgo build refuses the package the same way; Options.IgnoreTestFiles leaves out the test files (an option for plugins written for yaegi, see Options)", p.path, err)
		}
		return fmt.Errorf("%s: %v", p.path, err)
	}
	for _, name := range bp.GoFiles {
		f, err := l.parseFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("%s: %v", p.path, err)
		}
		p.files = append(p.files, f)
	}
	l.noteYaegiFiles(p.path, dir, bp)
	if l.pol.dupDecls {
		imp := l.importerFrom(dir)
		dropDupDecls(l.fset, p.files, func(path string) string {
			if tp := imp(path); tp != nil {
				return tp.Name()
			}
			return "" // go/types reports the error
		})
	}
	// go/types and go/constant cannot be canceled: refuse the sources that
	// make them run for minutes or allocate gigabytes (loadbomb.go).
	if err := checkSourceSize(l.fset, p.files, l.importerFrom(dir), &l.budget.constBytes); err != nil {
		return l.budget.trip(err)
	}
	p.info = &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Implicits: map[ast.Node]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Scopes: map[ast.Node]*types.Scope{},
		Instances: map[*ast.Ident]types.Instance{}, FileVersions: map[*ast.File]string{},
	}
	var errs []error
	head := -1 // index in errs of the last error, -1 when it was left out (with its parts)
	// accepts names the options of Options that would accept every error,
	// for the message: a plugin refused by default says which option
	// accepts it. Empty when an error is one no option accepts.
	accepts := typeErrPolicy{unusedNames: true, unusedExprs: true}
	var hint optionHint
	conf := types.Config{
		Importer:  l,
		Sizes:     types.SizesFor("gc", runtime.GOARCH),
		GoVersion: l.goVersionOf(p.path),
		Error: func(err error) {
			if te, ok := err.(types.Error); ok && isSubError(te.Msg) {
				// a part of the error before it (suberr.go)
				if head >= 0 {
					if h, ok := errs[head].(types.Error); ok {
						joinTypeError(&h, te)
						errs[head] = h
					}
				}
				return
			}
			head = -1
			if l.pol.ignore.ignoresErr(err) {
				return
			}
			name := accepts.option(err)
			if name == "" && !l.pol.dupDecls && isDupDeclErr(err) {
				name = "IgnoreDuplicateDecls (identical copies of a declaration only)"
			}
			hint.add(name)
			if len(errs) < 10 {
				head = len(errs)
				errs = append(errs, err)
			}
		},
	}
	p.types, _ = conf.Check(p.path, l.fset, p.files, p.info)
	if len(errs) > 0 {
		return hint.wrap(errors.Join(errs...))
	}
	return nil
}

// optionHint names the options of Options that accept every error of a
// load, for the message of the error (see check).
type optionHint struct {
	names map[string]bool
	none  bool // an error that no option accepts
}

// add records the option that accepts one error: "" when none does.
func (h *optionHint) add(name string) {
	if name == "" {
		h.none = true
		return
	}
	if h.names == nil {
		h.names = map[string]bool{}
	}
	h.names[name] = true
}

// wrap adds the options to err when they accept every error.
func (h *optionHint) wrap(err error) error {
	if h.none || len(h.names) == 0 {
		return err
	}
	names := make([]string, 0, len(h.names))
	for n := range h.names {
		names = append(names, "Options."+n)
	}
	sort.Strings(names)
	return fmt.Errorf("%w\n\tgo build refuses the package the same way; %s accepts it (an option for plugins written for yaegi, see Options)", err, strings.Join(names, " and "))
}

// importDir selects the files of the package in dir, as go run does. The
// VM cannot run assembly: when this selection has assembly files or
// functions without body, the package is selected again with the purego
// and noasm tags, which give the pure Go version of packages that also have
// one in assembly, as in the standard library: x/crypto, x/sys/cpu and
// xxhash use purego, klauspost/compress (s2, zstd, huff0) uses noasm.
// Other packages keep the files of go run, also when they have purego
// variants (the go list loader and the AOT copies select the same files).
func (l *srcLoader) importDir(dir string) (*build.Package, error) {
	if bp := l.bps[dir]; bp != nil {
		return bp, nil
	}
	bp, err := l.ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	if len(bp.SFiles) > 0 || l.hasBodyless(dir, bp.GoFiles) {
		ctx := l.ctx
		ctx.BuildTags = append(ctx.BuildTags[:len(ctx.BuildTags):len(ctx.BuildTags)], "purego", "noasm")
		if pbp, err := ctx.ImportDir(dir, 0); err == nil {
			bp = pbp
			l.purego[dir] = true
		}
	}
	l.bps[dir] = bp
	return bp, nil
}

// noteYaegiFiles adds a note when yaegi selected other files than bp in
// package path (yaegifiles.go). The yaegi side starts with the tags yaegi
// had (yaegiStartTags), plus purego and noasm when importDir added them:
// only the rule that reads the constraints differs.
func (l *srcLoader) noteYaegiFiles(path, dir string, bp *build.Package) {
	var extra []string
	if l.purego[dir] {
		extra = []string{"purego", "noasm"}
	}
	tags := yaegiStartTags(path == l.pluginPath, l.pluginTags, extra)
	names := baseNames(bp.GoFiles, bp.CgoFiles, bp.IgnoredGoFiles, bp.InvalidGoFiles)
	yaegi := newYaegiSelect().files(names, tags, func(name string) ([]*ast.CommentGroup, bool) {
		if f := l.parsed[filepath.Join(dir, name)]; f != nil {
			return headerOf(f), true
		}
		return readHeader(filepath.Join(dir, name))
	})
	if n := yaegiFilesNote(path, bp.GoFiles, yaegi); n != "" {
		l.notes = append(l.notes, n)
	}
}

// hasBodyless reports whether one of the files declares a function without
// body (written in assembly, or linked with go:linkname).
func (l *srcLoader) hasBodyless(dir string, names []string) bool {
	for _, name := range names {
		f, err := l.parseFile(filepath.Join(dir, name))
		if err != nil {
			continue // check reports it
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body == nil {
				return true
			}
		}
	}
	return false
}

// parseFile parses a Go file once. A symbolic link is followed only when
// its target is a regular file in the plugin archive (see openModuleFile).
// The load budget counts the size of the target.
func (l *srcLoader) parseFile(name string) (*ast.File, error) {
	if f := l.parsed[name]; f != nil {
		return f, nil
	}
	file, err := openModuleFile(l.realRoot, name)
	if file == nil && err == nil {
		err = fmt.Errorf("%s: no such file", name)
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := l.budget.addFile(st.Size()); err != nil {
		return nil, err
	}
	// Read at most the counted size: a file that grows after the check is
	// not read past the budget.
	src, err := io.ReadAll(io.LimitReader(file, st.Size()+1))
	if err != nil {
		return nil, err
	}
	if int64(len(src)) > st.Size() {
		return nil, fmt.Errorf("%s changed while it was read", name)
	}
	// Without SkipObjectResolution: checkSourceSize (loadbomb.go) follows the
	// local names to their declarations.
	f, err := parser.ParseFile(l.fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, scanErrors(err)
	}
	l.parsed[name] = f
	return f, nil
}

// stdClosure returns the standard library packages the source packages
// import, directly or not, sorted by path: SSA needs a package for every
// package whose objects the program refers to, not for the whole library.
func stdClosure(order []*srcPkg) []*types.Package {
	seen := map[*types.Package]bool{}
	var out []*types.Package
	var visit func(*types.Package)
	visit = func(tp *types.Package) {
		if seen[tp] || tp == types.Unsafe {
			return
		}
		seen[tp] = true
		out = append(out, tp)
		for _, imp := range tp.Imports() {
			visit(imp)
		}
	}
	src := map[*types.Package]bool{}
	for _, p := range order {
		src[p.types] = true
	}
	for _, p := range order {
		for _, imp := range p.types.Imports() {
			if !src[imp] {
				visit(imp)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path() < out[j].Path() })
	return out
}

// sourceLoad is the result of loadSource.
type sourceLoad struct {
	prog      *ssa.Program
	root      *ssa.Package
	pkgs      []*ssa.Package               // source packages, dependencies first
	stdSrc    []*ssa.Package               // standard library packages built from embedded sources
	src       []sourcePkg                  // the source packages, for the restrictions
	info      func(string) (srcInfo, bool) // for the AOT version check
	notes     []string                     // Plugin.Notes
	loadTime  time.Duration                // parse and type-check (and the first decoding of stdtypes)
	buildTime time.Duration                // SSA construction
}

// loadSource loads package pkgPath of the module holding dir (or of the
// nested module of dir that holds pkgPath, see mainModule), and builds the
// SSA of its source packages. The standard library packages are created from
// their types only (their functions are called natively). pol says what the
// loader accepts beyond go build (the yaegi compatibility options). limit is
// Options.Root (see findModule), goPath is Options.GoPath (GOPATH mode for a
// plugin without go.mod, gopath.go).
func loadSource(ctx context.Context, dir, limit, goPath, pkgPath string, limits LoadLimits, pol loadPolicy) (*sourceLoad, error) {
	t0 := time.Now()
	std, err := stdtypes.Load()
	if err != nil {
		return nil, err
	}
	l, err := newSrcLoader(ctx, dir, limit, goPath, pkgPath, std, newLoadBudget(limits))
	if err != nil {
		return nil, err
	}
	l.pol = pol
	if err := l.checkNewerGo(); err != nil {
		return nil, err
	}
	if err := l.checkVendor(); err != nil {
		return nil, err
	}
	if pol.ignoreTests {
		l.ctx.ReadDir = readDirNoTests
	}
	// The build tags of the "// yaegi:tags t1 t2" lines of the plugin
	// package: the yaegi side of the notes always has them (yaegi reads
	// them, in this package and in the other packages, in the order it loads
	// them); the loader selects its files with them only with
	// Options.YaegiTags (go build ignores these lines).
	l.pluginPath = pkgPath
	if d, err := l.dirOf(pkgPath); err == nil {
		l.pluginTags = yaegiTags("", d, l.ctx.OpenFile)
		if pol.yaegiTags {
			l.ctx.BuildTags = append(l.ctx.BuildTags, l.pluginTags...)
		}
	}
	if _, err := l.load(pkgPath); err != nil {
		if l.budget.tripped != nil {
			return nil, l.budget.tripped // unwrapped by go/types otherwise
		}
		if !pol.yaegiTags && len(l.pluginTags) > 0 {
			// The error may come from a file that the tags select.
			err = fmt.Errorf("%w\n\tthe plugin has a \"// yaegi:tags %s\" line, which go build does not read: Options.YaegiTags selects the files with these build tags, as yaegi does", err, strings.Join(l.pluginTags, " "))
		}
		return nil, err
	}
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	// The SSA builder cannot be canceled either: refuse the generic code
	// that makes it create too many or too large instances (loadinst.go).
	if err := checkInstances(l.fset, l.order); err != nil {
		return nil, err
	}
	sort.Strings(l.notes) // the order of the go list loader
	r := &sourceLoad{loadTime: time.Since(t0), info: l.info, notes: l.notes}
	t1 := time.Now()
	r.prog = ssa.NewProgram(l.fset, ssa.InstantiateGenerics)
	r.prog.CreatePackage(types.Unsafe, nil, nil, true)
	var stdSrc []*ssa.Package
	defer func() { r.stdSrc = stdSrc }()
	for _, tp := range stdClosure(l.order) {
		if s := std.Source(tp.Path()); s != nil {
			// Bodies for the instances of its generic functions,
			// interpreted (isInterp); the rest stays native.
			stdSrc = append(stdSrc, r.prog.CreatePackage(tp, s.Files, s.Info, true))
			continue
		}
		r.prog.CreatePackage(tp, nil, nil, true)
	}
	for _, sp := range stdSrc {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		sp.Build()
	}
	for _, p := range l.order {
		r.src = append(r.src, sourcePkg{fset: l.fset, types: p.types, files: p.files, info: p.info})
		sp := r.prog.CreatePackage(p.types, p.files, p.info, true)
		r.pkgs = append(r.pkgs, sp)
		if p.path == pkgPath {
			r.root = sp
		}
	}
	for _, sp := range r.pkgs {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		sp.Build()
	}
	r.buildTime = time.Since(t1)
	return r, nil
}
