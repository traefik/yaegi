package interp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/stdlib"
)

// TestUpstreamIssue1069 checks that a void statement or call does not return
// the result of a previous evaluation.
func TestUpstreamIssue1069(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"notebook/notebook": {"Show": reflect.ValueOf(func(v interface{}) {})}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "notebook"`); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`str := "foobar"`); err != nil {
		t.Fatal(err)
	}
	v, err := i.Eval(`notebook.Show(1)`)
	if err != nil {
		t.Fatal(err)
	}
	if v.IsValid() {
		t.Fatalf("got %v, want invalid reflect.Value", v)
	}
}

// TestUpstreamIssue1629 checks that Symbols("") does not panic on generic
// source packages (cmp, maps, slices) loaded with the stdlib symbols.
func TestUpstreamIssue1629(t *testing.T) {
	i := New(Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Symbols panicked: %v", r)
		}
	}()
	m := i.Symbols("")
	if s, ok := m["fmt"]; ok {
		if _, ok := s["Println"]; !ok {
			t.Fatal("fmt.Println not found in symbols")
		}
	} else {
		t.Fatal("package fmt not found in symbols")
	}
}

// TestUpstreamIssue1636 checks that a type declaration referencing an
// undefined package symbol returns an error instead of panicking.
func TestUpstreamIssue1636(t *testing.T) {
	for _, src := range []string{
		"package main\nimport \"pkg\"\ntype T pkg.Missing\nfunc main() {}\n",
		"package main\nimport \"pkg\"\ntype T = pkg.Missing\nfunc main() {}\n",
		"package main\nimport \"pkg\"\ntype T struct { F pkg.Missing }\nfunc main() {}\n",
	} {
		i := New(Options{})
		if err := i.Use(Exports{"pkg/pkg": {"Val": reflect.ValueOf(42)}}); err != nil {
			t.Fatal(err)
		}
		_, err := i.Eval(src)
		if err == nil {
			t.Errorf("got no error for %q, want an undefined type error", src)
			continue
		}
		if strings.Contains(err.Error(), "panic") {
			t.Errorf("got panic for %q: %v", src, err)
		}
	}
}

// TestUpstreamIssue1637 checks that ImportUsed does not bind a base package
// name shared by several packages, so that explicit imports resolve
// deterministically.
func TestUpstreamIssue1637(t *testing.T) {
	fonts := Exports{
		"github.com/user1/proj1/font/font": {"One": reflect.ValueOf("one")},
		"github.com/user2/proj2/font/font": {"Two": reflect.ValueOf("two")},
		"github.com/user3/proj3/font/font": {"Three": reflect.ValueOf("three")},
	}
	for iter := 0; iter < 20; iter++ {
		i := New(Options{})
		if err := i.Use(fonts); err != nil {
			t.Fatal(err)
		}
		i.ImportUsed()
		src := `package main
import font "github.com/user3/proj3/font"
func main() { _ = font.Three }
`
		if _, err := i.Eval(src); err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
	}
}
