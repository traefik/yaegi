// Package sideeffect adds to the native symbol table of the standard
// library (stdsyms.Symbols) the packages whose init changes the global
// state of the process: expvar and net/http/pprof register handlers on
// http.DefaultServeMux (/debug/vars, /debug/pprof/), and testing/quick
// defines the flag -quickchecks on flag.CommandLine. Package stdsyms leaves
// them out, so that a host that imports the VM keeps its DefaultServeMux
// and its flags as they are.
//
// A host imports this package for its side effects when its programs may
// use these packages (cmd/yaegi does, to run programs like "go run").
// Without it, a program that imports one of them is refused at load time.
// Restricted plugins may never import expvar and net/http/pprof
// (vm/restrict.go). The files are generated with the others
// (go generate ./stdsyms, cmd/stdsyms).
package sideeffect

import "github.com/traefik/yaegi/v2/stdsyms"

// Symbols is stdsyms.Symbols: the generated files add their tables to it.
var Symbols = stdsyms.Symbols
