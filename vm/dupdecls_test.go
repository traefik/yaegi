package vm

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// dupPlugin is a plugin whose files p.go and copy.go hold the same
// declarations (copy.go is p.go with other comments), as in a plugin of the
// catalog where main.go is a copy of the plugin file. Its subpackage sub
// does the same. gc refuses it ("redeclared in this block"); yaegi runs it.
const dupPlugin = `package p

import (
	"context"
	"net/http"

	"example.com/p/sub"
)

const greeting = "hello"

// Config is the configuration.
type Config struct {
	Name string ` + "`json:\"name,omitempty\"`" + `
}

func CreateConfig() *Config { return &Config{Name: greeting} }

type (
	handler struct{ word string }
	pair[T any] struct{ a, b T }
)

func (p pair[T]) first() T { return p.a }

func New(_ context.Context, _ http.Handler, c *Config, _ string) (http.Handler, error) {
	return &handler{word: pair[string]{c.Name, "x"}.first()}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	// a comment of the body
	w.Write([]byte(h.word + " " + sub.Word() + " " + string(rune('0'+inits))))
}

func init() { inits++ }

var inits int

const _ = 1
`

// dupSub is the file of the subpackage, written twice.
const dupSub = "package sub\n\nfunc Word() string { return \"world\" }\n"

// writeDupPlugin writes dupPlugin with its copies, and the files of extra
// (name -> source) in the plugin directory.
func writeDupPlugin(t *testing.T, extra map[string]string) (dir, pkgPath string) {
	t.Helper()
	dir, pkgPath = writePlugin(t, dupPlugin)
	cp := strings.Replace(dupPlugin, "// a comment of the body", "// another comment", 1)
	cp = strings.Replace(cp, "var inits int\n", "", 1) // a variable is never removed
	files := map[string]string{
		"copy.go":   cp + "\n// a comment at the end\n",
		"sub/a.go":  dupSub,
		"sub/b.go":  dupSub,
		"sub/c.go":  "package sub\n",
		"other.txt": "",
	}
	for name, src := range extra {
		files[name] = src
	}
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, pkgPath
}

// TestIgnoreDuplicateDecls checks Options.IgnoreDuplicateDecls with both
// loaders: without it, the plugin is refused as gc does; with it, the
// copies are removed and the plugin runs (init runs twice: init functions
// are not removed). The declarations whose removal could change the
// program still refuse the plugin.
func TestIgnoreDuplicateDecls(t *testing.T) {
	needSource(t)
	dir, pkgPath := writeDupPlugin(t, nil)
	for _, golist := range []bool{false, true} {
		name := "default"
		if golist {
			name = "golist"
		}
		t.Run(name, func(t *testing.T) {
			_, err := Load(Options{Dir: dir, GoList: golist}, pkgPath)
			if err == nil || (!golist && !strings.Contains(err.Error(), "redeclared in this block")) {
				t.Fatalf("without IgnoreDuplicateDecls: %v, want a redeclared error", err)
			}
			p, err := Load(Options{Dir: dir, GoList: golist, IgnoreDuplicateDecls: true}, pkgPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "dup")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got, want := w.Body.String(), "hello world 2"; got != want {
				t.Errorf("body %q, want %q", got, want)
			}

			for name, src := range map[string]string{
				// Another text for a name: yaegi keeps the last function
				// and constant, the first method.
				"other const":  "package p\n\nconst greeting = \"bye\"\n",
				"other func":   "package p\n\nfunc CreateConfig() *Config { return &Config{Name: \"bye\"} }\n",
				"other method": "package p\n\nimport \"net/http\"\n\nfunc (h *handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {}\n",
				// A variable: yaegi runs both initializers.
				"var": "package p\n\nvar inits int\n",
				// A part of a declaration.
				"part": "package p\n\nconst greeting, other = \"hello\", 1\n",
				// The same text with other imports or another Go version.
				"imports": "package p\n\nimport x \"path\"\n\nfunc join(a ...string) string { return x.Join(a...) }\n",
				"version": "//go:build go1.21\n\npackage p\n\nfunc twice() int { return 2 }\n",
			} {
				extra := map[string]string{"z.go": src}
				switch name {
				case "imports":
					extra["y.go"] = "package p\n\nimport x \"path/filepath\"\n\nfunc join(a ...string) string { return x.Join(a...) }\n"
				case "version":
					extra["y.go"] = "package p\n\nfunc twice() int { return 2 }\n"
				}
				bad, badPath := writeDupPlugin(t, extra)
				_, err := Load(Options{Dir: bad, GoList: golist, IgnoreDuplicateDecls: true}, badPath)
				if err == nil || (!golist && !strings.Contains(err.Error(), "redeclared") && !strings.Contains(err.Error(), "already declared")) {
					t.Errorf("%s: %v, want a redeclared error", name, err)
				}
			}
		})
	}
}

// TestIgnoreDuplicateDeclsSoftErrors checks IgnoreDuplicateDecls with
// IgnoreSoftErrors (the options of pluginhost): a soft error in a file
// whose copies are removed does not refuse the plugin with either loader.
// The go list loader compiles the file from an overlay copy: the copy must
// keep the base name of the file, since dropSoftErrors matches the lines
// of a compiler error to the errors of go/types by base name and line.
func TestIgnoreDuplicateDeclsSoftErrors(t *testing.T) {
	needSource(t)
	y := "package p\n\nfunc tri() int { return 3 }\n"
	z := y + "\nfunc onlyZ() { x := 1 }\n" // z.go: a copy of tri, and an unused variable
	for _, golist := range []bool{false, true} {
		dir, pkgPath := writeDupPlugin(t, map[string]string{"y.go": y, "z.go": z})
		if _, err := Load(Options{Dir: dir, GoList: golist, IgnoreDuplicateDecls: true}, pkgPath); err == nil {
			t.Errorf("golist=%v: loaded with an unused variable, without IgnoreSoftErrors", golist)
		}
		if _, err := Load(Options{Dir: dir, GoList: golist, IgnoreDuplicateDecls: true, IgnoreSoftErrors: true}, pkgPath); err != nil {
			t.Errorf("golist=%v: with IgnoreSoftErrors: %v", golist, err)
		}
	}
}

// TestDupDecls checks which declarations dupDecls removes: the second of
// two declarations with the same kind, names, text, imports and Go version,
// never an init function, a variable or a blank name.
func TestDupDecls(t *testing.T) {
	a := `package p

import "strings"

func F() string { return strings.ToUpper("a") }

func init() {}

var V = 1

const _ = 1

type T[P any] struct{ p P }

func (t *T[P]) M() P { return t.p }

func (T[Q]) N() {}

const (
	A = iota
	B
)
`
	b := strings.Replace(a, "func (T[Q]) N() {}", "func (T[R]) N() {}", 1)
	fset := token.NewFileSet()
	var files []*ast.File
	for i, src := range []string{a, b} {
		f, err := parser.ParseFile(fset, string(rune('a'+i))+".go", src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	var got []string
	for d := range dupDecls(fset, files) {
		got = append(got, fset.Position(d.Pos()).Filename+" "+dupDeclKey(d))
	}
	sort.Strings(got)
	want := []string{"b.go const A,B", "b.go func F", "b.go method T.M", "b.go type T"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("removed:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
