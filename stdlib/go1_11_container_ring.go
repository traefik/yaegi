// +build go1.11, !go1.12

package stdlib

// Code generated by 'goexports container/ring'. DO NOT EDIT.

import (
	"container/ring"
	"reflect"
)

func init() {
	Value["container/ring"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"New": reflect.ValueOf(ring.New),

		// type definitions
		"Ring": reflect.ValueOf((*ring.Ring)(nil)),
	}
}
