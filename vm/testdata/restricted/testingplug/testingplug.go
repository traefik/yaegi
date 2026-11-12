// Package testingplug runs the test driver of package testing, which parses
// the host's flags and ends the process with os.Exit.
package testingplug

import "testing"

func Run() {
	testing.Init()
	testing.Main(func(pat, str string) (bool, error) { return true, nil }, nil, nil, nil)
}
