// Code generated by 'github.com/containous/yaegi/extract runtime/trace'. DO NOT EDIT.

// +build go1.15,!go1.16

package stdlib

import (
	"reflect"
	"runtime/trace"
)

func init() {
	Symbols["runtime/trace"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"IsEnabled":   reflect.ValueOf(trace.IsEnabled),
		"Log":         reflect.ValueOf(trace.Log),
		"Logf":        reflect.ValueOf(trace.Logf),
		"NewTask":     reflect.ValueOf(trace.NewTask),
		"Start":       reflect.ValueOf(trace.Start),
		"StartRegion": reflect.ValueOf(trace.StartRegion),
		"Stop":        reflect.ValueOf(trace.Stop),
		"WithRegion":  reflect.ValueOf(trace.WithRegion),

		// type definitions
		"Region": reflect.ValueOf((*trace.Region)(nil)),
		"Task":   reflect.ValueOf((*trace.Task)(nil)),
	}
}
