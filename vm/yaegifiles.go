package vm

// Plugin.Notes: the files that yaegi selected in a package, when they are
// not the files that yaegi v2 selects.
//
// yaegi v2 selects the files of a package as go build does (go/build, or go
// list). yaegi reads only the "// +build" lines: a file with only a
// "//go:build" line is always kept, and a "// +build" tag that Go knows but
// yaegi does not (unix, gc, cgo, the release tags of a newer Go) is false. A
// plugin with "//go:build linux" and "//go:build !linux" variants of a file
// so runs another file under yaegi v2 than under yaegi, with no error. yaegi v2 is
// right and keeps its selection; the note only tells the host, which can
// log it.
//
// The rule below is the one of interp/build.go and interp/src.go of yaegi
// v0.16.1, checked against it (yaegifiles_test.go quotes the cases):
//
//   - the files of a directory are read in name order; a name is skipped
//     when it does not end in ".go", starts with "_" or ".", ends in
//     "_test", or has a GOOS or GOARCH suffix of another platform. yaegi
//     has its own (shorter) lists of GOOS and GOARCH values, so a_riscv64.go
//     or a_zos.go is kept; and a name whose last two parts are not
//     (GOOS, GOARCH), as in a_b_windows.go, is kept on every platform.
//   - the file is then parsed up to the package clause with its comments:
//     the comments before the clause, and the ones right after it up to the
//     next token (a "// +build" line after "package p" counts, before the
//     first import or declaration). Every "+build " line of these comments
//     must be true: the lines are an AND, the space-separated options of a
//     line an OR, the comma-separated tags of an option an AND. A tag is
//     true when it is GOOS, GOARCH, a build tag of the interpreter, or
//     "go1.N" with N at most the minor version of the Go that built yaegi.
//     A "yaegi:tags t1 t2" line of a file that passed adds t1 and t2 to the
//     build tags, for the files read after it (in this package and the
//     next ones).
//   - a "//go:build" line is never read.

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// yaegiSelect gives the files yaegi selected in a directory, as yaegi does
// it (see the top of the file).
type yaegiSelect struct {
	goos, goarch string
	goMinor      int // minor version of the Go release ("go1.N" tags)
}

func newYaegiSelect() yaegiSelect {
	return yaegiSelect{goos: runtime.GOOS, goarch: runtime.GOARCH, goMinor: goMinor(build.Default.ReleaseTags)}
}

