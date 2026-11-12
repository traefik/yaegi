//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

// TestExecuteTimeout checks that a timeout kills the whole process group:
// the shell prints the pid of its background sleep, then waits for it.
func TestExecuteTimeout(t *testing.T) {
	start := time.Now()
	o, _, timedOut, _ := execute(t.TempDir(), []string{"sh", "-c", "sleep 300 & echo $!; wait"}, nil, time.Second)
	if !timedOut {
		t.Fatalf("timedOut = false, output %q", o.out)
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("execute took %v after a 1s timeout", d)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(o.out))
	if err != nil {
		t.Fatalf("no pid in the output %q", o.out)
	}
	// The killed sleep is a zombie until its new parent (init) reaps it:
	// wait a little for its pid to go away.
	for i := 0; ; i++ {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			break
		}
		if b, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); strings.Contains(string(b), ") Z ") {
			break // dead, not reaped yet
		}
		if i == 100 {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the grandchild sleep (pid %d) is still running: %v", pid, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestExecuteNoTimeout(t *testing.T) {
	_, code, timedOut, err := execute(t.TempDir(), []string{"sh", "-c", "exit 0"}, nil, 30*time.Second)
	if timedOut || code != 0 || err != nil {
		t.Errorf("timedOut = %v, code = %d, err = %v", timedOut, code, err)
	}
	_, code, timedOut, _ = execute(t.TempDir(), []string{"sh", "-c", "exit 3"}, nil, 30*time.Second)
	if timedOut || code != 3 {
		t.Errorf("exit 3: timedOut = %v, code = %d", timedOut, code)
	}
}

func TestCheckNoModule(t *testing.T) {
	dir := t.TempDir()
	if err := checkNoModule(dir); err != nil {
		t.Fatalf("directory without go.mod refused: %v", err)
	}
	mod := filepath.Join(dir, "m")
	sub := filepath.Join(mod, "test", "fixedbugs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module m\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{mod, sub} {
		if err := checkNoModule(d); err == nil {
			t.Errorf("%s: directory inside a module accepted", d)
		}
	}
}

func TestCheckErrors(t *testing.T) {
	src := "package p\n\nvar x int = \"s\" // ERROR \"cannot use|incompatible\"\n"
	for _, tc := range []struct {
		name, engine, out string
		code              int
		want              string
	}{
		{"pass", "govm", "fixedbugs/bug1.go:3:13: cannot use \"s\" (untyped string constant) as int value", 4, "PASS"},
		{"pass with full path", "govm", "/r/fixedbugs/bug1.go:3:13: cannot use \"s\"", 4, "PASS"},
		{"wrong message", "yaegi", "run: fixedbugs/bug1.go:3:5: bad thing (and 2 more errors)", 1, "ERR_WRONGMSG"},
		{"other file", "govm", "fixedbugs/xbug1.go:3:13: cannot use", 4, "ERR_WRONGLINE"},
		{"wrong line", "govm", "fixedbugs/bug1.go:3:13: cannot use\nfixedbugs/bug1.go:1:1: cannot use", 4, "ERR_WRONGLINE"},
		{"crash", "yaegi", "panic: fixedbugs/bug1.go:3:6: CFG post-order panic: cannot use", 2, "ERR_CRASH"},
		{"panic text with the reject code", "yaegi", "fixedbugs/bug1.go:3:13: cannot use\npanic: boom", 1, "ERR_CRASH"},
		{"unsupported", "govm", "yaegi: unsupported: x", 3, "ERR_CRASH"},
		{"accepted", "govm", "", 0, "ERR_MISSED"},
		{"continuation", "govm", "fixedbugs/bug1.go:3:13: cannot use\nfixedbugs/bug1.go:1:1: \tother declaration of x", 4, "PASS"},
	} {
		var err error
		if tc.code != 0 {
			err = errExit
		}
		got, detail := checkErrors(tc.engine, "/r", "fixedbugs/bug1.go", src, tc.out, tc.code, err)
		if got != tc.want {
			t.Errorf("%s: got %s (%s), want %s", tc.name, got, detail, tc.want)
		}
	}
}

