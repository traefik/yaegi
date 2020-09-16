// +build go1.14,!go1.16

// Package syscall provide wrapper of standard library syscall package for native import in Yaegi.
package syscall

import "reflect"

// Symbols stores the map of syscall package symbols.
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/traefik/yaegi/stdlib/syscall"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

//go:generate ../../cmd/goexports/goexports syscall
