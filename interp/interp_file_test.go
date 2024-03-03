package interp_test

import (
	"bytes"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/unsafe"
)

// The following tests sometimes (not always) crash with go1.21 but not with go1.20 or go1.22.
// The reason of failure is not obvious, maybe due to the runtime itself, and will be investigated separately.
var testsToSkipGo121 = map[string]bool{"cli6.go": true, "cli7.go": true, "issue-1276.go": true, "issue-1330.go": true, "struct11.go": true}

var go121 = strings.HasPrefix(runtime.Version(), "go1.21")

func TestFile(t *testing.T) {
	filePath := "../_test/str.go"
	runCheck(t, filePath)

	t.Setenv("YAEGI_SPECIAL_STDIO", "1")

	baseDir := filepath.Join("..", "_test")
	files, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".go" {
			continue
		}
		// Skip some tests which are problematic in go1.21 only.
		if go121 && testsToSkipGo121[file.Name()] {
			continue
		}
		file := file
		t.Run(file.Name(), func(t *testing.T) {
			runCheck(t, filepath.Join(baseDir, file.Name()))
		})
	}
}

func runCheck(t *testing.T, p string) {
	t.Helper()

	wanted, goPath, errWanted := wantedFromComment(p)
	if wanted == "" {
		t.Skip(p, "has no comment 'Output:' or 'Error:'")
	}
	wanted = strings.TrimSpace(wanted)

	if goPath == "" {
		goPath = build.Default.GOPATH
	}
	var stdout, stderr bytes.Buffer
	i := interp.New(interp.Options{GoPath: goPath, Stdout: &stdout, Stderr: &stderr})
	if err := i.Use(interp.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(unsafe.Symbols); err != nil {
		t.Fatal(err)
	}

	_, err := i.EvalPath(p)
	if errWanted {
		if err == nil {
			t.Fatalf("got nil error, want: %q", wanted)
		}
		if res := strings.TrimSpace(err.Error()); !strings.Contains(res, wanted) {
			t.Errorf("got %q, want: %q", res, wanted)
		}
		return
	}

	if err != nil {
		t.Fatal(err)
	}

	// Remove path in output, to have results independent of location.
	re := regexp.MustCompile(p + ":")
	if res := re.ReplaceAllString(strings.TrimSpace(stdout.String()), ""); res != wanted {
		t.Errorf("\ngot:  %q,\nwant: %q", res, wanted)
	}
}

func wantedFromComment(p string) (res string, goPath string, err bool) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, p, nil, parser.ParseComments)
	if len(f.Comments) == 0 {
		return
	}
	text := f.Comments[len(f.Comments)-1].Text()
	if strings.HasPrefix(text, "GOPATH:") {
		parts := strings.SplitN(text, "\n", 2)
		text = parts[1]
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		goPath = filepath.Join(wd, "..", "_test", strings.TrimPrefix(parts[0], "GOPATH:"))
	}
	if strings.HasPrefix(text, "Output:\n") {
		return strings.TrimPrefix(text, "Output:\n"), goPath, false
	}
	if strings.HasPrefix(text, "Error:\n") {
		return strings.TrimPrefix(text, "Error:\n"), goPath, true
	}
	return
}
