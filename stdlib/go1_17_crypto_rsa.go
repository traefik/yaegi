// Code generated by 'yaegi extract crypto/rsa'. DO NOT EDIT.

//go:build go1.17 && !go1.18
// +build go1.17,!go1.18

package stdlib

import (
	"crypto/rsa"
	"go/constant"
	"go/token"
	"reflect"
)

func init() {
	Symbols["crypto/rsa/rsa"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"DecryptOAEP":               reflect.ValueOf(rsa.DecryptOAEP),
		"DecryptPKCS1v15":           reflect.ValueOf(rsa.DecryptPKCS1v15),
		"DecryptPKCS1v15SessionKey": reflect.ValueOf(rsa.DecryptPKCS1v15SessionKey),
		"EncryptOAEP":               reflect.ValueOf(rsa.EncryptOAEP),
		"EncryptPKCS1v15":           reflect.ValueOf(rsa.EncryptPKCS1v15),
		"ErrDecryption":             reflect.ValueOf(&rsa.ErrDecryption).Elem(),
		"ErrMessageTooLong":         reflect.ValueOf(&rsa.ErrMessageTooLong).Elem(),
		"ErrVerification":           reflect.ValueOf(&rsa.ErrVerification).Elem(),
		"GenerateKey":               reflect.ValueOf(rsa.GenerateKey),
		"GenerateMultiPrimeKey":     reflect.ValueOf(rsa.GenerateMultiPrimeKey),
		"PSSSaltLengthAuto":         reflect.ValueOf(constant.MakeFromLiteral("0", token.INT, 0)),
		"PSSSaltLengthEqualsHash":   reflect.ValueOf(constant.MakeFromLiteral("-1", token.INT, 0)),
		"SignPKCS1v15":              reflect.ValueOf(rsa.SignPKCS1v15),
		"SignPSS":                   reflect.ValueOf(rsa.SignPSS),
		"VerifyPKCS1v15":            reflect.ValueOf(rsa.VerifyPKCS1v15),
		"VerifyPSS":                 reflect.ValueOf(rsa.VerifyPSS),

		// type definitions
		"CRTValue":               reflect.ValueOf((*rsa.CRTValue)(nil)),
		"OAEPOptions":            reflect.ValueOf((*rsa.OAEPOptions)(nil)),
		"PKCS1v15DecryptOptions": reflect.ValueOf((*rsa.PKCS1v15DecryptOptions)(nil)),
		"PSSOptions":             reflect.ValueOf((*rsa.PSSOptions)(nil)),
		"PrecomputedValues":      reflect.ValueOf((*rsa.PrecomputedValues)(nil)),
		"PrivateKey":             reflect.ValueOf((*rsa.PrivateKey)(nil)),
		"PublicKey":              reflect.ValueOf((*rsa.PublicKey)(nil)),
	}
}
