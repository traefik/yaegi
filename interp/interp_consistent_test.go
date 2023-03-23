package interp_test

import (
	"go/build"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/unsafe"
)

func TestInterpConsistencyBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	dir := filepath.Join("..", "_test", "tmp")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	baseDir := filepath.Join("..", "_test")
	files, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".go" ||
			file.Name() == "assign11.go" || // expect error
			file.Name() == "assign12.go" || // expect error
			file.Name() == "assign15.go" || // expect error
			file.Name() == "bad0.go" || // expect error
			file.Name() == "break0.go" || // expect error
			file.Name() == "cont3.go" || // expect error
			file.Name() == "const9.go" || // expect error
			file.Name() == "export1.go" || // non-main package
			file.Name() == "export0.go" || // non-main package
			file.Name() == "for7.go" || // expect error
			file.Name() == "fun21.go" || // expect error
			file.Name() == "fun22.go" || // expect error
			file.Name() == "fun23.go" || // expect error
			file.Name() == "fun24.go" || // expect error
			file.Name() == "fun25.go" || // expect error
			file.Name() == "gen7.go" || // expect error
			file.Name() == "gen8.go" || // expect error
			file.Name() == "gen9.go" || // expect error
			file.Name() == "if2.go" || // expect error
			file.Name() == "import6.go" || // expect error
			file.Name() == "init1.go" || // expect error
			file.Name() == "io0.go" || // use random number
			file.Name() == "issue-1093.go" || // expect error
			file.Name() == "issue-1276.go" || // expect error
			file.Name() == "issue-1330.go" || // expect error
			file.Name() == "op1.go" || // expect error
			file.Name() == "op7.go" || // expect error
			file.Name() == "op9.go" || // expect error
			file.Name() == "bltn0.go" || // expect error
			file.Name() == "method16.go" || // private struct field
			file.Name() == "method39.go" || // expect error
			file.Name() == "switch8.go" || // expect error
			file.Name() == "switch9.go" || // expect error
			file.Name() == "switch13.go" || // expect error
			file.Name() == "switch19.go" || // expect error
			file.Name() == "time0.go" || // display time (similar to random number)
			file.Name() == "factor.go" || // bench
			file.Name() == "fib.go" || // bench

			file.Name() == "type5.go" || // used to illustrate a limitation with no workaround, related to the fact that the reflect package does not allow the creation of named types
			file.Name() == "type6.go" || // used to illustrate a limitation with no workaround, related to the fact that the reflect package does not allow the creation of named types

			file.Name() == "redeclaration0.go" || // expect error
			file.Name() == "redeclaration1.go" || // expect error
			file.Name() == "redeclaration2.go" || // expect error
			file.Name() == "redeclaration3.go" || // expect error
			file.Name() == "redeclaration4.go" || // expect error
			file.Name() == "redeclaration5.go" || // expect error
			file.Name() == "redeclaration-global0.go" || // expect error
			file.Name() == "redeclaration-global1.go" || // expect error
			file.Name() == "redeclaration-global2.go" || // expect error
			file.Name() == "redeclaration-global3.go" || // expect error
			file.Name() == "redeclaration-global4.go" || // expect error
			file.Name() == "redeclaration-global5.go" || // expect error
			file.Name() == "redeclaration-global6.go" || // expect error
			file.Name() == "redeclaration-global7.go" || // expect error
			file.Name() == "pkgname0.go" || // has deps
			file.Name() == "pkgname1.go" || // expect error
			file.Name() == "pkgname2.go" || // has deps
			file.Name() == "ipp_as_key.go" || // has deps
			file.Name() == "restricted0.go" || // expect error
			file.Name() == "restricted1.go" || // expect error
			file.Name() == "restricted2.go" || // expect error
			file.Name() == "restricted3.go" || // expect error
			file.Name() == "server6.go" || // syntax parsing
			file.Name() == "server5.go" || // syntax parsing
			file.Name() == "server4.go" || // syntax parsing
			file.Name() == "server3.go" || // syntax parsing
			file.Name() == "server2.go" || // syntax parsing
			file.Name() == "server1a.go" || // syntax parsing
			file.Name() == "server1.go" || // syntax parsing
			file.Name() == "server0.go" || // syntax parsing
			file.Name() == "server.go" || // syntax parsing
			file.Name() == "range9.go" || // expect error
			file.Name() == "unsafe6.go" || // needs go.mod to be 1.17
			file.Name() == "unsafe7.go" || // needs go.mod to be 1.17
			file.Name() == "type24.go" || // expect error
			file.Name() == "type27.go" || // expect error
			file.Name() == "type28.go" || // expect error
			file.Name() == "type29.go" || // expect error
			file.Name() == "type30.go" || // expect error
			file.Name() == "type31.go" || // expect error
			file.Name() == "type32.go" || // expect error
			file.Name() == "type33.go" { // expect error
			continue
		}

		file := file
		t.Run(file.Name(), func(t *testing.T) {
			filePath := filepath.Join(baseDir, file.Name())

			// catch stdout
			backupStdout := os.Stdout
			defer func() {
				os.Stdout = backupStdout
			}()
			r, w, _ := os.Pipe()
			os.Stdout = w

			i := interp.New(interp.Options{GoPath: build.Default.GOPATH})
			if err := i.Use(stdlib.Symbols); err != nil {
				t.Fatal(err)
			}
			if err := i.Use(interp.Symbols); err != nil {
				t.Fatal(err)
			}
			if err := i.Use(unsafe.Symbols); err != nil {
				t.Fatal(err)
			}

			_, err = i.EvalPath(filePath)
			if err != nil {
				t.Fatal(err)
			}

			// read stdout
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			outInterp, err := io.ReadAll(r)
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
			fileName:       "assign11.go",
			expectedInterp: "6:2: assignment mismatch: 3 variables but fmt.Println returns 2 values",
			expectedExec:   "6:12: assignment mismatch: 3 variables but fmt.Println returns 2 values",
		},
		{
			fileName:       "assign12.go",
			expectedInterp: "6:2: assignment mismatch: 3 variables but fmt.Println returns 2 values",
			expectedExec:   "6:13: assignment mismatch: 3 variables but fmt.Println returns 2 values",
		},
		{
			fileName:       "bad0.go",
			expectedInterp: "1:1: expected 'package', found println",
			expectedExec:   "1:1: expected 'package', found println",
		},
		{
			fileName:       "break0.go",
			expectedInterp: "15:5: invalid break label OuterLoop",
			expectedExec:   "15:11: invalid break label OuterLoop",
		},
		{
			fileName:       "cont3.go",
			expectedInterp: "15:5: invalid continue label OuterLoop",
			expectedExec:   "15:14: invalid continue label OuterLoop",
		},
		{
			fileName:       "const9.go",
			expectedInterp: "5:2: constant definition loop",
			expectedExec:   "5:2: initialization",
		},
		{
			fileName:       "if2.go",
			expectedInterp: "7:5: non-bool used as if condition",
			expectedExec:   "7:5: non-boolean condition in if statement",
		},
		{
			fileName:       "for7.go",
			expectedInterp: "4:14: non-bool used as for condition",
			expectedExec:   "4:14: non-boolean condition in for statement",
		},
		{
			fileName:       "fun21.go",
			expectedInterp: "4:2: not enough arguments to return",
			expectedExec:   "4:2: not enough return values",
		},
		{
			fileName:       "fun22.go",
			expectedInterp: "6:2: not enough arguments in call to time.Date",
			expectedExec:   "6:2: not enough arguments in call to time.Date",
		},
		{
			fileName:       "fun23.go",
			expectedInterp: "3:17: too many arguments to return",
			expectedExec:   "3:24: too many return values",
		},
		{
			fileName:       "issue-1093.go",
			expectedInterp: "9:6: cannot use type untyped string as type int in assignment",
			expectedExec:   `9:6: cannot use "a" + b() (value of type string)`,
		},
		{
			fileName:       "op1.go",
			expectedInterp: "5:2: invalid operation: mismatched types int and untyped float",
			expectedExec:   "5:7: 1.3 (untyped float constant) truncated to int",
		},
		{
			fileName:       "bltn0.go",
			expectedInterp: "4:7: use of builtin println not in function call",
			expectedExec:   "4:7: println (built-in) must be called",
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
			expectedExec:   "fallthrough",
		},
		{
			fileName:       "switch13.go",
			expectedInterp: "9:2: i is not a type",
			expectedExec:   "9:7: i (variable of type interface{}) is not a type",
		},
		{
			fileName:       "switch19.go",
			expectedInterp: "37:2: duplicate case Bir in type switch",
			expectedExec:   "37:7: duplicate case Bir in type switch",
		},
	}

	for _, test := range testCases {
		t.Run(test.fileName, func(t *testing.T) {
			if len(test.expectedInterp) == 0 && len(test.expectedExec) == 0 {
				t.Fatal("at least expectedInterp must be define")
			}

			filePath := filepath.Join("..", "_test", test.fileName)

			i := interp.New(interp.Options{GoPath: build.Default.GOPATH})
			if err := i.Use(stdlib.Symbols); err != nil {
				t.Fatal(err)
			}

			_, errEval := i.EvalPath(filePath)
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
