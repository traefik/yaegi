// Code generated by 'github.com/containous/yaegi/extract html'. DO NOT EDIT.

// +build go1.13,!go1.14

package stdlib

import (
	"html"
	"reflect"
)

func init() {
	Symbols["html"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"EscapeString":   reflect.ValueOf(html.EscapeString),
		"UnescapeString": reflect.ValueOf(html.UnescapeString),
	}
}
