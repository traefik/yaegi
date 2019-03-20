package pkg

import (
	"path/filepath"
	"testing"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
)

func TestPackages(t *testing.T) {
	testCases := []struct {
		desc     string
		goPath   string
		expected string
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
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {

			goPath, err := filepath.Abs(test.goPath)
			if err != nil {
				t.Fatal(err)
			}

			// Init go interpreter
			i := interp.New(interp.Opt{
				GoPath: goPath,
			})
			i.Use(stdlib.Value) // Use binary standard library

			// Load pkg from sources
			if _, err = i.Eval(`import "github.com/foo/pkg"`); err != nil {
				t.Fatal(err)
			}

			value, err := i.Eval(`pkg.NewSample()`)
			if err != nil {
				t.Fatal(err)
			}

			fn := value.Interface().(func() string)

			msg := fn()

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
			expected: "found packages pkg and pkgfalse in _pkg9/src/github.com/foo/pkg",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {

			// Init go interpreter
			i := interp.New(interp.Opt{
				GoPath: test.goPath,
			})
			i.Use(stdlib.Value) // Use binary standard library

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
