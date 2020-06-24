package pkg

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func TestPackages(t *testing.T) {
	testCases := []struct {
		desc      string
		goPath    string
		expected  string
		topImport string
		evalFile  string
	}{
		{
			desc:     "vendor",
			goPath:   "./_pkg/",
			expected: "root Fromage",
		},
		{
			desc:     "sub-subpackage",
			goPath:   "./_pkg0/",
			expected: "root Fromage Cheese",
		},
		{
			desc:     "subpackage",
			goPath:   "./_pkg1/",
			expected: "root Fromage!",
		},
		{
			desc:     "multiple vendor folders",
			goPath:   "./_pkg2/",
			expected: "root Fromage Cheese!",
		},
		{
			desc:     "multiple vendor folders and subpackage in vendor",
			goPath:   "./_pkg3/",
			expected: "root Fromage Couteau Cheese!",
		},
		{
			desc:     "multiple vendor folders and multiple subpackages in vendor",
			goPath:   "./_pkg4/",
			expected: "root Fromage Cheese Vin! Couteau",
		},
		{
			desc:     "vendor flat",
			goPath:   "./_pkg5/",
			expected: "root Fromage Cheese Vin! Couteau",
		},
		{
			desc:     "fallback to GOPATH",
			goPath:   "./_pkg6/",
			expected: "root Fromage Cheese Vin! Couteau",
		},
		{
			desc:     "recursive vendor",
			goPath:   "./_pkg7/",
			expected: "root vin cheese fromage",
		},
		{
			desc:     "named subpackage",
			goPath:   "./_pkg8/",
			expected: "root Fromage!",
		},
		{
			desc:      "at the project root",
			goPath:    "./_pkg10/",
			expected:  "root Fromage",
			topImport: "github.com/foo",
		},
		{
			desc:     "eval main that has vendored dep",
			goPath:   "./_pkg11/",
			expected: "Fromage",
			evalFile: "./_pkg11/src/foo/foo.go",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			goPath, err := filepath.Abs(test.goPath)
			if err != nil {
				t.Fatal(err)
			}

			// Init go interpreter
			i := interp.New(interp.Options{GoPath: goPath})
			i.Use(stdlib.Symbols) // Use binary standard library

			var msg string
			if test.evalFile != "" {
				// setting i.Name as this is how it's actually done in cmd/yaegi
				i.Name = test.evalFile
				data, err := ioutil.ReadFile(test.evalFile)
				if err != nil {
					t.Fatal(err)
				}

				// TODO(mpl): this is brittle if we do concurrent tests and stuff, do better later.
				stdout := os.Stdout
				defer func() { os.Stdout = stdout }()
				pr, pw, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				os.Stdout = pw

				if _, err := i.Eval(string(data)); err != nil {
					t.Fatal(err)
				}

				outC := make(chan string)
				go func() {
					var buf bytes.Buffer
					if _, err := io.Copy(&buf, pr); err != nil {
						t.Fatal(err)
					}
					outC <- buf.String()
				}()

				pw.Close()
				msg = <-outC
			} else {
				// Load pkg from sources
				topImport := "github.com/foo/pkg"
				if test.topImport != "" {
					topImport = test.topImport
				}
				if _, err = i.Eval(fmt.Sprintf(`import "%s"`, topImport)); err != nil {
					t.Fatal(err)
				}
				value, err := i.Eval(`pkg.NewSample()`)
				if err != nil {
					t.Fatal(err)
				}

				fn := value.Interface().(func() string)

				msg = fn()
			}

			if msg != test.expected {
				t.Errorf("Got %q, want %q", msg, test.expected)
			}
		})
	}
}

func TestPackagesError(t *testing.T) {
	testCases := []struct {
		desc     string
		goPath   string
		expected string
	}{
		{
			desc:     "different packages in the same directory",
			goPath:   "./_pkg9/",
			expected: "1:21: import \"github.com/foo/pkg\" error: found packages pkg and pkgfalse in _pkg9/src/github.com/foo/pkg",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			// Init go interpreter
			i := interp.New(interp.Options{GoPath: test.goPath})
			i.Use(stdlib.Symbols) // Use binary standard library

			// Load pkg from sources
			_, err := i.Eval(`import "github.com/foo/pkg"`)
			if err == nil {
				t.Fatalf("got no error, want %q", test.expected)
			}

			if err.Error() != test.expected {
				t.Errorf("got %q, want %q", err.Error(), test.expected)
			}
		})
	}
}
