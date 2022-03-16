// Code generated by 'yaegi extract crypto/md5'. DO NOT EDIT.

//go:build go1.17 && !go1.18
// +build go1.17,!go1.18

package stdlib

import (
	"crypto/md5"
	"go/constant"
	"go/token"
	"reflect"
)

func init() {
	Symbols["crypto/md5/md5"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"BlockSize": reflect.ValueOf(constant.MakeFromLiteral("64", token.INT, 0)),
		"New":       reflect.ValueOf(md5.New),
		"Size":      reflect.ValueOf(constant.MakeFromLiteral("16", token.INT, 0)),
		"Sum":       reflect.ValueOf(md5.Sum),
	}
}
