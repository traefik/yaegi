// Package stdsyms is the native symbol table of the standard library used
// by the VM: for every package a plugin may import, its exported functions,
// variables, constants and types as reflect values, and wrapper types for
// its interfaces. It is generated for the Go release yaegi v2 is built with
// (go generate ./stdsyms, cmd/stdsyms), with yaegi v2's extractor
// (internal/extract, derived from yaegi's), so that plugins see the API of
// that release and not only the one of a given yaegi version.
//
// The table is restricted as yaegi's restricted stdlib is: os/exec, plugin,
// unsafe and syscall have no entries (see vm/restrict.go for the import
// checks), and the functions that would end or inspect the host process are
// replaced (restricted.go). Programs run without restrictions also get
// Unrestricted (unrestricted.go): the real functions, and os/exec and
// syscall. The packages whose init changes the global state of the
// process (expvar, net/http/pprof, testing/quick) are in package
// stdsyms/sideeffect, which a host imports only if it wants them.
package stdsyms

//go:generate go run ../cmd/stdsyms

import "reflect"

// Symbols maps "importpath/name" (e.g. "net/http/http") to the symbols of
// the package.
var Symbols = map[string]map[string]reflect.Value{}
