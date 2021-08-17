// Code generated by 'yaegi extract crypto/ed25519'. DO NOT EDIT.

// +build go1.16,!go1.17

package stdlib

import (
	"crypto/ed25519"
	"go/constant"
	"go/token"
	"reflect"
)

func init() {
	Symbols["crypto/ed25519/ed25519"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"GenerateKey":    reflect.ValueOf(ed25519.GenerateKey),
		"NewKeyFromSeed": reflect.ValueOf(ed25519.NewKeyFromSeed),
		"PrivateKeySize": reflect.ValueOf(constant.MakeFromLiteral("64", token.INT, 0)),
		"PublicKeySize":  reflect.ValueOf(constant.MakeFromLiteral("32", token.INT, 0)),
		"SeedSize":       reflect.ValueOf(constant.MakeFromLiteral("32", token.INT, 0)),
		"Sign":           reflect.ValueOf(ed25519.Sign),
		"SignatureSize":  reflect.ValueOf(constant.MakeFromLiteral("64", token.INT, 0)),
		"Verify":         reflect.ValueOf(ed25519.Verify),

		// type definitions
		"PrivateKey": reflect.ValueOf((*ed25519.PrivateKey)(nil)),
		"PublicKey":  reflect.ValueOf((*ed25519.PublicKey)(nil)),
	}
}
