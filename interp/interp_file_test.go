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
	wanted := wantedFromComment(p)
	if wanted == "" {
		t.Skip(p, "has no block '// Output:'")
	}

	src, err := ioutil.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	// catch stdout
	backupStdout := os.Stdout
	defer func() { os.Stdout = backupStdout }()
	r, w, _ := os.Pipe()
	os.Stdout = w

	i := interp.New(interp.Opt{Entry: "main"})
	i.Use(stdlib.Value)

	_, err = i.Eval(string(src))
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
	if strings.TrimSpace(string(outInterp)) != strings.TrimSpace(wanted) {
		t.Errorf("\ngot:  %q,\nwant: %q", string(outInterp), wanted)
	}
}

func wantedFromComment(p string) (res string) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, p, nil, parser.ParseComments)
	if len(f.Comments) == 0 {
		return
	}
	// wanted output text is in last block comment and start by: '// Output:'
	last := f.Comments[len(f.Comments)-1].List
	if header := last[0].Text; header != "// Output:" {
		return
	}
	for _, l := range last[1:] {
		res += l.Text[3:] + "\n"
	}
	return
}
