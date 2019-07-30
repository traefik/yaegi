// +build go1.11,!go1.13

package syscall

import "reflect"

// Symbols stores the map of syscall package symbols
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/containous/yaegi/stdlib/syscall"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

//go:generate ../../cmd/goexports/goexports syscall
