// Package msan stands for internal/msan in the test files run by
// cmd/stdtests: a program outside the standard library cannot import an
// internal package.
package msan

// Enabled is false: the programs are never built with -msan.
const Enabled = false