// TestCheckErrorsPathInMessage: the path of the file in a message becomes
// its base name, as in testdir (fixedbugs/issue68292.go).
func TestCheckErrorsPathInMessage(t *testing.T) {
	src := "package p\n\nfunc f() { g() } // ERROR \"declared at bug1.go:3:6\"\n"
	for _, out := range []string{
		"fixedbugs/bug1.go:3:12: x (declared at fixedbugs/bug1.go:3:6)",
		"./fixedbugs/bug1.go:3:12: x (declared at ./fixedbugs/bug1.go:3:6)",
	} {
		if got, detail := checkErrors("govm", "/r", "fixedbugs/bug1.go", src, out, 4, errExit); got != "PASS" {
			t.Errorf("%q: got %s (%s), want PASS", out, got, detail)
		}
	}
}

func TestCheckErrorsNotMain(t *testing.T) {
	src := "package p\n\nvar x int = \"s\" // ERROR \"cannot use\"\n"
	for _, out := range []string{
		"load: command-line-arguments is not a main package",
		"fixedbugs/bug1.go:1:1: package p; expected package main",
	} {
		got, detail := checkErrors("govm", "/r", "fixedbugs/bug1.go", src, out, 4, errExit)
		if got != "ERR_MISSED" {
			t.Errorf("%q: got %s (%s), want ERR_MISSED", out, got, detail)
		}
	}
}

func TestCheckErrorsFiles(t *testing.T) {
	files := []srcFile{
		{abs: "/t/src/test/a/a.go", base: "a.go", src: "package a\n\nvar X int\n"},
		{abs: "/t/src/test/b/b.go", base: "b.go", src: "package b\n\nimport \"test/a\"\n\nvar y string = a.X // ERROR \"cannot use\"\n"},
	}
	for _, tc := range []struct{ name, out, want string }{
		{"pass", "/t/src/test/b/b.go:5:16: cannot use a.X (variable of type int) as string value", "PASS"},
		{"relative path", "b/b.go:5:16: cannot use a.X", "PASS"},
		{"error in the other package", "a/a.go:3:5: cannot use", "ERR_WRONGLINE"},
		{"wrong message", "b/b.go:5:16: undefined: a.X", "ERR_WRONGMSG"},
	} {
		got, detail := checkErrorsFiles("govm", tc.out, 4, errExit, files)
		if got != tc.want {
			t.Errorf("%s: got %s (%s), want %s", tc.name, got, detail, tc.want)
		}
	}
}

func TestRewriteImports(t *testing.T) {
	src := "package main\n\nimport \"./a\"\nimport (\n\tb \"./b\"\n\t\"fmt\"\n\t. \"./c\"\n)\n\n// \"./d\" in a comment\nvar s = \"./e\"\n"
	want := "package main\n\nimport \"test/a\"\nimport (\n\tb \"test/b\"\n\t\"fmt\"\n\t. \"test/c\"\n)\n\n// \"./d\" in a comment\nvar s = \"./e\"\n"
	if got := string(rewriteImports([]byte(src))); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestDirPackages(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"a.go": "package a\n", "a2.go": "package a\n", "b_test.go": "package b\n", "main.go": "// comment\npackage main\n", "x.s": "",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pkgs, err := dirPackages(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range pkgs {
		got = append(got, p.name+"="+strings.Join(p.files, "+")+"@"+p.pkgPath())
	}
	if want := "a=a.go+a2.go@test/a b=b_test.go@test/b_test main=main.go@test/main"; strings.Join(got, " ") != want {
		t.Errorf("got %s, want %s", strings.Join(got, " "), want)
	}
	if pkgs, _ := dirPackages(dir, true); len(pkgs) != 4 {
		t.Errorf("-s: %d packages, want 4", len(pkgs))
	}
	if got := layoutName("b_test.go"); got != "b_test_.go" {
		t.Errorf("layoutName = %s", got)
	}
}

var errExit = &exitErr{}

type exitErr struct{}

func (*exitErr) Error() string { return "exit status 1" }

func TestCut(t *testing.T) {
	s := strings.Repeat("é", 10) // 2 bytes per rune
	for n := 0; n <= len(s)+1; n++ {
		c := cut(s, n)
		if !utf8.ValidString(c) || len(c) > n {
			t.Errorf("cut(%d) = %q", n, c)
		}
	}
}

func TestFirstDiff(t *testing.T) {
	got := firstDiff("a\nb\nc\n", "a\nb\nd\n")
	if want := `line 3: got "c\n" want "d\n"`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestClean(t *testing.T) {
	initCleaner("/x/goroot/test")
	got := clean("/x/goroot/test/a.go /tmp/abc/def/b.go /home/someone/c.go \x00")
	if want := "$GOROOT/test/a.go $TMP/b.go $HOME/c.go ?"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
