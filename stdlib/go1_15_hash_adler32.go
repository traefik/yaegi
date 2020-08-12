// Code generated by 'github.com/containous/yaegi/extract hash/adler32'. DO NOT EDIT.

// +build go1.15,!go1.16

package stdlib

import (
	"go/constant"
	"go/token"
	"hash/adler32"
	"reflect"
)

func init() {
	Symbols["hash/adler32"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Checksum": reflect.ValueOf(adler32.Checksum),
		"New":      reflect.ValueOf(adler32.New),
		"Size":     reflect.ValueOf(constant.MakeFromLiteral("4", token.INT, 0)),
	}
}
