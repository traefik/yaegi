//go:build go1.19
// +build go1.19

// Package syscall provide wrapper of standard library syscall package for native import in Yaegi.
package syscall

import "reflect"

// Symbols stores the map of syscall package symbols.
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/traefik/yaegi/stdlib/syscall/syscall"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

//go:generate ../../internal/cmd/extract/extract -exclude=^Exec,Exit,ForkExec,Kill,Ptrace,Reboot,Shutdown,StartProcess,Syscall syscall
