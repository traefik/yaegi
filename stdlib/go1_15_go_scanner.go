// Code generated by 'yaegi extract go/scanner'. DO NOT EDIT.

// +build go1.15,!go1.16

package stdlib

import (
	"go/scanner"
	"reflect"
)

func init() {
	Symbols["go/scanner"] = map[string]reflect.Value{
		// default package name identifier
		".name": reflect.ValueOf("scanner"),

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
