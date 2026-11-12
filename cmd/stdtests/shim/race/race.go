// Package race stands for internal/race in the test files run by
// cmd/stdtests: a program outside the standard library cannot import an
// internal package.
package race

// Enabled is false: the programs are never built with -race.
const Enabled = false
