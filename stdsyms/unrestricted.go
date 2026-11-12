package stdsyms

// The real versions of the functions restricted.go replaces, for programs
// run with the standard library as in Go (vm.RunMain without
// RunOptions.Restricted and RestrictedStd, plugins loaded with
// Options.Unrestricted and Options.RealProcess): os.Exit and log.Fatal end
// the process, as in Go.

import (
	"log"
	"os"
	"reflect"
)

// Unrestricted maps "importpath/name" to the symbols that programs run
// without restrictions get in addition to Symbols, or instead of the
// entries of Symbols with the same name. It also holds the tables of
// os/exec and syscall, which restricted programs never get (the generated
// files whose table is Unrestricted).
var Unrestricted = map[string]map[string]reflect.Value{}

func init() {
	Unrestricted["os/os"] = map[string]reflect.Value{
		"Exit":        reflect.ValueOf(os.Exit),
		"FindProcess": reflect.ValueOf(os.FindProcess),
	}
	Unrestricted["log/log"] = map[string]reflect.Value{
		"Default": reflect.ValueOf(log.Default),
		"Fatal":   reflect.ValueOf(log.Fatal),
		"Fatalf":  reflect.ValueOf(log.Fatalf),
		"Fatalln": reflect.ValueOf(log.Fatalln),
		"New":     reflect.ValueOf(log.New),
		"Logger":  reflect.ValueOf((*log.Logger)(nil)),
	}
}
