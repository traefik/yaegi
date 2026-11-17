package vm

// Shims: a source file of a dependency, replaced by a version that the VM
// can run.
//
// Some libraries read the internals of the Go runtime with unsafe (the
// layout of a runtime type, a field of a map header). The VM has no runtime
// types, so such code cannot run, and the VM should not learn the internals
// of each library. A shim is plain Go source instead: the whole content of
// one file of the library, written with the standard library only, which
// the plugin runs as its own code, under the same restrictions. The VM
// knows nothing of the library.
//
// A shim replaces a file only when the file has one of the given SHA-256
// sums: a shim is written for some versions of a file, and another version
// is loaded as it is. So a shim never applies to code it was not written
// for. Each replaced file gives a line of Plugin.Notes.
//
// Package github.com/traefik/yaegi/v2/shims has the shims of yaegi v2, and
// host.Options sets them.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Shim replaces a source file of a dependency of the plugin (Options.Shims).
type Shim struct {
	// File is the base name of the file, "runtime.go".
	File string
	// SHA256 are the sums (hex) of the contents that the shim replaces: the
	// file of each version of the library that the shim is written for.
	SHA256 []string
	// Src is the new content of the file. It is Go source of the same
	// package, with the same build constraints, that declares what the
	// other files of the package use from the file.
	Src []byte
	// Why says why the file is replaced, for Plugin.Notes.
	Why string
}

// shimSet finds the shim of a file.
type shimSet struct {
	byName map[string]bool
	bySum  map[[sha256.Size]byte]*Shim
}

// newShimSet returns the set of shims, or nil when there is none. A sum that
// is not 64 hex digits is ignored.
func newShimSet(shims []Shim) *shimSet {
	if len(shims) == 0 {
		return nil
	}
	s := &shimSet{byName: map[string]bool{}, bySum: map[[sha256.Size]byte]*Shim{}}
	for i := range shims {
		sh := &shims[i]
		for _, h := range sh.SHA256 {
			var sum [sha256.Size]byte
			if b, err := hex.DecodeString(h); err == nil && len(b) == sha256.Size {
				copy(sum[:], b)
				s.bySum[sum] = sh
				s.byName[sh.File] = true
			}
		}
	}
	return s
}

// replace returns the shim of the file name with content src, or nil.
func (s *shimSet) replace(name string, src []byte) *Shim {
	if s == nil || !s.byName[filepath.Base(name)] {
		return nil
	}
	return s.bySum[sha256.Sum256(src)]
}

// note is the line of Plugin.Notes for the file name replaced by sh.
func (sh *Shim) note(name string) string {
	return fmt.Sprintf("%s: replaced by a shim of yaegi v2: %s", name, sh.Why)
}

// sources returns the new contents of the files of pkgs (and their
// dependencies) that a shim replaces, by file name, and the notes. The
// loaders that use go list give them to it as an overlay: a shim may import
// packages that the original file does not. go refuses an overlay of a file
// of the module cache, so these loaders replace only the files of a vendor
// directory or of a module outside the cache; the default loader of Load
// replaces any file.
func (s *shimSet) sources(pkgs []*packages.Package) (map[string][]byte, []string) {
	if s == nil {
		return nil, nil
	}
	cache := goModCache()
	var srcs map[string][]byte
	var notes []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, name := range p.CompiledGoFiles {
			if !s.byName[filepath.Base(name)] || cache != "" && strings.HasPrefix(name, cache+string(filepath.Separator)) {
				continue
			}
			b, err := os.ReadFile(name)
			if err != nil {
				continue
			}
			if sh := s.replace(name, b); sh != nil {
				if srcs == nil {
					srcs = map[string][]byte{}
				}
				srcs[name] = sh.Src
				notes = append(notes, sh.note(name))
			}
		}
	})
	return srcs, notes
}

// goModCache is the module cache of go list: GOMODCACHE, or pkg/mod in the
// first directory of GOPATH.
func goModCache() string {
	if d := os.Getenv("GOMODCACHE"); d != "" {
		return filepath.Clean(d)
	}
	if gp := filepath.SplitList(build.Default.GOPATH); len(gp) > 0 && gp[0] != "" {
		return filepath.Join(gp[0], "pkg", "mod")
	}
	return ""
}