// goMinor returns N of the last release tag "go1.N" (yaegi reads it there),
// or 0.
func goMinor(releaseTags []string) int {
	if len(releaseTags) == 0 {
		return 0
	}
	v, ok := strings.CutPrefix(releaseTags[len(releaseTags)-1], "go1.")
	if !ok {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

// files returns the names (sorted) of the files yaegi selected among names
// (base names of the .go files of one directory, selected by go/build or
// not), with the build tags yaegi had when it entered the directory. The
// names are read in order, as yaegi reads a directory: a "yaegi:tags" line
// of a file that passed counts for the files after it. header returns the
// comments of a file up to the package clause (headerOf), or ok false when
// the file cannot be read (it is left out: yaegi reports the error, yaegi v2
// too).
func (y yaegiSelect) files(names, tags []string, header func(name string) ([]*ast.CommentGroup, bool)) []string {
	names = append([]string(nil), names...)
	sort.Strings(names)
	tags = tags[:len(tags):len(tags)]
	var out []string
	for _, name := range names {
		if y.skipName(name) {
			continue
		}
		cgs, ok := header(name)
		if !ok || !y.headerOK(cgs, tags) {
			continue
		}
		tags = addYaegiTags(tags, cgs)
		out = append(out, name)
	}
	return out
}

// skipName is skipFile of yaegi: names that are not Go files, start with
// "_" or ".", are test files, or have a GOOS or GOARCH suffix of another
// platform (with the lists of yaegi: yaegiOS and yaegiArch). When the name
// has two parts after its first "_", only the last two parts are read, and
// a pair that is not (GOOS, GOARCH), (an OS, an arch) or (anything, an
// arch) keeps the file: a_b_windows.go is kept on linux.
func (y yaegiSelect) skipName(name string) bool {
	if !strings.HasSuffix(name, ".go") {
		return true
	}
	p := strings.TrimSuffix(name, ".go")
	if strings.HasPrefix(p, "_") || strings.HasPrefix(p, ".") || strings.HasSuffix(p, "_test") {
		return true
	}
	i := strings.Index(p, "_")
	if i < 0 {
		return false
	}
	a := strings.Split(p[i+1:], "_")
	last := len(a) - 1
	if last-1 >= 0 {
		switch x, z := a[last-1], a[last]; {
		case x == y.goos:
			if yaegiArch[z] {
				return z != y.goarch
			}
			return false
		case yaegiOS[x] && yaegiArch[z]:
			return true
		case yaegiArch[z] && z != y.goarch:
			return true
		default:
			return false
		}
	}
	x := a[last]
	return yaegiOS[x] && x != y.goos || yaegiArch[x] && x != y.goarch
}

// headerOK reports whether all the "+build " lines of the comments are
// true with the build tags. The lines of one file are an AND, the options
// of a line an OR, the comma-separated tags of an option an AND. The text
// of a group is trimmed as a whole (yaegi does it): "/* +build x */" and
// "//  +build x" are constraints, "// x\n// +build y" too.
func (y yaegiSelect) headerOK(cgs []*ast.CommentGroup, tags []string) bool {
	for _, cg := range cgs {
		for _, line := range strings.Split(strings.TrimSpace(cg.Text()), "\n") {
			if !y.lineOK(line, tags) {
				return false
			}
		}
	}
	return true
}

func (y yaegiSelect) lineOK(line string, tags []string) bool {
	if !strings.HasPrefix(line, "+build ") {
		return true
	}
	for _, o := range strings.Split(strings.TrimSpace(line[6:]), " ") {
		if y.optionOK(o, tags) {
			return true
		}
	}
	return false
}

func (y yaegiSelect) optionOK(o string, tags []string) bool {
	for _, t := range strings.Split(o, ",") {
		if !y.tagOK(t, tags) {
			return false
		}
	}
	return true
}

// tagOK is buildTagOk of yaegi. An empty tag (two spaces between two
// options) makes yaegi panic; here it is false, and the next option is
// read.
func (y yaegiSelect) tagOK(s string, tags []string) bool {
	not := strings.HasPrefix(s, "!")
	if not {
		s = s[1:]
	}
	r := false
	switch {
	case containsStr(tags, s), s == y.goos, s == y.goarch:
		r = true
	case len(s) > 4 && s[:4] == "go1.":
		n, err := strconv.Atoi(s[4:])
		r = err == nil && y.goMinor >= n
	}
	return r != not
}

// addYaegiTags is setYaegiTags of yaegi: the tags of the "yaegi:tags t1
// t2" lines of the comments are added to tags (a copy).
func addYaegiTags(tags []string, cgs []*ast.CommentGroup) []string {
	for _, cg := range cgs {
		for _, line := range strings.Split(strings.TrimSpace(cg.Text()), "\n") {
			if !strings.HasPrefix(line, "yaegi:tags ") {
				continue
			}
			for _, t := range strings.Split(strings.TrimSpace(line[10:]), " ") {
				if !containsStr(tags, t) {
					tags = append(tags, t)
				}
			}
		}
	}
	return tags
}

func containsStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// readHeader returns the comments up to the package clause of file name
// (headerOf). A symbolic link is not read (the loaders refuse it).
func readHeader(name string) ([]*ast.CommentGroup, bool) {
	st, err := os.Lstat(name)
	if err != nil || !st.Mode().IsRegular() {
		return nil, false
	}
	f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return nil, false
	}
	return headerOf(f), true
}

// headerOf returns the comments of f that a parse with PackageClauseOnly
// gives (yaegi reads the constraints there): the comments before the
// package clause, and the ones after it up to the next token (the first
// import or declaration). f may be the result of a full parse.
func headerOf(f *ast.File) []*ast.CommentGroup {
	end := token.Pos(-1) // no declaration: every comment
	if len(f.Decls) > 0 {
		end = f.Decls[0].Pos()
	}
	var out []*ast.CommentGroup
	for _, cg := range f.Comments {
		if end < 0 || cg.Pos() < end {
			out = append(out, cg)
		}
	}
	return out
}

// yaegiFilesNote compares the files yaegi v2 selected in package path (goFiles)
// with the files yaegi selected, and returns a note when they differ (""
// otherwise).
func yaegiFilesNote(path string, goFiles, yaegi []string) string {
	vm := map[string]bool{}
	for _, f := range goFiles {
		vm[f] = true
	}
	ya := map[string]bool{}
	for _, f := range yaegi {
		ya[f] = true
	}
	var onlyVM, onlyYaegi []string
	for _, f := range goFiles {
		if !ya[f] {
			onlyVM = append(onlyVM, f)
		}
	}
	for _, f := range yaegi {
		if !vm[f] {
			onlyYaegi = append(onlyYaegi, f)
		}
	}
	if len(onlyVM) == 0 && len(onlyYaegi) == 0 {
		return ""
	}
	sort.Strings(onlyVM)
	sort.Strings(onlyYaegi)
	list := func(l []string) string {
		if len(l) == 0 {
			return "none"
		}
		return strings.Join(l, ", ")
	}
	return fmt.Sprintf("package %s: the build constraints select other files than yaegi did (yaegi reads only \"// +build\" lines): only yaegi v2 loads %s; only yaegi loaded %s",
		path, list(onlyVM), list(onlyYaegi))
}

// baseNames returns the base names of the .go files of names.
func baseNames(names ...[]string) []string {
	var out []string
	for _, l := range names {
		for _, n := range l {
			if strings.HasSuffix(n, ".go") {
				out = append(out, filepath.Base(n))
			}
		}
	}
	return out
}

// yaegiOS and yaegiArch are the GOOS and GOARCH values that a file name
// suffix selects under yaegi (knownOs and knownArch of interp/build.go):
// shorter than the lists of go/build, so a_riscv64.go or a_zos.go has no
// suffix for yaegi.
var yaegiOS = setOf("aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js",
	"linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows")

var yaegiArch = setOf("386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle",
	"ppc64", "ppc64le", "s390x", "wasm")

func setOf(l ...string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, s := range l {
		m[s] = true
	}
	return m
}

// yaegiStartTags returns the build tags yaegi has when it enters the
// directory of a package: none for the plugin package (the host sets none),
// and for the other packages the "yaegi:tags" of the plugin package (which
// yaegi read before it imported them; yaegi v2 selects its files with them
// too, run.go). extra holds the tags yaegi v2 added to its own selection
// (purego and noasm, srcload.go): the yaegi side gets them too, so that only
// the rule that reads the constraints differs.
func yaegiStartTags(isPlugin bool, pluginTags, extra []string) []string {
	var tags []string
	if !isPlugin {
		tags = append(tags, pluginTags...)
	}
	return append(tags, extra...)
}

// goListYaegiNotes returns the notes of the GoList loader: for each loaded
// package that own accepts, the files of go list against the files yaegi
// selected, with the "// yaegi:tags" of the plugin package pkgPath (go
// list does not use them: a difference is then a real one too). extra are
// the tags that the loader added for the packages with assembly (asmTags):
// the yaegi side starts with them too, as in noteYaegiFiles.
func goListYaegiNotes(pkgs []*packages.Package, pkgPath string, extra []string, own func(path string) bool) []string {
	var pluginTags []string
	for _, p := range pkgs {
		if p.PkgPath == pkgPath && len(p.GoFiles) > 0 {
			pluginTags = yaegiTags("", filepath.Dir(p.GoFiles[0]), nil) // go list reads these files too
		}
	}
	y := newYaegiSelect()
	var notes []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if !own(p.PkgPath) {
			return
		}
		var dir string
		switch {
		case len(p.GoFiles) > 0:
			dir = filepath.Dir(p.GoFiles[0])
		case len(p.IgnoredFiles) > 0:
			dir = filepath.Dir(p.IgnoredFiles[0])
		default:
			return
		}
		tags := yaegiStartTags(p.PkgPath == pkgPath, pluginTags, extra)
		yaegi := y.files(baseNames(p.GoFiles, p.IgnoredFiles), tags, func(name string) ([]*ast.CommentGroup, bool) {
			return readHeader(filepath.Join(dir, name))
		})
		if n := yaegiFilesNote(p.PkgPath, baseNames(p.GoFiles), yaegi); n != "" {
			notes = append(notes, n)
		}
	})
	sort.Strings(notes)
	return notes
}
