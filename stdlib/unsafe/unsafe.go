// +build go1.12,!go1.14

// Unsafe provides wrapper of standard library unsafe package to be imported natively in Yaegi.
package unsafe

import "reflect"

// Symbols stores the map of unsafe package symbols
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/containous/yaegi/stdlib/unsafe"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

//go:generate ../../cmd/goexports/goexports unsafe
