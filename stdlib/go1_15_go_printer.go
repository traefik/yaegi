// Code generated by 'yaegi extract go/printer'. DO NOT EDIT.

// +build go1.15,!go1.16

package stdlib

import (
	"go/printer"
	"reflect"
)

func init() {
	Symbols["go/printer"] = map[string]reflect.Value{
		// default package name identifier
		".name": reflect.ValueOf("printer"),

		// function, constant and variable definitions
		"Fprint":    reflect.ValueOf(printer.Fprint),
		"RawFormat": reflect.ValueOf(printer.RawFormat),
		"SourcePos": reflect.ValueOf(printer.SourcePos),
		"TabIndent": reflect.ValueOf(printer.TabIndent),
		"UseSpaces": reflect.ValueOf(printer.UseSpaces),

		// type definitions
		"CommentedNode": reflect.ValueOf((*printer.CommentedNode)(nil)),
		"Config":        reflect.ValueOf((*printer.Config)(nil)),
		"Mode":          reflect.ValueOf((*printer.Mode)(nil)),
	}
}
