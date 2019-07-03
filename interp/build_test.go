package interp

import (
	"testing"
)

type testBuild struct {
	src string
	res bool
}

func TestBuildTag(t *testing.T) {
	// Assume a specific OS, arch and go version no matter the real underlying system
	oo, oa, ov := goos, goarch, goversion
	goos, goarch, goversion = "linux", "amd64", 11
	defer func() { goos, goarch, goversion = oo, oa, ov }()

	tests := []testBuild{
		{"// +build linux", true},
		{"// +build windows", false},
		{"// +build go1.11", true},
		{"// +build !go1.12", true},
		{"// +build go1.12", false},
		{"// +build !go1.10", false},
		{"// +build go1.9", true},
		{"// +build ignore", false},
		{"// +build linux,amd64", true},
		{"// +build linux,i386", false},
		{"// +build linux,i386 go1.11", true},
		{"// +build linux\n// +build amd64", true},
		{"// +build linux\n\n\n// +build amd64", true},
		{"// +build linux\n// +build i386", false},
	}

	i := New(Options{})
	for _, test := range tests {
		test := test
		src := test.src + "\npackage x"
		t.Run("", func(t *testing.T) {
			if r := i.buildOk("", src); r != test.res {
				t.Errorf("got %v, want %v", r, test.res)
			}
		})
	}
}

func TestBuildFile(t *testing.T) {
	// Assume a specific OS, arch and go pattern no matter the real underlying system
	oo, oa := goos, goarch
	goos, goarch = "linux", "amd64"
	defer func() { goos, goarch = oo, oa }()

	tests := []testBuild{
		{"foo/bar_linux_amd64.go", false},
		{"foo/bar.go", false},
		{"bar.go", false},
		{"bar_linux.go", false},
		{"bar_maix.go", false},
		{"bar_mlinux.go", false},
		{"bar_aix_foo.go", false},
		{"bar_aix_s390x.go", true},
		{"bar_aix_amd64.go", true},
		{"bar_linux_arm.go", true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.src, func(t *testing.T) {
			if r := skipFile(test.src); r != test.res {
				t.Errorf("got %v, want %v", r, test.res)
			}
		})
	}
}
