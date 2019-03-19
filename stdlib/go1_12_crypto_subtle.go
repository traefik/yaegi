// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports crypto/subtle'. DO NOT EDIT.

import (
	"crypto/subtle"
	"reflect"
)

func init() {
	Value["crypto/subtle"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"ConstantTimeByteEq":   reflect.ValueOf(subtle.ConstantTimeByteEq),
		"ConstantTimeCompare":  reflect.ValueOf(subtle.ConstantTimeCompare),
		"ConstantTimeCopy":     reflect.ValueOf(subtle.ConstantTimeCopy),
		"ConstantTimeEq":       reflect.ValueOf(subtle.ConstantTimeEq),
		"ConstantTimeLessOrEq": reflect.ValueOf(subtle.ConstantTimeLessOrEq),
		"ConstantTimeSelect":   reflect.ValueOf(subtle.ConstantTimeSelect),

		// type definitions

	}
}
