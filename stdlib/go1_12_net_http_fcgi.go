// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports net/http/fcgi'. DO NOT EDIT.

import (
	"net/http/fcgi"
	"reflect"
)

func init() {
	Value["net/http/fcgi"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"ErrConnClosed":     reflect.ValueOf(&fcgi.ErrConnClosed).Elem(),
		"ErrRequestAborted": reflect.ValueOf(&fcgi.ErrRequestAborted).Elem(),
		"ProcessEnv":        reflect.ValueOf(fcgi.ProcessEnv),
		"Serve":             reflect.ValueOf(fcgi.Serve),

		// type definitions

	}
}
