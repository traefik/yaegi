package interp_test

import (
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func TestInterpConsistencyBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	dir := filepath.Join("..", "_test", "tmp")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	baseDir := filepath.Join("..", "_test")
	files, err := ioutil.ReadDir(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".go" ||
			file.Name() == "bad0.go" || // expect error
			file.Name() == "export1.go" || // non-main package
			file.Name() == "export0.go" || // non-main package
			file.Name() == "import6.go" || // expect error
			file.Name() == "io0.go" || // use random number
			file.Name() == "op1.go" || // expect error
			file.Name() == "bltn0.go" || // expect error
			file.Name() == "method16.go" || // private struct field
			file.Name() == "switch8.go" || // expect error
			file.Name() == "switch9.go" || // expect error
			file.Name() == "switch13.go" || // expect error
			file.Name() == "switch19.go" || // expect error
			file.Name() == "time0.go" || // display time (similar to random number)
			file.Name() == "factor.go" || // bench
			file.Name() == "fib.go" || // bench

			file.Name() == "type5.go" || // used to illustrate a limitation with no workaround, related to the fact that the reflect package does not allow the creation of named types
			file.Name() == "type6.go" || // used to illustrate a limitation with no workaround, related to the fact that the reflect package does not allow the creation of named types

			file.Name() == "server6.go" || // syntax parsing
			file.Name() == "server5.go" || // syntax parsing
			file.Name() == "server4.go" || // syntax parsing
			file.Name() == "server3.go" || // syntax parsing
			file.Name() == "server2.go" || // syntax parsing
			file.Name() == "server1a.go" || // syntax parsing
			file.Name() == "server1.go" || // syntax parsing
			file.Name() == "server0.go" || // syntax parsing
			file.Name() == "server.go" { // syntax parsing
			continue
		}

		file := file
		t.Run(file.Name(), func(t *testing.T) {
			filePath := filepath.Join(baseDir, file.Name())

			src, err := ioutil.ReadFile(filePath)
			if err != nil {
				t.Fatal(err)
			}

			// catch stdout
			backupStdout := os.Stdout
			defer func() {
				os.Stdout = backupStdout
			}()
			r, w, _ := os.Pipe()
			os.Stdout = w

			i := interp.New(interp.Options{GoPath: build.Default.GOPATH})
			i.Name = filePath
			i.Use(stdlib.Symbols)
			i.Use(interp.Symbols)

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

			// restore Stdout
			os.Stdout = backupStdout

			bin := filepath.Join(dir, strings.TrimSuffix(file.Name(), ".go"))

			cmdBuild := exec.Command("go", "build", "-tags=dummy", "-o", bin, filePath)
			outBuild, err := cmdBuild.CombinedOutput()
			if err != nil {
				t.Log(string(outBuild))
				t.Fatal(err)
			}

			cmd := exec.Command(bin)
			outRun, err := cmd.CombinedOutput()
			if err != nil {
				t.Log(string(outRun))
				t.Fatal(err)
			}

			if string(outInterp) != string(outRun) {
				t.Errorf("\nGot: %q,\n want: %q", string(outInterp), string(outRun))
			}
		})
	}
}

func TestInterpErrorConsistency(t *testing.T) {
	testCases := []struct {
		fileName       string
		expectedInterp string
		expectedExec   string
	}{
		{
			fileName:       "bad0.go",
			expectedInterp: "1:1: expected 'package', found println",
			expectedExec:   "1:1: expected 'package', found println",
		},
		{
			fileName:       "op1.go",
			expectedInterp: "5:2: illegal operand types for '+=' operator",
			expectedExec:   "5:4: constant 1.3 truncated to integer",
		},
		{
			fileName:       "bltn0.go",
			expectedInterp: "4:7: use of builtin println not in function call",
		},
		{
			fileName:       "import6.go",
			expectedInterp: "import cycle not allowed",
			expectedExec:   "import cycle not allowed",
		},
		{
			fileName:       "switch8.go",
			expectedInterp: "5:2: fallthrough statement out of place",
			expectedExec:   "5:2: fallthrough statement out of place",
		},
		{
			fileName:       "switch9.go",
			expectedInterp: "9:3: cannot fallthrough in type switch",
			expectedExec:   "9:3: cannot fallthrough in type switch",
		},
		{
			fileName:       "switch13.go",
			expectedInterp: "9:2: i is not a type",
			expectedExec:   "9:2: i (type interface {}) is not a type",
		},
		{
			fileName:       "switch19.go",
			expectedInterp: "37:2: duplicate case Bir in type switch",
			expectedExec:   "37:2: duplicate case Bir in type switch",
		},
	}

	for _, test := range testCases {
		t.Run(test.fileName, func(t *testing.T) {
			if len(test.expectedInterp) == 0 && len(test.expectedExec) == 0 {
				t.Fatal("at least expectedInterp must be define")
			}

			filePath := filepath.Join("..", "_test", test.fileName)

			src, err := ioutil.ReadFile(filePath)
			if err != nil {
				t.Fatal(err)
			}

			i := interp.New(interp.Options{GoPath: build.Default.GOPATH})
			i.Name = filePath
			i.Use(stdlib.Symbols)

			_, errEval := i.Eval(string(src))
			if errEval == nil {
				t.Fatal("An error is expected but got none.")
			}

			if !strings.Contains(errEval.Error(), test.expectedInterp) {
				t.Errorf("got %q, want: %q", errEval.Error(), test.expectedInterp)
			}

			cmd := exec.Command("go", "run", filePath)
			outRun, errExec := cmd.CombinedOutput()
			if errExec == nil {
				t.Log(string(outRun))
				t.Fatal("An error is expected but got none.")
			}

			if len(test.expectedExec) == 0 && !strings.Contains(string(outRun), test.expectedInterp) {
				t.Errorf("got %q, want: %q", string(outRun), test.expectedInterp)
			} else if !strings.Contains(string(outRun), test.expectedExec) {
				t.Errorf("got %q, want: %q", string(outRun), test.expectedExec)
			}
		})
	}
}
