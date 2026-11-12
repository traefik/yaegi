package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbedFile checks that //go:embed into a string reads one regular file
// of the package directory and nothing else.
func TestEmbedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("B"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "a.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if got, err := embedFileContent(dir, []string{"a.txt"}); err != nil || got != "A" {
		t.Errorf("a.txt: %q, %v", got, err)
	}
	if got, err := embedFileContent(dir, []string{"all:b.*"}); err != nil || got != "B" {
		t.Errorf("all:b.*: %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("C"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := embedFileContent(dir, []string{"sub/c.txt"}); err != nil || got != "C" {
		t.Errorf("sub/c.txt: %q, %v", got, err)
	}
	// A directory with one file is that file, as with go build.
	if got, err := embedFileContent(dir, []string{"sub"}); err != nil || got != "C" {
		t.Errorf("sub: %q, %v", got, err)
	}
	// The same file named twice is one file.
	if got, err := embedFileContent(dir, []string{"a.txt", "a.*"}); err != nil || got != "A" {
		t.Errorf("a.txt a.*: %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "d.txt"), []byte("D"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A symbolic link to a directory outside the package: the file behind
	// it is a regular file, but it is not in the package (gc refuses it:
	// "in non-directory").
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	for _, pats := range [][]string{
		{"../a.txt"}, {"./a.txt"}, {"/etc/hosts"}, {`a\b`}, {"*.txt"}, {"."}, {"sub/"}, {"[a"},
		{"a.txt", "b.txt"}, {"sub"}, {"link.txt"}, {"none.txt"},
		{"linkdir/secret.txt"}, {"linkdir/*"}, {"linkdir"},
	} {
		if got, err := embedFileContent(dir, pats); err == nil {
			t.Errorf("%q: read %q, want an error", pats, got)
		}
	}
}

// TestEmbedFSRules checks the files of an embed.FS against the rules of the
// go command: hidden names, symbolic links, other modules, version control
// directories, the order of the entries, and the limits.
func TestEmbedFSRules(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"d/a.txt":         "a",
		"d/.h":            "hidden",
		"d/_u":            "underscore",
		"d/e/b.txt":       "bb",
		"d/.git/config":   "git",
		"d/mod/go.mod":    "module other\n",
		"d/mod/m.txt":     "m",
		"d/z/.only":       "dot",
		"other/go.mod":    "module other2\n",
		"other/o.txt":     "o",
		"empty/.x":        "x",
		"big/f1":          strings.Repeat("x", 600),
		"big/f2":          strings.Repeat("y", 600),
		"many/1":          "1",
		"many/2":          "2",
		"many/3":          "3",
		"many/sub/4":      "4",
		"outside/s.txt":   "s",
		"d/e/f/g/h/i.txt": "deep",
		"badfile/ok.txt":  "ok",
		"badfile/x;y":     "bad",
		"baddir/ok.txt":   "ok",
		"baddir/d;r/f":    "bad",
		"baddir/_u;v":     "hidden",
	})
	if err := os.Symlink(filepath.Join(dir, "d", "a.txt"), filepath.Join(dir, "d", "link")); err != nil {
		t.Fatal(err)
	}
	names := func(pats ...string) string {
		t.Helper()
		v, err := embedRead(dir, pats, true, newEmbedLimits(LoadLimits{}))
		if err != nil {
			return "error: " + err.Error()
		}
		var out []string
		for _, f := range v.files {
			out = append(out, f.name+"="+f.data)
		}
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		pats []string
		want string
	}{
		// The tree of d: no hidden names, no link, no other module, no .git
		// (z holds only a hidden file: the directory is not there either).
		{[]string{"d"}, "d/= d/a.txt=a d/e/= d/e/b.txt=bb d/e/f/= d/e/f/g/= d/e/f/g/h/= d/e/f/g/h/i.txt=deep"},
		// all: keeps the hidden names, not the rest.
		{[]string{"all:d"}, "d/= d/.h=hidden d/_u=underscore d/a.txt=a d/e/= d/z/= d/e/b.txt=bb d/e/f/= d/e/f/g/= d/e/f/g/h/= d/e/f/g/h/i.txt=deep d/z/.only=dot"},
		// A hidden file named by a pattern is embedded.
		{[]string{"d/.h", "d/_*"}, "d/= d/.h=hidden d/_u=underscore"},
		{[]string{"d/e/b.txt", "d/a.txt", "d/*.txt"}, "d/= d/a.txt=a d/e/= d/e/b.txt=bb"},
		{[]string{"d/link"}, "error: pattern d/link: cannot embed irregular file d/link"},
		{[]string{"d/mod/m.txt"}, "error: pattern d/mod/m.txt: cannot embed file d/mod/m.txt: in different module"},
		{[]string{"other"}, "error: pattern other: cannot embed directory other: in different module"},
		{[]string{"d/.git"}, "error: pattern d/.git: cannot embed directory d/.git: invalid name .git"},
		{[]string{"d/.git/config"}, "error: pattern d/.git/config: cannot embed file d/.git/config: in invalid directory .git"},
		{[]string{"empty"}, "error: pattern empty: cannot embed directory empty: contains no embeddable files"},
		{[]string{"d/none*"}, "error: pattern d/none*: no matching files found"},
		// A file with a name that a module cannot hold is an error (the go
		// command, upstream-issue: 54003); such a directory, or a hidden
		// file with such a name, is left out.
		{[]string{"badfile"}, "error: pattern badfile: cannot embed file badfile/x;y: invalid name x;y"},
		{[]string{"baddir"}, "baddir/= baddir/ok.txt=ok"},
		{[]string{"all:baddir"}, "baddir/= baddir/ok.txt=ok"},
		{[]string{"badfile/x;y"}, "error: pattern badfile/x;y: cannot embed file badfile/x;y: invalid name x;y"},
		{[]string{"../outside"}, "error: pattern ../outside: invalid pattern syntax"},
	} {
		if got := names(c.pats...); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.pats, got, c.want)
		}
	}
	// The limits: the bytes and the entries of a load.
	if _, err := embedRead(dir, []string{"big"}, true, newEmbedLimits(LoadLimits{MaxTotalSourceBytes: 1000})); err == nil || !strings.Contains(err.Error(), "more than 1000 bytes") {
		t.Errorf("big with 1000 bytes: %v", err)
	}
	if _, err := embedRead(dir, []string{"big"}, true, newEmbedLimits(LoadLimits{MaxTotalSourceBytes: 1200})); err != nil {
		t.Errorf("big with 1200 bytes: %v", err)
	}
	if _, err := embedRead(dir, []string{"many"}, true, newEmbedLimits(LoadLimits{MaxFiles: 4})); err == nil || !strings.Contains(err.Error(), "more than 4 embedded") {
		t.Errorf("many with 4 entries: %v", err)
	}
	lim := newEmbedLimits(LoadLimits{MaxFiles: 1 << 30, MaxTotalSourceBytes: 1 << 40})
	if lim.maxBytes != maxEmbedBytes || lim.maxEntries != maxEmbedEntries {
		t.Errorf("the hard limits are not kept: %d %d", lim.maxBytes, lim.maxEntries)
	}
}

