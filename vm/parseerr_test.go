package vm

import (
	"strings"
	"testing"
)

// TestSyntaxErrorsAsGc: a NUL character or bytes that are not UTF-8 are
// reported once, as gc does: not again as an ILLEGAL token of go/scanner
// ("illegal character U+0000"), nor as the tokens that go/parser expected
// there ("expected declaration, found 'ILLEGAL'"), nor as type errors of
// what go/parser could read ("undefined: A"). GOROOT test nul1.go.
func TestSyntaxErrorsAsGc(t *testing.T) {
	src := "package main\n\nvar x = 1\n\n/* in source code */ \x00// NUL\n\nvar z\xc1\x81 int\n\nvar \xc2A int\n\nfunc main() {}\n"
	dir := writeModule(t, map[string]string{
		"go.mod":       "module example.com/t\n\ngo 1.22\n",
		"main/main.go": src,
		"plug/plug.go": strings.Replace(src, "package main", "package plug", 1),
	})
	check := func(t *testing.T, err error, want ...string) {
		t.Helper()
		if err == nil {
			t.Fatal("no error")
		}
		msg := err.Error()
		for _, w := range want {
			if !strings.Contains(msg, w) {
				t.Errorf("no %q in the error:\n%s", w, msg)
			}
		}
		for _, bad := range []string{"ILLEGAL", "U+0000", "U+FFFD", "expected", "undefined"} {
			if strings.Contains(msg, bad) {
				t.Errorf("follow-on error %q in the error:\n%s", bad, msg)
			}
		}
	}
	t.Run("runmain", func(t *testing.T) {
		_, err := RunMain(RunOptions{Dir: dir}, "./main")
		check(t, err, "main.go:5:22: illegal character NUL", "main.go:7:6: illegal UTF-8 encoding", "main.go:9:5: illegal UTF-8 encoding")
	})
	t.Run("load", func(t *testing.T) {
		// this loader reports the first syntax error of a file
		_, err := Load(Options{Dir: dir}, "example.com/t/plug")
		check(t, err, "plug.go:5:22: illegal character NUL")
	})
}

// TestSyntaxErrorInImport: a package that imports a package with a syntax
// error gets no type error (gc does not compile it): the syntax error of
// dep loses the declaration of G, and "undefined: dep.G" in the importer
// would only repeat it.
func TestSyntaxErrorInImport(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":       "module example.com/t\n\ngo 1.22\n",
		"dep/dep.go":   "package dep\n\nfunc F() int { return 1\n\nfunc G() string { return \"g\" }\n",
		"main/main.go": "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/t/dep\"\n)\n\nfunc main() { fmt.Println(dep.F(), dep.G()) }\n",
	})
	_, err := RunMain(RunOptions{Dir: dir}, "./main")
	if err == nil {
		t.Fatal("no error")
	}
	msg := err.Error()
	if want := "dep.go:5:6: expected '(', found G"; !strings.Contains(msg, want) {
		t.Errorf("no %q in the error:\n%s", want, msg)
	}
	if strings.Contains(msg, "undefined") {
		t.Errorf("follow-on type error in the importer:\n%s", msg)
	}
}
