package vm

// Version check of the AOT copies of non-standard packages.
//
// An AOT copy (aot/...) is generated from one version of a library: the one
// of yaegi v2's go.mod. A plugin may vendor another version of the same
// library, or a fork. Choosing the copy by import path only would silently
// run yaegi v2's version instead of the plugin's code, and a function that
// exists only in the plugin's version would pass type checking and then
// fail at request time ("not in registry").
//
// So aotgen records in each registration (Package) the module and the
// version the copy was generated from, and a hash of the sources of every
// non-standard package the copy was built from (srchash.Dir). At load, a
// package that would run in AOT mode keeps its copy only when the plugin
// has the same module at the same version, not replaced, and the same
// sources for each of these packages. Otherwise the package is interpreted
// from the plugin's own sources, and the reason is kept in
// Machine.AOTFallback. The standard library copies are not checked here.

import (
	"fmt"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/traefik/yaegi/v2/internal/srchash"
)

// srcInfo is what the version check needs to know of a package of the
// plugin: its module ("" for the plugin's own module or an unknown one), the
// module version, whether the module is replaced, its directory, and the
// names of the files of the directory that the loader compiles.
type srcInfo struct {
	module, version string
	replaced        bool
	dir             string
	files           []string
}

// srcHash is the srchash.DirFiles of a directory: the hash, the hashed
// files, or the error.
type srcHash struct {
	hash  string
	files map[string]bool
	err   error
}

// aotFallbacks returns modes with ModeInterp for each non-standard path
// whose mode is ModeAOT but whose AOT copy does not match the plugin's
// version of the package (see aotMismatch), and the reason for each one.
// modes itself is not changed.
func aotFallbacks(modes map[string]Mode, aot bool, paths []string, info func(string) (srcInfo, bool)) (map[string]Mode, map[string]string) {
	var why map[string]string
	hashes := map[string]*srcHash{}
	out := modes
	sort.Strings(paths)
	for _, path := range paths {
		if isStdPath(path) || ModeFor(modes, aot, path) != ModeAOT {
			continue
		}
		r := aotMismatch(aotPkg(path), info, hashes)
		if r == "" {
			continue
		}
		if why == nil {
			why = map[string]string{}
			out = make(map[string]Mode, len(modes)+1)
			for k, v := range modes {
				out[k] = v
			}
		}
		out[path] = ModeInterp
		why[path] = r
	}
	return out, why
}

// aotMismatch returns why the AOT copy p cannot run the plugin's version of
// its package, or "" when it can. hashes caches srchash.DirFiles per
// directory.
func aotMismatch(p *Package, info func(string) (srcInfo, bool), hashes map[string]*srcHash) string {
	if p.Module == "" || len(p.Sources) == 0 {
		return "the AOT copy has no module version (regenerate it with scripts/aotgen.sh)"
	}
	si, ok := info(p.Path)
	switch {
	case !ok || si.module == "":
		return fmt.Sprintf("the plugin's %s is not from module %s", p.Path, p.Module)
	case si.module != p.Module:
		return fmt.Sprintf("the plugin's %s is from module %s, the AOT copy from %s", p.Path, si.module, p.Module)
	case si.replaced:
		return fmt.Sprintf("the plugin replaces module %s (go.mod replace directive)", p.Module)
	case si.version != p.Version:
		return fmt.Sprintf("the plugin uses %s %s, the AOT copy was generated from %s", p.Module, si.version, p.Version)
	}
	deps := make([]string, 0, len(p.Sources))
	for q := range p.Sources {
		deps = append(deps, q)
	}
	sort.Strings(deps)
	for _, q := range deps {
		qi, ok := info(q)
		if !ok || qi.dir == "" {
			return fmt.Sprintf("the AOT copy of %s was built with %s, which the plugin does not have", p.Path, q)
		}
		h := hashes[qi.dir]
		if h == nil {
			hash, files, err := srchash.DirFiles(qi.dir)
			h = &srcHash{hash: hash, files: map[string]bool{}, err: err}
			for _, f := range files {
				h.files[f] = true
			}
			hashes[qi.dir] = h
		}
		if h.err != nil {
			return fmt.Sprintf("cannot hash the plugin's sources of %s: %v", q, h.err)
		}
		if h.hash != p.Sources[q] {
			return fmt.Sprintf("the plugin's sources of %s differ from the ones of the AOT copy of %s (fork or edited vendor directory)", q, p.Path)
		}
		// The hash must cover every file the loader compiles. srchash leaves
		// out the same files as go/build, so this only fails if they ever
		// read a build constraint differently.
		for _, f := range qi.files {
			if !h.files[f] {
				return fmt.Sprintf("the plugin's file %s of %s is compiled but not in the source hash", f, q)
			}
		}
	}
	return ""
}

