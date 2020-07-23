// Code generated by 'github.com/containous/yaegi/extract encoding/base64'. DO NOT EDIT.

// +build go1.13,!go1.14

package stdlib

import (
	"encoding/base64"
	"reflect"
)

func init() {
	Symbols["encoding/base64"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"NewDecoder":     reflect.ValueOf(base64.NewDecoder),
		"NewEncoder":     reflect.ValueOf(base64.NewEncoder),
		"NewEncoding":    reflect.ValueOf(base64.NewEncoding),
		"NoPadding":      reflect.ValueOf(base64.NoPadding),
		"RawStdEncoding": reflect.ValueOf(&base64.RawStdEncoding).Elem(),
		"RawURLEncoding": reflect.ValueOf(&base64.RawURLEncoding).Elem(),
		"StdEncoding":    reflect.ValueOf(&base64.StdEncoding).Elem(),
		"StdPadding":     reflect.ValueOf(base64.StdPadding),
		"URLEncoding":    reflect.ValueOf(&base64.URLEncoding).Elem(),

		// type definitions
		"CorruptInputError": reflect.ValueOf((*base64.CorruptInputError)(nil)),
		"Encoding":          reflect.ValueOf((*base64.Encoding)(nil)),
	}
}
