package stdlib

// Code generated by 'goexports unicode/utf16'. DO NOT EDIT.

import (
	"reflect"
	"unicode/utf16"
)

func init() {
	Value["unicode/utf16"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Decode":      reflect.ValueOf(utf16.Decode),
		"DecodeRune":  reflect.ValueOf(utf16.DecodeRune),
		"Encode":      reflect.ValueOf(utf16.Encode),
		"EncodeRune":  reflect.ValueOf(utf16.EncodeRune),
		"IsSurrogate": reflect.ValueOf(utf16.IsSurrogate),

		// type definitions

	}
}
