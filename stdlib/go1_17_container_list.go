// Code generated by 'yaegi extract container/list'. DO NOT EDIT.

//go:build go1.17 && !go1.18
// +build go1.17,!go1.18

package stdlib

import (
	"container/list"
	"reflect"
)

func init() {
	Symbols["container/list/list"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"New": reflect.ValueOf(list.New),

		// type definitions
		"Element": reflect.ValueOf((*list.Element)(nil)),
		"List":    reflect.ValueOf((*list.List)(nil)),
	}
}
