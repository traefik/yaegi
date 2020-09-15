// Code generated by 'github.com/traefik/yaegi/extract go/scanner'. DO NOT EDIT.

// +build go1.14,!go1.15

package stdlib

import (
	"go/scanner"
	"reflect"
)

func init() {
	Symbols["go/scanner"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"PrintError":   reflect.ValueOf(scanner.PrintError),
		"ScanComments": reflect.ValueOf(scanner.ScanComments),

		// type definitions
		"Error":        reflect.ValueOf((*scanner.Error)(nil)),
		"ErrorHandler": reflect.ValueOf((*scanner.ErrorHandler)(nil)),
		"ErrorList":    reflect.ValueOf((*scanner.ErrorList)(nil)),
		"Mode":         reflect.ValueOf((*scanner.Mode)(nil)),
		"Scanner":      reflect.ValueOf((*scanner.Scanner)(nil)),
	}
}
