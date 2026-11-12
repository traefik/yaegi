package vm

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"

	"golang.org/x/tools/go/packages"
)

// asmTags are the build tags that select the pure Go version of a package
// that also has one in assembly: x/crypto, x/sys/cpu and cespare/xxhash use
// purego, klauspost/compress (s2, zstd, huff0) uses noasm. The VM cannot run
// assembly.
var asmTags = []string{"purego", "noasm"}

// needsAsmTags reports whether a package of pkgs (or of their dependencies)
// that src accepts, a package the VM runs from source, has assembly in the
// files of go run (a .s file or a function without body) and other files
// with asmTags. tags are the build tags of the load.
//
// The default loader of plugins adds asmTags to such a package only
// (srcLoader.importDir). go list has one set of build tags for all the
// packages, and an overlay cannot change the files of the module cache, so
// the go list loaders (RunMain, and Load with Options.GoList or Options.Src)
// load again with asmTags for all the packages when this is true. The other
// packages then also take their purego or noasm files, when they have
// some: these files do the same work without assembly or without unsafe,
// so the program gives the same results.
//
// A package of the standard library is compiled into the host without
// these tags. When the tags change the files of one that the VM interprets
// (stdSrc: RunOptions.Src, Options.Src, InterpretedStd), the interpreted copy
// would not be the code of the native one: needsAsmTags is then false, and
// the load keeps the files of go run (a call to the assembly is refused, as
// before).
func needsAsmTags(pkgs []*packages.Package, src, stdSrc func(path string) bool, tags []string) bool {
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH = runtime.GOOS, runtime.GOARCH
	ctx.BuildTags = slices.Clone(tags)
	pure := ctx
	pure.BuildTags = append(slices.Clone(tags), asmTags...)
	changes := func(dir string) (asm, changed bool) {
		bp, err := ctx.ImportDir(dir, 0)
		if err != nil {
			return false, false
		}
		pbp, err := pure.ImportDir(dir, 0)
		if err != nil {
			return false, false // keep the files of go run: the run reports what is missing
		}
		asm = len(bp.SFiles) > 0 || hasBodylessFiles(dir, bp.GoFiles)
		return asm, !slices.Equal(bp.GoFiles, pbp.GoFiles) || !slices.Equal(bp.SFiles, pbp.SFiles)
	}
	found, stdChanged := false, false
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Dir == "" {
			return
		}
		switch {
		case src(p.PkgPath):
			if !found {
				asm, changed := changes(p.Dir)
				found = asm && changed
			}
		case stdSrc(p.PkgPath):
			if _, changed := changes(p.Dir); changed {
				stdChanged = true
			}
		}
	})
	return found && !stdChanged
}

// hasBodylessFiles reports whether one of the Go files names in dir declares
// a function without body (written in assembly, or linked with go:linkname).
func hasBodylessFiles(dir string, names []string) bool {
	fset := token.NewFileSet()
	for _, name := range names {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			continue // the load reports it
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body == nil {
				return true
			}
		}
	}
	return false
}
