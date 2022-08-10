package interp

import (
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
			root:     "github.com/foo/plugin/vendor/guthib.com/traefik/fromage",
			path:     "guthib.com/traefik/fromage/couteau/lol",
			expected: "github.com/foo/plugin/vendor/guthib.com/traefik/fromage/couteau/lol",
		},
		{
			desc:     "path is a vendored package",
			root:     "github.com/foo/plugin/vendor/guthib.com/traefik/fromage",
			path:     "vendor/guthib.com/traefik/vin",
			expected: "github.com/foo/plugin/vendor/guthib.com/traefik/fromage/vendor/guthib.com/traefik/vin",
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
	goPath, err := os.MkdirTemp("", "pkdir")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(goPath)
	}()

	// Create project
	project := filepath.Join(goPath, "src", "guthib.com", "foo", "root")
	if err := os.MkdirAll(project, 0o700); err != nil {
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
				return os.MkdirAll(filepath.Join(goPath, "src", "guthib.com", "foo", "bar"), 0o700)
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
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bar"), 0o700)
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
				return os.MkdirAll(filepath.Join(goPath, "src", "guthib.com", "foo", "bar"), 0o700)
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
				if err := os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bar"), 0o700); err != nil {
					return err
				}
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bir"), 0o700)
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
				if err := os.MkdirAll(filepath.Join(goPath, "src", "guthib.com", "foo", "bar"), 0o700); err != nil {
					return err
				}
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bir"), 0o700)
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
					0o700); err != nil {
					return err
				}
				return os.MkdirAll(filepath.Join(project, "vendor", "guthib.com", "foo", "bar"), 0o700)
			},
			expected: expected{
				dir:   filepath.Join(project, "vendor", "guthib.com", "foo", "bar"),
				rpath: filepath.Join("guthib.com", "foo", "root", "vendor"),
			},
		},
	}

	interp := New(Options{
		Env: []string{"GOCACHE=" + os.Getenv("GOCACHE")},
	})

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			if err := os.RemoveAll(goPath); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(goPath, 0o700); err != nil {
				t.Fatal(err)
			}

			if test.setup != nil {
				err := test.setup()
				if err != nil {
					t.Fatal(err)
				}
			}

			dir, err := interp.getPackageDir(test.path)
			if err != nil {
				t.Fatal(err)
			}

			if dir != test.expected.dir {
				t.Errorf("[dir] got: %s, want: %s", dir, test.expected.dir)
			}

			if test.root != test.expected.rpath {
				t.Errorf(" [rpath] got: %s, want: %s", test.root, test.expected.rpath)
			}
		})
	}
}
