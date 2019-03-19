// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports crypto/ecdsa'. DO NOT EDIT.

import (
	"crypto/ecdsa"
	"reflect"
)

func init() {
	Value["crypto/ecdsa"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"GenerateKey": reflect.ValueOf(ecdsa.GenerateKey),
		"Sign":        reflect.ValueOf(ecdsa.Sign),
		"Verify":      reflect.ValueOf(ecdsa.Verify),

		// type definitions
		"PrivateKey": reflect.ValueOf((*ecdsa.PrivateKey)(nil)),
		"PublicKey":  reflect.ValueOf((*ecdsa.PublicKey)(nil)),
	}
}
