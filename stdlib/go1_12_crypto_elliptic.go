// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports crypto/elliptic'. DO NOT EDIT.

import (
	"crypto/elliptic"
	"reflect"
)

func init() {
	Value["crypto/elliptic"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"GenerateKey": reflect.ValueOf(elliptic.GenerateKey),
		"Marshal":     reflect.ValueOf(elliptic.Marshal),
		"P224":        reflect.ValueOf(elliptic.P224),
		"P256":        reflect.ValueOf(elliptic.P256),
		"P384":        reflect.ValueOf(elliptic.P384),
		"P521":        reflect.ValueOf(elliptic.P521),
		"Unmarshal":   reflect.ValueOf(elliptic.Unmarshal),

		// type definitions
		"Curve":       reflect.ValueOf((*elliptic.Curve)(nil)),
		"CurveParams": reflect.ValueOf((*elliptic.CurveParams)(nil)),
	}
}
