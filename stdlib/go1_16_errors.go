// Code generated by 'yaegi extract errors'. DO NOT EDIT.

// +build go1.16

package stdlib

import (
	"errors"
	"reflect"
)

func init() {
	Symbols["errors"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"As":     reflect.ValueOf(errors.As),
		"Is":     reflect.ValueOf(errors.Is),
		"New":    reflect.ValueOf(errors.New),
		"Unwrap": reflect.ValueOf(errors.Unwrap),
	}
}
