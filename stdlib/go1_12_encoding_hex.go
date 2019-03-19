// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports encoding/hex'. DO NOT EDIT.

import (
	"encoding/hex"
	"reflect"
)

func init() {
	Value["encoding/hex"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Decode":         reflect.ValueOf(hex.Decode),
		"DecodeString":   reflect.ValueOf(hex.DecodeString),
		"DecodedLen":     reflect.ValueOf(hex.DecodedLen),
		"Dump":           reflect.ValueOf(hex.Dump),
		"Dumper":         reflect.ValueOf(hex.Dumper),
		"Encode":         reflect.ValueOf(hex.Encode),
		"EncodeToString": reflect.ValueOf(hex.EncodeToString),
		"EncodedLen":     reflect.ValueOf(hex.EncodedLen),
		"ErrLength":      reflect.ValueOf(&hex.ErrLength).Elem(),
		"NewDecoder":     reflect.ValueOf(hex.NewDecoder),
		"NewEncoder":     reflect.ValueOf(hex.NewEncoder),

		// type definitions
		"InvalidByteError": reflect.ValueOf((*hex.InvalidByteError)(nil)),
	}
}
