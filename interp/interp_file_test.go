package interp_test

import (
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func TestFile(t *testing.T) {
	filePath := "../_test/str.go"
	runCheck(t, filePath)

	baseDir := filepath.Join("..", "_test")
	files, err := ioutil.ReadDir(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".go" {
			continue
		}
		file := file
		t.Run(file.Name(), func(t *testing.T) {
			runCheck(t, filepath.Join(baseDir, file.Name()))
		})
	}
}

func runCheck(t *testing.T, p string) {
	wanted, errWanted := wantedFromComment(p)
	if wanted == "" {
		t.Skip(p, "has no comment 'Output:' or 'Error:'")
	}
	wanted = strings.TrimSpace(wanted)

	src, err := ioutil.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	// catch stdout
	backupStdout := os.Stdout
	defer func() { os.Stdout = backupStdout }()
	r, w, _ := os.Pipe()
	os.Stdout = w

	i := interp.New()
	i.Name = p
	i.Use(interp.Symbols)
	i.Use(stdlib.Symbols)

	_, err = i.Eval(string(src))
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

	// read stdout
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	outInterp, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if res := strings.TrimSpace(string(outInterp)); res != wanted {
		t.Errorf("\ngot:  %q,\nwant: %q", res, wanted)
	}
}

func wantedFromComment(p string) (res string, err bool) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, p, nil, parser.ParseComments)
	if len(f.Comments) == 0 {
		return
	}
	text := f.Comments[len(f.Comments)-1].Text()
	if strings.HasPrefix(text, "Output:\n") {
		return strings.TrimPrefix(text, "Output:\n"), false
	}
	if strings.HasPrefix(text, "Error:\n") {
		return strings.TrimPrefix(text, "Error:\n"), true
	}
	return
}
