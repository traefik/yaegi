// Package testmain starts the tests of a program made by cmd/stdtests. It
// does what the test main of "go test" does (testing.MainStart with the
// testDeps of testing/internal/testdeps), for a program run natively or by
// yaegi v2.
//
// testing.MainStart takes an unexported interface (testDeps): code run by
// yaegi v2 cannot implement it. So this package is always native: the vmhost
// of cmd/stdtests registers it and runs it in native mode, and the program
// only passes the lists of its tests to MainStart.
package testmain

import (
	"errors"
	"io"
	"os"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"
)

// Main runs the tests, fuzz targets (their seeds) and examples of a test
// program and ends the process, as the test main of "go test" does.
// importPath is the import path of the package under test. testMain is the
// TestMain function of the test files, or nil.
func Main(importPath string, tests []testing.InternalTest, fuzz []testing.InternalFuzzTarget, examples []testing.InternalExample, testMain func(*testing.M)) {
	m := testing.MainStart(deps{importPath: importPath}, tests, nil, fuzz, examples)
	if testMain == nil {
		os.Exit(m.Run())
	}
	testMain(m)
	// As go test: when TestMain returns, the exit code is the one of m.Run.
	os.Exit(int(reflect.ValueOf(m).Elem().FieldByName("exitCode").Int()))
}

// corpusEntry is the type of testing (an alias of an unnamed struct, as in
// testing/internal/testdeps).
type corpusEntry = struct {
	Parent     string
	Path       string
	Data       []byte
	Values     []any
	Generation int
	IsSeed     bool
}

var errNoFuzzing = errors.New("cmd/stdtests: fuzzing is not supported, only the seed corpus of f.Add runs")

// deps implements the testDeps interface of package testing. Profiles,
// coverage and fuzzing are not supported: the runner does not use them.
// The fuzz targets run their f.Add seeds, as "go test" without -fuzz;
// the corpus files of testdata/fuzz are not read.
type deps struct {
	importPath string
}

var (
	matchMu  sync.Mutex
	matchPat string
	matchRe  *regexp.Regexp
)

func (deps) MatchString(pat, str string) (bool, error) {
	matchMu.Lock()
	defer matchMu.Unlock()
	if matchRe == nil || matchPat != pat {
		re, err := regexp.Compile(pat)
		if err != nil {
			return false, err
		}
		matchPat, matchRe = pat, re
	}
	return matchRe.MatchString(str), nil
}

func (deps) StartCPUProfile(io.Writer) error { return errors.New("cmd/stdtests: no profiles") }
func (deps) StopCPUProfile()                 {}
func (deps) StartTestLog(io.Writer)          {}
func (deps) StopTestLog() error              { return nil }
func (deps) WriteProfileTo(string, io.Writer, int) error {
	return errors.New("cmd/stdtests: no profiles")
}
func (d deps) ImportPath() string                                     { return d.importPath }
func (deps) ModulePath() string                                       { return "std" }
func (deps) SetPanicOnExit0(bool)                                     {}
func (deps) ResetCoverage()                                           {}
func (deps) SnapshotCoverage()                                        {}
func (deps) CheckCorpus([]any, []reflect.Type) error                  { return nil }
func (deps) RunFuzzWorker(func(corpusEntry) error) error              { return errNoFuzzing }
func (deps) ReadCorpus(string, []reflect.Type) ([]corpusEntry, error) { return nil, nil }

func (deps) CoordinateFuzzing(time.Duration, int64, time.Duration, int64, int, []corpusEntry, []reflect.Type, string, string) error {
	return errNoFuzzing
}

func (deps) InitRuntimeCoverage() (string, func(string, string) (string, error), func() float64) {
	return "", nil, nil
}
