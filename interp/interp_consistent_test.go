package interp_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

func TestInterpConsistent(t *testing.T) {
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
			file.Name() == "time0.go" || // display time (similar to random number)
			file.Name() == "time1.go" || // display time (similar to random number)

			file.Name() == "interface0.go" || // TODO not implemented yet
			file.Name() == "heap.go" || // TODO not implemented yet
			file.Name() == "bltn.go" || // TODO not implemented yet
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

			// read and restore stdout
			err = w.Close()
			if err != nil {
				t.Fatal(err)
			}
			outInterp, err := ioutil.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}

			// Restore Stdout
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
