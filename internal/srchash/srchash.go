// Package srchash hashes the Go sources of a package directory. cmd/aotgen
// records the hash of the sources an AOT copy was generated from, and the
// VM compares it with the plugin's sources at load (vm/aotsource.go).
package srchash

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/build/constraint"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Dir returns the hash of the Go sources of the package in dir, as
// recorded in vm.Package.Sources: the .go files that `go mod vendor` copies
// (no _test.go file, no file whose build constraint is false whatever the
// tags), so that the hash is the same on every platform and in the module
// cache and a vendor directory.
//
// A file is left out only when go/build leaves it out too for any build
// tags, so a file that a build compiles is always in the hash. A .go entry
// that is not a regular file (a symbolic link, say) is an error: go/build
// would compile the file it points to.
func Dir(dir string) (string, error) {
	h, _, err := DirFiles(dir)
	return h, err
}

// DirFiles is Dir, and it also returns the names of the hashed files.
func DirFiles(dir string) (string, []string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	var names []string
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		if !e.Type().IsRegular() {
			return "", nil, fmt.Errorf("%s: not a regular file", filepath.Join(dir, n))
		}
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	var files []string
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return "", nil, err
		}
		if ignoredFile(b) {
			continue
		}
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(b))
		h.Write(b)
		files = append(files, n)
	}
	return "s1:" + hex.EncodeToString(h.Sum(nil)), files, nil
}

// ignoredFile reports whether `go mod vendor` leaves the Go file out: its
// build constraint is false even when every tag but "ignore" may be set or
// not (cmd/go/internal/imports.ShouldBuild with AnyTags). As in go/build,
// the //go:build line controls when there is one, and the "// +build"
// lines count only when there is none. A header that go/build refuses (two
// //go:build lines, a bad expression) does not leave the file out: hashing
// one file too many only makes the hash differ, which is safe.
func ignoredFile(src []byte) bool {
	header, goBuild, ok := fileHeader(src)
	if !ok {
		return false
	}
	if goBuild != nil {
		x, err := constraint.Parse(string(goBuild))
		return err == nil && !evalAnyTag(x, true)
	}
	for _, line := range bytes.Split(header, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("//")) || !bytes.Contains(line, []byte("+build")) {
			continue
		}
		t := string(line)
		if !constraint.IsPlusBuild(t) {
			continue
		}
		if x, err := constraint.Parse(t); err == nil && !evalAnyTag(x, true) {
			return true
		}
	}
	return false
}

// fileHeader is parseFileHeader of go/build and cmd/go/internal/imports:
// header is the leading run of // comments and blank lines up to the last
// blank line (where "// +build" lines count), goBuild the //go:build line
// of the leading comments, if any. ok is false for two //go:build lines.
func fileHeader(content []byte) (header, goBuild []byte, ok bool) {
	end := 0
	p := content
	ended := false       // found a non-blank, non-// line: no more "// +build" lines
	inSlashStar := false // in a /* */ comment
Lines:
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 && !ended { // blank line
			end = len(content) - len(p)
			continue Lines
		}
		if !bytes.HasPrefix(line, []byte("//")) {
			ended = true
		}
		if !inSlashStar && isGoBuildComment(line) {
			if goBuild != nil {
				return nil, nil, false
			}
			goBuild = line
		}
	Comments:
		for len(line) > 0 {
			if inSlashStar {
				if i := bytes.Index(line, []byte("*/")); i >= 0 {
					inSlashStar = false
					line = bytes.TrimSpace(line[i+2:])
					continue Comments
				}
				continue Lines
			}
			if bytes.HasPrefix(line, []byte("//")) {
				continue Lines
			}
			if bytes.HasPrefix(line, []byte("/*")) {
				inSlashStar = true
				line = bytes.TrimSpace(line[2:])
				continue Comments
			}
			break Lines // non-comment text
		}
	}
	return content[:end], goBuild, true
}

func isGoBuildComment(line []byte) bool {
	const prefix = "//go:build"
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return false
	}
	rest := bytes.TrimSpace(line)[len(prefix):]
	return len(rest) == 0 || len(bytes.TrimSpace(rest)) < len(rest)
}

func evalAnyTag(x constraint.Expr, prefer bool) bool {
	switch x := x.(type) {
	case *constraint.TagExpr:
		for _, c := range x.Tag {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '.' {
				return false
			}
		}
		return x.Tag != "ignore" && prefer
	case *constraint.NotExpr:
		return !evalAnyTag(x.X, !prefer)
	case *constraint.AndExpr:
		return evalAnyTag(x.X, prefer) && evalAnyTag(x.Y, prefer)
	case *constraint.OrExpr:
		return evalAnyTag(x.X, prefer) || evalAnyTag(x.Y, prefer)
	}
	return false
}
