package vm

import (
	"regexp"
	"testing"
)

// TestSubErrorsJoined: the parts of a type error that have a position of
// their own (the steps of an invalid recursive type, the other declaration
// of a name) are printed as gc prints them, on lines that start with a tab
// after the first line of the error, not as errors of their own (GOROOT
// test fixedbugs/issue50788.go).
func TestSubErrorsJoined(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":       "module example.com/t\n\ngo 1.22\n",
		"a/a.go":       "package a\n\ntype T[P any] struct {\n\t_ P\n}\n",
		"plug/plug.go": "package plug\n\nimport \"example.com/t/a\"\n\ntype T a.T[T]\n\nvar x int\nvar x string\n",
		"main/main.go": "package main\n\nimport \"example.com/t/a\"\n\ntype T a.T[T]\n\nvar x int\nvar x string\n\nfunc main() {}\n",
	})
	// [/\\]: the paths have backslashes on Windows
	want := []*regexp.Regexp{
		regexp.MustCompile(`(?m)[/\\](plug|main)\.go:5:6: invalid recursive type T\n\t\S*[/\\](plug|main)\.go:5:6: T refers to a\.T(\[T\])?\n\t\S*[/\\](plug|main)\.go:5:8: a\.T(\[T\])? refers to T$`),
		regexp.MustCompile(`(?m)[/\\](plug|main)\.go:8:5: x redeclared in this block\n\t\S*[/\\](plug|main)\.go:7:5: other declaration of x$`),
	}
	notWant := regexp.MustCompile(`(?m)^\S+: \t`) // a part as an error of its own
	check := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("no error")
		}
		msg := err.Error()
		for _, rx := range want {
			if !rx.MatchString(msg) {
				t.Errorf("error does not match %s:\n%s", rx, msg)
			}
		}
		if notWant.MatchString(msg) {
			t.Errorf("a part of an error is an error of its own:\n%s", msg)
		}
	}
	t.Run("load", func(t *testing.T) {
		_, err := Load(Options{Dir: dir}, "example.com/t/plug")
		check(t, err)
	})
	t.Run("runmain", func(t *testing.T) {
		_, err := RunMain(RunOptions{Dir: dir}, "./main")
		check(t, err)
	})
}
