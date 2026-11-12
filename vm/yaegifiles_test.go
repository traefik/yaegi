package vm

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// writeBuildPlugin writes a plugin with two variants of osName, one for
// windows and one for the other systems, with the constraint lines
// head(tag) ("//go:build windows", maybe also "// +build windows").
func writeBuildPlugin(t *testing.T, head func(tag string) string) (dir, pkgPath string) {
	t.Helper()
	dir, pkgPath = writePlugin(t, "package p\n\nfunc OSName() string { return osName() }\n")
	writeFiles(t, dir, map[string]string{
		"windows.go":    head("windows") + "\npackage p\n\nfunc osName() string { return \"windows\" }\n",
		"notwindows.go": head("!windows") + "\npackage p\n\nfunc osName() string { return \"other\" }\n",
	})
	return dir, pkgPath
}

// loaders runs f with both loaders (default and GoList).
func loaders(t *testing.T, f func(t *testing.T, golist bool)) {
	for _, golist := range []bool{false, true} {
		name := "default"
		if golist {
			name = "golist"
		}
		t.Run(name, func(t *testing.T) { f(t, golist) })
	}
}

// TestYaegiFilesNote checks Plugin.Notes with both loaders: files with only
// "//go:build" lines (yaegi kept both, yaegi v2 keeps one) give one note that
// names the files; the same files with "// +build" lines too give none.
// The selection of yaegi v2 does not change: the plugin runs the !windows file.
func TestYaegiFilesNote(t *testing.T) {
	needSource(t)
	if runtime.GOOS == "windows" {
		t.Skip("the layout selects the other file on windows")
	}
	onlyGoBuild, _ := writeBuildPlugin(t, func(tag string) string { return "//go:build " + tag + "\n" })
	both, pkgPath := writeBuildPlugin(t, func(tag string) string {
		return "//go:build " + tag + "\n// +build " + tag + "\n"
	})
	loaders(t, func(t *testing.T, golist bool) {
		p, err := Load(Options{Dir: onlyGoBuild, GoList: golist}, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Notes) != 1 {
			t.Fatalf("notes = %q, want one", p.Notes)
		}
		for _, want := range []string{"package example.com/p:", "only yaegi v2 loads none", "only yaegi loaded windows.go"} {
			if !strings.Contains(p.Notes[0], want) {
				t.Errorf("note %q does not contain %q", p.Notes[0], want)
			}
		}
		res, err := p.Call(context.Background(), "OSName")
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := res[0].Interface(); v != "other" {
			t.Errorf("OSName() = %v, want other", v)
		}
		p, err = Load(Options{Dir: both, GoList: golist}, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		if p.Notes != nil {
			t.Errorf("notes with // +build lines = %q, want none", p.Notes)
		}
	})
}

// TestYaegiFilesNoteTagsOrder checks that the yaegi side reads the
// "yaegi:tags" lines in the order of the files, as yaegi does: a tag set in
// z.go does not select b.go, read before it. With Options.YaegiTags, the
// loaders read the tags of the whole directory (and load b.go), so they
// give a note that names b.go. The same tag set in a.go, read before b.go,
// gives no note. Without the option, the loaders do not read the lines (as
// go build): b.go is loaded in neither layout, and the notes say the
// opposite. Both loaders give the same notes.
func TestYaegiFilesNoteTagsOrder(t *testing.T) {
	needSource(t)
	dirZ, pkgPath := writePlugin(t, "package p\n\nfunc N() int { return 1 }\n")
	writeFiles(t, dirZ, map[string]string{
		"b.go": "//go:build foo\n// +build foo\n\npackage p\n\nvar n = 1\n",
		"z.go": "// yaegi:tags foo\n\npackage p\n",
	})
	dirA, _ := writePlugin(t, "package p\n\nfunc N() int { return 1 }\n")
	writeFiles(t, dirA, map[string]string{
		"b.go": "//go:build foo\n// +build foo\n\npackage p\n\nvar n = 1\n",
		"a.go": "// yaegi:tags foo\n\npackage p\n",
	})
	loaders(t, func(t *testing.T, golist bool) {
		p, err := Load(Options{Dir: dirZ, GoList: golist, YaegiTags: true}, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Notes) != 1 || !strings.Contains(p.Notes[0], "only yaegi v2 loads b.go; only yaegi loaded none") {
			t.Errorf("YaegiTags, notes with the tag after the file = %q, want one that names b.go", p.Notes)
		}
		p, err = Load(Options{Dir: dirA, GoList: golist, YaegiTags: true}, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		if p.Notes != nil {
			t.Errorf("YaegiTags, notes with the tag before the file = %q, want none", p.Notes)
		}
		p, err = Load(Options{Dir: dirZ, GoList: golist}, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		if p.Notes != nil {
			t.Errorf("without YaegiTags, notes with the tag after the file = %q, want none", p.Notes)
		}
		p, err = Load(Options{Dir: dirA, GoList: golist}, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Notes) != 1 || !strings.Contains(p.Notes[0], "only yaegi v2 loads none; only yaegi loaded b.go") {
			t.Errorf("without YaegiTags, notes with the tag before the file = %q, want one that names b.go", p.Notes)
		}
	})
}

// TestYaegiFilesNoteDeps checks a note of a dependency package, with both
// loaders, in the same order: the notes are sorted (by text, so by package
// path: "p/dep" comes before "p:"). The
// dependency has a "//go:build"-only variant pair (a note), and the
// plugin package too.
func TestYaegiFilesNoteDeps(t *testing.T) {
	needSource(t)
	if runtime.GOOS == "windows" {
		t.Skip("the layout selects the other file on windows")
	}
	dir, pkgPath := writePlugin(t, "package p\n\nimport \"example.com/p/dep\"\n\nfunc OSName() string { return dep.OSName() + osName() }\n")
	writeFiles(t, dir, map[string]string{
		"windows.go":    "//go:build windows\n\npackage p\n\nfunc osName() string { return \"windows\" }\n",
		"notwindows.go": "//go:build !windows\n\npackage p\n\nfunc osName() string { return \"other\" }\n",
	})
	depDir := filepath.Join(dir, "dep")
	if err := os.Mkdir(depDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, depDir, map[string]string{
		"dep.go":        "package dep\n\nfunc OSName() string { return osName() }\n",
		"windows.go":    "//go:build windows\n\npackage dep\n\nfunc osName() string { return \"windows\" }\n",
		"notwindows.go": "//go:build !windows\n\npackage dep\n\nfunc osName() string { return \"other\" }\n",
	})
	var notes [][]string
	loaders(t, func(t *testing.T, golist bool) {
		p, err := Load(Options{Dir: dir, GoList: golist}, pkgPath)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"package example.com/p/dep: the build constraints select other files than yaegi did (yaegi reads only \"// +build\" lines): only yaegi v2 loads none; only yaegi loaded windows.go",
			"package example.com/p: the build constraints select other files than yaegi did (yaegi reads only \"// +build\" lines): only yaegi v2 loads none; only yaegi loaded windows.go",
		}
		if strings.Join(p.Notes, "\n") != strings.Join(want, "\n") {
			t.Errorf("notes = %q, want %q", p.Notes, want)
		}
		notes = append(notes, p.Notes)
	})
	if len(notes) == 2 && strings.Join(notes[0], "\n") != strings.Join(notes[1], "\n") {
		t.Errorf("the loaders differ: %q and %q", notes[0], notes[1])
	}
}

// TestYaegiSelect checks the rule of yaegi, case by case against what
// yaegi v0.16.1 does on linux/amd64 (interp.New with the files in a GOPATH,
// then `import` of the package): file name suffixes, the "// +build" lines
// (not "//go:build", not the unix tag), the comments after the package
// clause, and the "yaegi:tags" lines in file order.
func TestYaegiSelect(t *testing.T) {
	y := yaegiSelect{goos: "linux", goarch: "amd64", goMinor: 22}
	for name, want := range map[string]bool{
		"a.go": false, "a_linux.go": false, "a_linux_amd64.go": false, "a_amd64.go": false,
		"a_windows.go": true, "a_linux_arm64.go": true, "a_darwin_amd64.go": true, "a_arm64.go": true,
		"a_test.go": true, "_a.go": true, ".a.go": true, "a.txt": true, "a_b.go": false,
		"a_wasip1_wasm.go": true, "a_ios.go": true, "a_android.go": true, "a_illumos.go": true,
		// yaegi has its own lists of GOOS and GOARCH values: these are not in them.
		"a_riscv64.go": false, "a_linux_riscv64.go": false, "a_zos.go": false, "a_hurd.go": false, "a_nacl.go": false,
		// yaegi reads the last two parts only, and keeps a pair that is
		// not (an OS, an arch) or (GOOS, an arch) or (anything, an arch).
		"a_b_windows.go": false, "a_b_amd64.go": false, "a_b_arm64.go": true, "a_x_linux.go": false,
		"a_linux_x.go": false, "a_windows_b.go": false, "a_amd64_linux.go": false, "a_linux_amd64_b.go": false,
		"a_unix.go": false,
	} {
		if got := y.skipName(name); got != want {
			t.Errorf("skipName(%s) = %v, want %v", name, got, want)
		}
	}
	// head is the text before "package p", after the text after it.
	type file struct{ head, after string }
	for _, c := range []struct {
		file file
		tags []string
		want bool
	}{
		{file{"//go:build windows\n", ""}, nil, true},
		{file{"// +build linux\n", ""}, nil, true},
		{file{"// +build windows\n", ""}, nil, false},
		{file{"// +build !windows\n", ""}, nil, true},
		{file{"// +build windows linux\n", ""}, nil, true},
		{file{"// +build linux,!amd64\n", ""}, nil, false},
		{file{"// +build linux,amd64\n", ""}, nil, true},
		{file{"// +build linux\n// +build windows\n", ""}, nil, false},
		{file{"// +build linux\n\n// +build windows\n", ""}, nil, false},
		{file{"// +build foo\n", ""}, []string{"foo"}, true},
		{file{"// +build foo\n", ""}, nil, false},
		{file{"// +build !foo\n", ""}, []string{"foo"}, false},
		{file{"// +build unix\n", ""}, nil, false},
		{file{"// +build !unix\n", ""}, nil, true},
		{file{"// +build gc\n", ""}, nil, false},
		{file{"// +build cgo\n", ""}, nil, false},
		{file{"// +build ignore\n", ""}, nil, false},
		{file{"// +build go1.21\n", ""}, nil, true},
		{file{"// +build go1.23\n", ""}, nil, false},
		{file{"// +build !go1.23\n", ""}, nil, true},
		{file{"// +build go1.21.3\n", ""}, nil, false},
		{file{"// +build Linux\n", ""}, nil, false},
		{file{"// a comment\n", ""}, nil, true},
		// The text of a group is trimmed as a whole, then split in lines.
		{file{"/* +build windows */\n", ""}, nil, false},
		{file{"// x\n// +build windows\n", ""}, nil, false},
		{file{"// Package p.\n//\n// +build windows\n", ""}, nil, false},
		{file{"//  +build windows\n", ""}, nil, false},
		{file{"// +build\twindows\n", ""}, nil, true},
		{file{"// +build \n", ""}, nil, true},
		{file{"// +buildwindows\n", ""}, nil, true},
		{file{"// +build linux \n", ""}, nil, true},
		// Two spaces between two options: yaegi panics (an empty tag);
		// here the empty tag is false and the next option is read.
		{file{"// +build windows  linux\n", ""}, nil, true},
		// The comments after the package clause, up to the next token,
		// are read too.
		{file{"", " // +build windows"}, nil, false},
		{file{"", "\n// +build windows\n"}, nil, false},
		{file{"", "\n\n// +build windows\n\nvar y = 1\n"}, nil, false},
		{file{"", "\n\nvar y = 1 // +build windows\n"}, nil, true},
		{file{"", "\n\nimport \"fmt\"\n\n// +build windows\n\nvar y = fmt.Sprint\n"}, nil, true},
	} {
		src := c.file.head + "package p" + c.file.after + "\n"
		for _, mode := range []parser.Mode{parser.PackageClauseOnly, 0} {
			f, err := parser.ParseFile(token.NewFileSet(), "a.go", src, mode|parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			if got := y.headerOK(headerOf(f), c.tags); got != c.want {
				t.Errorf("headerOK(%q, mode %v, tags %v) = %v, want %v", src, mode, c.tags, got, c.want)
			}
		}
	}
	// The yaegi:tags of a file that passed count for the files after it
	// (in name order), not before it, and not when the file is skipped.
	for _, c := range []struct {
		files map[string]string
		tags  []string
		want  []string
	}{
		{map[string]string{"a.go": "// yaegi:tags foo\n", "b.go": "// +build foo\n"}, nil, []string{"a.go", "b.go"}},
		{map[string]string{"z.go": "// yaegi:tags foo\n", "b.go": "// +build foo\n"}, nil, []string{"z.go"}},
		{map[string]string{"a.go": "// +build windows\n// yaegi:tags foo\n", "b.go": "// +build foo\n"}, nil, nil},
		{map[string]string{"a.go": "/* yaegi:tags foo */\n", "b.go": "// +build foo\n"}, nil, []string{"a.go", "b.go"}},
		{map[string]string{"a.go": "// yaegi:tags  foo bar\n", "b.go": "// +build bar\n"}, nil, []string{"a.go", "b.go"}},
		{map[string]string{"b.go": "// +build foo\n"}, []string{"foo"}, []string{"b.go"}},
	} {
		var names []string
		for name := range c.files {
			names = append(names, name)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		got := y.files(names, c.tags, func(name string) ([]*ast.CommentGroup, bool) {
			f, err := parser.ParseFile(token.NewFileSet(), name, c.files[name]+"package p\n", parser.PackageClauseOnly|parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			return headerOf(f), true
		})
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("files(%v, %v) = %v, want %v", c.files, c.tags, got, c.want)
		}
	}
	if got := goMinor([]string{"go1.1", "go1.26"}); got != 26 {
		t.Errorf("goMinor = %d, want 26", got)
	}
}
