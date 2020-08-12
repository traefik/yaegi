// Code generated by 'github.com/containous/yaegi/extract crypto/ecdsa'. DO NOT EDIT.

// +build go1.15,!go1.16

package stdlib

import (
	"crypto/ecdsa"
	"reflect"
)

func init() {
	Symbols["crypto/ecdsa"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"GenerateKey": reflect.ValueOf(ecdsa.GenerateKey),
		"Sign":        reflect.ValueOf(ecdsa.Sign),
		"SignASN1":    reflect.ValueOf(ecdsa.SignASN1),
		"Verify":      reflect.ValueOf(ecdsa.Verify),
		"VerifyASN1":  reflect.ValueOf(ecdsa.VerifyASN1),

		// type definitions
		"PrivateKey": reflect.ValueOf((*ecdsa.PrivateKey)(nil)),
		"PublicKey":  reflect.ValueOf((*ecdsa.PublicKey)(nil)),
	}
}
