package main

import (
	"go/build"
	"go/importer"
	"go/token"
	"slices"
	"testing"
)

// TestSetPlatform checks the build context used to extract syscall for
// another platform. With only GOOS and GOARCH changed, the tool tags stayed
// those of the machine: on an amd64 or arm64 machine, internal/abi did not
// type check for 386 or arm (goexperiment.regabiargs hides the file that
// defines IntArgRegs there), so syscall could not be extracted for them.
func TestSetPlatform(t *testing.T) {
	saved := build.Default
	defer func() { build.Default = saved }()
	for _, pl := range []string{"linux/386", "linux/arm", "linux/s390x", "windows/386", "openbsd/riscv64"} {
		if err := setPlatform(&build.Default, saved, pl); err != nil {
			t.Fatal(err)
		}
		if build.Default.GOOS+"/"+build.Default.GOARCH != pl || build.Default.CgoEnabled {
			t.Fatalf("%s: context %s/%s, cgo %v", pl, build.Default.GOOS, build.Default.GOARCH, build.Default.CgoEnabled)
		}
		regabi := slices.Contains(build.Default.ToolTags, "goexperiment.regabiargs")
		if want := pl == "linux/s390x" || pl == "openbsd/riscv64"; regabi != want {
			t.Errorf("%s: goexperiment.regabiargs in the tool tags: %v, want %v (%v)", pl, regabi, want, build.Default.ToolTags)
		}
		if _, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import("internal/abi"); err != nil {
			t.Errorf("%s: %v", pl, err)
		}
	}
}