// TestEmbedFS checks that an embed.FS variable of a restricted plugin holds
// its files, and that a //go:embed in a file that does not import embed, or
// into a variable of another type, is refused as gc refuses it.
func TestEmbedFS(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":          "module example.com/emb\n\ngo 1.22\n",
		"data/a.txt":      "alpha",
		"data/sub/b.json": "{}",
		"emb.go": `package emb

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed data
var files embed.FS

func Name() string {
	var out []string
	fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		out = append(out, p)
		return err
	})
	b, err := files.ReadFile("data/a.txt")
	return strings.Join(out, ",") + " " + string(b) + " " + fmtErr(err)
}

func fmtErr(err error) string {
	if err != nil {
		return err.Error()
	}
	return "nil"
}
`,
	})
	p, err := Load(Options{Dir: dir, Env: []string{}}, "example.com/emb")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := callName(t, p), ".,data,data/a.txt,data/sub,data/sub/b.json alpha nil"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	for name, src := range map[string]string{
		"no import": "package emb\n\n//go:embed data/a.txt\nvar s string\n",
		"bad type":  "package emb\n\nimport _ \"embed\"\n\n//go:embed data/a.txt\nvar s int\n",
	} {
		writeFiles(t, dir, map[string]string{"emb.go": src})
		_, err := Load(Options{Dir: dir, Env: []string{}}, "example.com/emb")
		want := map[string]string{"no import": `go:embed only allowed in Go files that import "embed"`, "bad type": "go:embed cannot apply to var of type int"}[name]
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", name, err, want)
		}
	}
}

// embedFileContent returns the content of the one file that the patterns
// name in dir (a string or []byte variable).
func embedFileContent(dir string, pats []string) (string, error) {
	v, err := embedRead(dir, pats, false, newEmbedLimits(LoadLimits{}))
	return v.data, err
}
