// Package testingplug runs the test driver of package testing, which parses
// the host's flags and ends the process with os.Exit, also through
// (*testing.M).Run on a zero M, and the methods of testing that change the
// working directory and the environment of the host.
package testingplug

import "testing"

func Run() {
	testing.Init()
	testing.Main(func(pat, str string) (bool, error) { return true, nil }, nil, nil, nil)
}

func Allocs() float64 { return testing.AllocsPerRun(1, func() {}) }

func RunM() int { return new(testing.M).Run() }

func Process() {
	t := new(testing.T)
	t.Chdir("/")
	t.Setenv("YAEGI_TESTINGPLUG", "1")
	b := new(testing.B)
	b.Chdir("/")
	b.Setenv("YAEGI_TESTINGPLUG", "1")
}
