// +build go1.11,!go1.13

package stdlib

import "reflect"

// Symbols variable stores the map of stdlib symbols per package
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/containous/yaegi/stdlib"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
}

// Provide access to go standard library (http://golang.org/pkg/)

//go:generate go run ./generate.go
