// Code generated by 'yaegi extract go/importer'. DO NOT EDIT.

//go:build go1.17 && !go1.18
// +build go1.17,!go1.18

package stdlib

import (
	"go/importer"
	"reflect"
)

func init() {
	Symbols["go/importer/importer"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Default":     reflect.ValueOf(importer.Default),
		"For":         reflect.ValueOf(importer.For),
		"ForCompiler": reflect.ValueOf(importer.ForCompiler),

		// type definitions
		"Lookup": reflect.ValueOf((*importer.Lookup)(nil)),
	}
}
