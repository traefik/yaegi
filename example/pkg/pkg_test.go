package pkg

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
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
		{
			desc:      "vendor dir is a sibling or an uncle",
			goPath:    "./_pkg12/",
			expected:  "Yo hello",
			topImport: "guthib.com/foo/pkg",
		},
		{
			desc:     "eval main with vendor as a sibling",
			goPath:   "./_pkg12/",
			expected: "Yo hello",
			evalFile: "./_pkg12/src/guthib.com/foo/main.go",
		},
		{
			desc:     "eval main with vendor",
			goPath:   "./_pkg13/",
			expected: "foobar",
			evalFile: "./_pkg13/src/guthib.com/foo/bar/main.go",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			goPath, err := filepath.Abs(filepath.FromSlash(test.goPath))
			if err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			i := interp.New(interp.Options{GoPath: goPath, Stdout: &stdout, Stderr: &stderr})
			// Use binary standard library
			if err := i.Use(stdlib.Symbols); err != nil {
				t.Fatal(err)
			}

			var msg string
			if test.evalFile != "" {
				if _, err := i.EvalPath(filepath.FromSlash(test.evalFile)); err != nil {
					fatalStderrf(t, "%v", err)
				}
				msg = stdout.String()
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
				fatalStderrf(t, "Got %q, want %q", msg, test.expected)
			}
		})
	}
}

func fatalStderrf(t *testing.T, format string, args ...interface{}) {
	t.Helper()

	fmt.Fprintf(os.Stderr, format+"\n", args...)
	t.FailNow()
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
			expected: `1:21: import "github.com/foo/pkg" error: found packages pkg and pkgfalse in ` + filepath.FromSlash("_pkg9/src/github.com/foo/pkg"),
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			// Init go interpreter
			i := interp.New(interp.Options{GoPath: test.goPath})
			// Use binary standard library
			if err := i.Use(stdlib.Symbols); err != nil {
				t.Fatal(err)
			}

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