// goListInfo is the srcInfo of the packages loaded by go list.
func goListInfo(pkgs []*packages.Package) func(string) (srcInfo, bool) {
	byPath := map[string]*packages.Package{}
	packages.Visit(pkgs, nil, func(p *packages.Package) { byPath[p.PkgPath] = p })
	return func(path string) (srcInfo, bool) {
		p := byPath[path]
		if p == nil {
			return srcInfo{}, false
		}
		si := srcInfo{dir: p.Dir}
		if si.dir == "" && len(p.GoFiles) > 0 {
			si.dir = filepath.Dir(p.GoFiles[0])
		}
		for _, f := range p.GoFiles {
			if filepath.Dir(f) == si.dir {
				f = filepath.Base(f)
			}
			si.files = append(si.files, f) // a file of another directory is never in the hash
		}
		if mod := p.Module; mod != nil && !mod.Main {
			si.module, si.version, si.replaced = mod.Path, mod.Version, mod.Replace != nil
		}
		return si, true
	}
}

// checkAOTSymbols makes sure that every package-level function, variable
// and type of a non-standard AOT package that the interpreted packages of
// src use is in the AOT symbol table: a missing one would only fail at its
// first use, during a request ("not in registry").
func checkAOTSymbols(modes map[string]Mode, aot bool, src []sourcePkg) error {
	for _, sp := range src {
		if ModeFor(modes, aot, sp.types.Path()) == ModeAOT || sp.info == nil {
			continue
		}
		var missing []string
		seen := map[types.Object]bool{}
		for id, obj := range sp.info.Uses {
			pkg := obj.Pkg()
			if pkg == nil || seen[obj] || obj.Parent() != pkg.Scope() || isStdPath(pkg.Path()) || ModeFor(modes, aot, pkg.Path()) != ModeAOT {
				continue
			}
			seen[obj] = true
			if !aotHasSymbol(aotPkg(pkg.Path()), obj) {
				missing = append(missing, fmt.Sprintf("%s: %s.%s", sp.fset.Position(id.Pos()), pkg.Path(), obj.Name()))
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("%s: not in the AOT copy (regenerate aot/ or run the package interpreted)", strings.Join(missing, ", "))
		}
	}
	return nil
}

// aotHasSymbol reports whether the AOT copy p implements obj. Constants,
// generic functions and generic types are not in the table: constants are
// compiled in, and generic code is interpreted from source.
func aotHasSymbol(p *Package, obj types.Object) bool {
	key := p.Path + "." + obj.Name()
	switch o := obj.(type) {
	case *types.Func:
		if o.Type().(*types.Signature).TypeParams() != nil {
			return true
		}
		_, ok := p.Funcs[o.Name()]
		if !ok {
			_, ok = nativeFuncs[key]
		}
		if !ok {
			_, ok = nativeHooks[key]
		}
		return ok
	case *types.Var:
		_, ok := p.Vars[o.Name()]
		if !ok {
			_, ok = nativeVars[key]
		}
		return ok
	case *types.TypeName:
		switch t := o.Type().(type) {
		case *types.Alias:
			return true // its target is checked where it is declared
		case *types.Named:
			if t.TypeParams() != nil {
				return true
			}
		}
		if it, ok := o.Type().Underlying().(*types.Interface); ok && !it.IsMethodSet() {
			return true // constraint interface
		}
		_, ok := p.Types[o.Name()]
		if !ok {
			_, ok = nativeTypes[key]
		}
		return ok
	}
	return true
}
