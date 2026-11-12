//go:build race

package vm

// raceStackFactor scales the Go stack limits of the tests: the race
// detector makes the frames about twice as large.
const raceStackFactor = 2

// raceEnabled is true in a -race build: the race detector allocates on its
// own, so the tests that count allocations do not run.
const raceEnabled = true
