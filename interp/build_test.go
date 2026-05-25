package interp

import (
	"go/build"
	"os"
	"path/filepath"
	"testing"
)

type testBuild struct {
	src string
	res bool
}

func TestBuildTag(t *testing.T) {
	// Assume a specific OS, arch and go version no matter the real underlying system
	ctx := build.Context{
		GOARCH:      "amd64",
		GOOS:        "linux",
		BuildTags:   []string{"foo"},
		ReleaseTags: []string{"go1.11"},
	}

	tests := []testBuild{
		{"// +build linux", true},
		{"// +build windows", false},
		{"// +build go1.9", true},
		{"// +build go1.11", true},
		{"// +build go1.12", false},
		{"// +build !go1.10", false},
		{"// +build !go1.12", true},
		{"// +build ignore", false},
		{"// +build linux,amd64", true},
		{"// +build linux,i386", false},
		{"// +build linux,i386 go1.11", true},
		{"// +build linux\n// +build amd64", true},
		{"// +build linux\n\n\n// +build amd64", true},
		{"// +build linux\n// +build i386", false},
		{"// +build foo", true},
		{"// +build !foo", false},
		{"// +build bar", false},
		{"//go:build linux", true},
		{"//go:build windows", false},
		{"//go:build linux && amd64", true},
		{"//go:build linux && arm64", false},
		{"//go:build foo || bar", true},
		{"//go:build !foo", false},
	}

	i := New(Options{})
	for _, test := range tests {
		test := test
		src := test.src + "\npackage x"
		t.Run(test.src, func(t *testing.T) {
			if r, _ := i.buildOk(&ctx, "", src); r != test.res {
				t.Errorf("got %v, want %v", r, test.res)
			}
		})
	}
}

func TestEvalPathBuildConstraints(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{name: "legacy", tag: "// +build windows"},
		{name: "gobuild", tag: "//go:build windows"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "main.go")
			src := test.tag + "\n\npackage main\n\nfunc main() { panic(\"should not run\") }\n"
			if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}

			i := New(Options{})
			v, err := i.EvalPath(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.IsValid() {
				t.Fatalf("got %v, want invalid zero result", v)
			}
		})
	}
}

func TestSkipFile(t *testing.T) {
	// Assume a specific OS, arch and go pattern no matter the real underlying system
	ctx := build.Context{
		GOARCH: "amd64",
		GOOS:   "linux",
	}

	tests := []testBuild{
		{"foo/bar_linux_amd64.go", false},
		{"foo/bar.go", false},
		{"bar.go", false},
		{"bar_linux.go", false},
		{"bar_maix.go", false},
		{"bar_mlinux.go", false},

		{"bar_aix_foo.go", false},
		{"bar_linux_foo.go", false},
		{"bar_foo_amd64.go", false},
		{"bar_foo_arm.go", true},

		{"bar_aix_s390x.go", true},
		{"bar_aix_amd64.go", true},
		{"bar_linux_arm.go", true},

		{"bar_amd64.go", false},
		{"bar_arm.go", true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.src, func(t *testing.T) {
			if r := skipFile(&ctx, test.src, NoTest); r != test.res {
				t.Errorf("got %v, want %v", r, test.res)
			}
		})
	}
}

func Test_goMinorVersion(t *testing.T) {
	tests := []struct {
		desc     string
		context  build.Context
		expected int
	}{
		{
			desc: "stable",
			context: build.Context{ReleaseTags: []string{
				"go1.1", "go1.2", "go1.3", "go1.4", "go1.5", "go1.6", "go1.7", "go1.8", "go1.9", "go1.10", "go1.11", "go1.12",
			}},
			expected: 12,
		},
		{
			desc: "devel/beta/rc",
			context: build.Context{ReleaseTags: []string{
				"go1.1", "go1.2", "go1.3", "go1.4", "go1.5", "go1.6", "go1.7", "go1.8", "go1.9", "go1.10", "go1.11", "go1.12", "go1.13",
			}},
			expected: 13,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			minor := goMinorVersion(&test.context)

			if minor != test.expected {
				t.Errorf("got %v, want %v", minor, test.expected)
			}
		})
	}
}
