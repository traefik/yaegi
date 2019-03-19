// +build go1.11, !go1.12

package stdlib

// Code generated by 'goexports crypto/rc4'. DO NOT EDIT.

import (
	"crypto/rc4"
	"reflect"
)

func init() {
	Value["crypto/rc4"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"NewCipher": reflect.ValueOf(rc4.NewCipher),

		// type definitions
		"Cipher":       reflect.ValueOf((*rc4.Cipher)(nil)),
		"KeySizeError": reflect.ValueOf((*rc4.KeySizeError)(nil)),
	}
}
