package vm

// GOPATH mode of the default loader (Options.GoPath): a local plugin without
// a go.mod is read from <GoPath>/src, as yaegi reads Traefik local plugins.
// The go command no longer has this mode by default (GO111MODULE=on), so it
// is off unless the host sets GoPath.
//
// An import path P of a package in directory D of <GoPath>/src is resolved
// as in Go's GOPATH mode (and as yaegi does, in the usual layouts):
//
//   - D/vendor/P, then the vendor directory of each parent of D, up to
//     <GoPath>/src/vendor/P;
//   - then <GoPath>/src/P.
//
// The standard library comes first, as in module mode. A package found in a
// vendor directory below <GoPath>/src/<X> has the import path X/vendor/P,
// as in Go: two vendored copies of P are two packages. A package of
// <GoPath>/src/vendor keeps the path P, as in yaegi.
//
// Nothing is read outside of <GoPath>/src: import paths with ".." or a
// vendor element are refused, a directory that leaves <GoPath>/src through
// a symbolic link is refused (dirOf), and every file is read with the rule
// of the module files, with <GoPath>/src in place of the module
// (openModuleFile): a link is followed only when its target is a regular
// file in <GoPath>/src, as in a Kubernetes ConfigMap volume.
//
// A package found this way has no module version: it is always interpreted,
// never run as an AOT copy (info). The Go version of the language is the
// one of the host, as go build uses in GOPATH mode.

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/types"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/module"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// modLimit is the directory above which the loaders never look for a
// go.mod: Options.Root, or <GoPath>/src when only GoPath is set.
func (opts Options) modLimit() string {
	if opts.Root != "" || opts.GoPath == "" {
		return opts.Root
	}
	return filepath.Join(opts.GoPath, "src")
}

// newGoPathLoader returns the loader of package pkgPath of the plugin in dir,
// which has no go.mod, in GOPATH mode: dir must be in <goPath>/src.
func newGoPathLoader(ctx context.Context, dir, goPath, pkgPath string, std *stdtypes.Universe, budget *loadBudget) (*srcLoader, error) {
	gp, err := filepath.Abs(goPath)
	if err != nil {
		return nil, err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	// underRoot compares the paths as given, then without their symbolic
	// links; src is then the path the loader builds every directory from.
	absDir, src, _, err := underRoot(absDir, filepath.Join(gp, "src"))
	if err != nil {
		return nil, fmt.Errorf("GOPATH mode (Options.GoPath): %v", err)
	}
	realSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		return nil, err
	}
	bctx := build.Default
	bctx.GOOS, bctx.GOARCH = runtime.GOOS, runtime.GOARCH
	bctx.GOROOT, bctx.GOPATH = "", ""
	bctx.CgoEnabled = false
	bctx.Dir = src
	// Every file is read with the rule of the module files, with
	// <GoPath>/src in place of the module (openModuleFile, parseFile).
	bctx.OpenFile = moduleOpenFile(realSrc, budget)
	l := &srcLoader{realRoot: realSrc, budget: budget, lctx: ctx,
		fset: std.Fset, std: std, ctx: bctx, modRoot: src, archRoot: src, dir: absDir,
		goPathSrc: src, nested: map[string]bool{},
		goVer: std.GoVersion, vendGo: map[string]string{}, vendMod: map[string]vendMod{}, pkgs: map[string]*srcPkg{},
		bps: map[string]*build.Package{}, parsed: map[string]*ast.File{}, purego: map[string]bool{},
	}
	if _, err := l.goPathResolve(pkgPath, ""); err != nil {
		return nil, err
	}
	return l, nil
}

// goPathResolve returns the package path of import path in GOPATH mode,
// for a file in directory from ("" for the plugin package itself): the
// path of the nearest vendored copy (X/vendor/path), or path itself. dirOf
// then gives its directory.
func (l *srcLoader) goPathResolve(path, from string) (string, error) {
	if err := module.CheckImportPath(path); err != nil {
		return "", fmt.Errorf("import %q: %v", path, err)
	}
	if hasVendorElem(path) {
		return "", fmt.Errorf("import %q: an import path cannot have a vendor element", path)
	}
	if from == "" {
		// The plugin package itself. An import of a standard library path
		// gets the standard library (ImportFrom): the plugin cannot be one
		// either, as go build in GOPATH mode builds GOROOT's fmt, never
		// <GoPath>/src/fmt.
		if l.std.Has(path) {
			return "", fmt.Errorf("package %s is in the standard library: it cannot be loaded from %s (Options.GoPath)", path, l.goPathSrc)
		}
		return path, nil
	}
	rel, ok := relBelow(l.goPathSrc, from)
	if !ok {
		return "", fmt.Errorf("import %q from %s: outside of %s", path, from, l.goPathSrc)
	}
	// <GoPath>/src/vendor itself is read by dirOf, for every importer.
	for sub := filepath.ToSlash(rel); sub != "."; sub = pathpkg.Dir(sub) {
		cand := sub + "/vendor/" + path
		if st, err := os.Stat(filepath.Join(l.goPathSrc, filepath.FromSlash(cand))); err == nil && st.IsDir() {
			return cand, nil
		}
	}
	return path, nil
}

// goPathDir returns the directory of package path (a result of
// goPathResolve) in GOPATH mode, before the check of dirOf.
func (l *srcLoader) goPathDir(path string) string {
	if !hasVendorElem(path) {
		d := filepath.Join(l.goPathSrc, "vendor", filepath.FromSlash(path))
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return filepath.Join(l.goPathSrc, filepath.FromSlash(path))
}

// hasVendorElem reports whether import path has a "vendor" element.
func hasVendorElem(path string) bool {
	for _, el := range strings.Split(path, "/") {
		if el == "vendor" {
			return true
		}
	}
	return false
}

// importerFrom returns the import function of the checks of the package in
// directory dir (checkSourceSize, dropDupDecls): in GOPATH mode, an import
// resolves from the directory of the file that has it.
func (l *srcLoader) importerFrom(dir string) func(string) *types.Package {
	return func(path string) *types.Package {
		tp, err := l.ImportFrom(path, dir, 0)
		if err != nil {
			return nil
		}
		return tp
	}
}
