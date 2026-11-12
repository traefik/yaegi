//go:build !race

package vm

// raceStackFactor scales the Go stack limits of the tests (see
// race_test.go).
const raceStackFactor = 1

// raceEnabled is true in a -race build (see race_test.go).
const raceEnabled = false
