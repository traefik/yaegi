package interp

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func Test_effectivePkg(t *testing.T) {
	testCases := []struct {
		desc     string
		root     string
		path     string
		expected string
	}{
		{
			desc:     "path is a subpackage",
			root:     "github.com/foo/plugin/vendor/guthib.com/containous/fromage",
			path:     "guthib.com/containous/fromage/couteau/lol",
			expected: "github.com/foo/plugin/vendor/guthib.com/containous/fromage/couteau/lol",
		},
		{
			desc:     "path is a vendored package",
			root:     "github.com/foo/plugin/vendor/guthib.com/containous/fromage",
			path:     "vendor/guthib.com/containous/vin",
			expected: "github.com/foo/plugin/vendor/guthib.com/containous/fromage/vendor/guthib.com/containous/vin",
		},
		{
			desc:     "path is non-existent",
			root:     "foo",
			path:     "githib.com/foo/app",
			expected: "foo/githib.com/foo/app",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			pkg := effectivePkg(test.root, test.path)

			if pkg != test.expected {
				t.Errorf("Got %s, want %s", pkg, test.expected)
			}
		})
	}
}

func Test_pkgDir(t *testing.T) {
	// create GOPATH
	goPath, err := ioutil.TempDir("", "pkdir")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(goPath)
	}()

	// Create project
	project := filepath.Join(goPath, "src", "guthib.com", "foo", "root")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}

	type expected struct {
		dir   string
		rpath string
	}

	testCases := []struct {
		desc     string
		path     string
		root     string
		setup    func() error
		expected expected
	}{
		{
			desc: "GOPATH only",
			path: "guthib.com/foo/bar",
			root: "",
			setup: func() error {
				return os.MkdirAll(filepath.Join(goPath, "src", "guthib.com", "foo", "bar"), 0700)
			},
			expected: expected{
				dir:   filepath.Join(goPath, "src", "guthib.com", "foo", "bar"),
				rpath: "",
			},
		},
		{
			desc: "vendor",
			path: "guthib.com/foo/bar",
			root: filepath.Join("guthib.com", "foo", "root"),
			setup: func() error {
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bar"), 0700)
			},
			expected: expected{
				dir:   filepath.Join(goPath, "src", "guthib.com", "foo", "root", "vendor", "guthib.com", "foo", "bar"),
				rpath: filepath.Join("guthib.com", "foo", "root", "vendor"),
			},
		},
		{
			desc: "GOPATH flat",
			path: "guthib.com/foo/bar",
			root: filepath.Join("guthib.com", "foo", "root"),
			setup: func() error {
				return os.MkdirAll(filepath.Join(goPath, "src", "guthib.com", "foo", "bar"), 0700)
			},
			expected: expected{
				dir:   filepath.Join(goPath, "src", "guthib.com", "foo", "bar"),
				rpath: "",
			},
		},
		{
			desc: "vendor flat",
			path: "guthib.com/foo/bar",
			root: filepath.Join("guthib.com", "foo", "root", "vendor", "guthib.com", "foo", "bir"),
			setup: func() error {
				if err := os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bar"), 0700); err != nil {
					return err
				}
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bir"), 0700)
			},
			expected: expected{
				dir:   filepath.Join(goPath, "src", "guthib.com", "foo", "root", "vendor", "guthib.com", "foo", "bar"),
				rpath: filepath.Join("guthib.com", "foo", "root", "vendor"),
			},
		},
		{
			desc: "fallback to GOPATH",
			path: "guthib.com/foo/bar",
			root: filepath.Join("guthib.com", "foo", "root", "vendor", "guthib.com", "foo", "bir"),
			setup: func() error {
				if err := os.MkdirAll(filepath.Join(goPath, "src", "guthib.com", "foo", "bar"), 0700); err != nil {
					return err
				}
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bir"), 0700)
			},
			expected: expected{
				dir:   filepath.Join(goPath, "src", "guthib.com", "foo", "bar"),
				rpath: "",
			},
		},
		{
			desc: "vendor recursive",
			path: "guthib.com/foo/bar",
			root: filepath.Join("guthib.com", "foo", "root", "vendor", "guthib.com", "foo", "bir", "vendor", "guthib.com", "foo", "bur"),
			setup: func() error {
				if err := os.MkdirAll(
					filepath.Join(goPath, "src", "guthib.com", "foo", "root", "vendor", "guthib.com", "foo", "bir", "vendor", "guthib.com", "foo", "bur"),
					0700); err != nil {
					return err
				}
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bar"), 0700)
			},
			expected: expected{
				dir:   filepath.Join(project, "vendor", "guthib.com", "foo", "bar"),
				rpath: filepath.Join("guthib.com", "foo", "root", "vendor"),
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			if err := os.RemoveAll(goPath); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(goPath, 0700); err != nil {
				t.Fatal(err)
			}

			if test.setup != nil {
				err := test.setup()
				if err != nil {
					t.Fatal(err)
				}
			}

			dir, rPath, err := pkgDir(goPath, test.root, test.path)
			if err != nil {
				t.Fatal(err)
			}

			if dir != test.expected.dir {
				t.Errorf("[dir] got: %s, want: %s", dir, test.expected.dir)
			}

			if rPath != test.expected.rpath {
				t.Errorf(" [rpath] got: %s, want: %s", rPath, test.expected.rpath)
			}
		})
	}
}

func Test_previousRoot(t *testing.T) {
	testCases := []struct {
		desc     string
		root     string
		expected string
	}{
		{
			desc:     "GOPATH",
			root:     "github.com/foo/pkg/",
			expected: "",
		},
		{
			desc:     "vendor level 1",
			root:     "github.com/foo/pkg/vendor/guthib.com/containous/fromage",
			expected: "github.com/foo/pkg",
		},
		{
			desc:     "vendor level 2",
			root:     "github.com/foo/pkg/vendor/guthib.com/containous/fromage/vendor/guthib.com/containous/fuu",
			expected: "github.com/foo/pkg/vendor/guthib.com/containous/fromage",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			p := previousRoot(test.root)

			if p != test.expected {
				t.Errorf("got: %s, want: %s", p, test.expected)
			}
		})
	}
}
