// Package b has types that embed the types of package a and declare a
// method of the same unexported name: it is another method.
package b

import "github.com/traefik/yaegi/v2/testdata/methodids/a"

// U embeds a.T and has its own m.
type U struct{ a.T }

func (U) m() string { return "b.U.m" }

// I is satisfied by the types that have the method m of package b.
type I interface{ m() string }

// Call calls the method m of package b.
func Call(i I) string { return i.m() }

// Value returns the method value i.m.
func Value(i I) func() string { return i.m }
