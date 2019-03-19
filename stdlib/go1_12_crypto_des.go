// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports crypto/des'. DO NOT EDIT.

import (
	"crypto/des"
	"reflect"
)

func init() {
	Value["crypto/des"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"BlockSize":          reflect.ValueOf(des.BlockSize),
		"NewCipher":          reflect.ValueOf(des.NewCipher),
		"NewTripleDESCipher": reflect.ValueOf(des.NewTripleDESCipher),

		// type definitions
		"KeySizeError": reflect.ValueOf((*des.KeySizeError)(nil)),
	}
}
