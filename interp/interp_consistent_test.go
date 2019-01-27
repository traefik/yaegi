package interp_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

func TestInterpConsistency(t *testing.T) {
	baseDir := filepath.Join("..", "_test")
	files, err := ioutil.ReadDir(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".go" ||
			file.Name() == "export1.go" || // non-main package
			file.Name() == "export0.go" || // non-main package
			file.Name() == "io0.go" || // use random number
			file.Name() == "op1.go" || // expect error
			file.Name() == "bltn0.go" || // expect error
			file.Name() == "time0.go" || // display time (similar to random number)
			file.Name() == "time1.go" || // display time (similar to random number)
			file.Name() == "time2.go" || // display time (similar to random number)

			file.Name() == "cli1.go" || // FIXME global vars
			file.Name() == "interface0.go" || // TODO not implemented yet
			file.Name() == "heap.go" || // TODO not implemented yet
			file.Name() == "chan6.go" || // FIXME related to channel #7
			file.Name() == "select1.go" || // FIXME related to channel #7
			file.Name() == "ret1.go" || // TODO not implemented yet #22
			file.Name() == "time3.go" || // FIXME only hour is printed, and other returned values minute and second are skipped.
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

			i := interp.New(interp.Opt{Entry: "main"})
			i.Use(stdlib.Value, stdlib.Type)

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

			cmd := exec.Command("go", "run", filePath)
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
			fileName:       "op1.go",
			expectedInterp: "5:7: invalid float truncate",
			expectedExec:   "5:4: constant 1.3 truncated to integer",
		},
		{
			fileName:       "bltn0.go",
			expectedInterp: "4:7: use of builtin println not in function call",
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

			i := interp.New(interp.Opt{Entry: "main"})
			i.Use(stdlib.Value, stdlib.Type)

			_, errEval := i.Eval(string(src))
			if errEval == nil {
				t.Fatal("An error is expected but got none.")
			}

			if errEval.Error() != test.expectedInterp {
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
