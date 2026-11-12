// Package asan stands for internal/asan in the test files run by
// cmd/stdtests: a program outside the standard library cannot import an
// internal package.
package asan

// Enabled is false: the programs are never built with -asan.
const Enabled = false
