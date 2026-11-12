// Package testenv stands for internal/testenv in the test files run by
// cmd/stdtests: a program outside the standard library cannot import an
// internal package. It has the functions that the test files of the
// packages of cmd/stdtests use; a test file that uses another one loses the
// declarations that use it (see cmd/stdtests).
//
// The tests that need to run a program (the go command, the test binary)
// are skipped: a plugin cannot use os/exec. The skip happens in the native
// run too, so such a test is "skip" in every column.
package testenv

import (
	"os"
	"testing"
)

// Builder is the name of the Go builder, as in internal/testenv.
func Builder() string { return os.Getenv("GO_BUILDER_NAME") }

// OptimizationOff is false: the programs are built with the default flags.
func OptimizationOff() bool { return false }

// SkipIfOptimizationOff does nothing: see OptimizationOff.
func SkipIfOptimizationOff(t testing.TB) {}

// NeedsExec is the reason of the skips of this package.
const NeedsExec = "cmd/stdtests: needs os/exec (a plugin cannot run programs)"

// MustHaveExec skips the test.
func MustHaveExec(t testing.TB) {
	t.Helper()
	t.Skip(NeedsExec)
}

// MustHaveGoBuild skips the test.
func MustHaveGoBuild(t testing.TB) {
	t.Helper()
	t.Skip(NeedsExec)
}

// GoToolPath skips the test.
func GoToolPath(t testing.TB) string {
	t.Helper()
	t.Skip(NeedsExec)
	return ""
}

// Executable skips the test: the path of the test binary is only useful
// to run it again.
func Executable(t testing.TB) string {
	t.Helper()
	t.Skip(NeedsExec)
	return ""
}
