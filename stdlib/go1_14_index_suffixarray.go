// Code generated by 'github.com/containous/yaegi/extract index/suffixarray'. DO NOT EDIT.

// +build go1.14,!go1.15

package stdlib

import (
	"index/suffixarray"
	"reflect"
)

func init() {
	Symbols["index/suffixarray"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"New": reflect.ValueOf(suffixarray.New),

		// type definitions
		"Index": reflect.ValueOf((*suffixarray.Index)(nil)),
	}
}
