// +build go1.13,!go1.15

// Package unsafe provides wrapper of standard library unsafe package to be imported natively in Yaegi.
package unsafe

import (
	"reflect"
	"unsafe"
)

// Symbols stores the map of unsafe package symbols.
var Symbols = map[string]map[string]reflect.Value{}

func init() {
	Symbols["github.com/containous/yaegi/stdlib/unsafe"] = map[string]reflect.Value{
		"Symbols": reflect.ValueOf(Symbols),
	}
	Symbols["github.com/containous/yaegi"] = map[string]reflect.Value{
		"convert": reflect.ValueOf(convert),
	}
}

func convert(from, to reflect.Type) func(src, dest reflect.Value) {
	switch {
	case to.Kind() == reflect.UnsafePointer && from.Kind() == reflect.Uintptr:
		return func(src, dest reflect.Value) {
			dest.SetPointer(unsafe.Pointer(src.Interface().(uintptr))) //nolint:govet
		}
	case to.Kind() == reflect.UnsafePointer:
		return func(src, dest reflect.Value) {
			dest.SetPointer(unsafe.Pointer(src.Pointer()))
		}
	case to.Kind() == reflect.Uintptr && from.Kind() == reflect.UnsafePointer:
		return func(src, dest reflect.Value) {
			ptr := src.Interface().(unsafe.Pointer)
			dest.Set(reflect.ValueOf(uintptr(ptr)))
		}
	case from.Kind() == reflect.UnsafePointer:
		return func(src, dest reflect.Value) {
			ptr := src.Interface().(unsafe.Pointer)
			v := reflect.NewAt(dest.Type().Elem(), ptr)
			dest.Set(v)
		}
	default:
		return nil
	}
}

//go:generate ../../cmd/goexports/goexports unsafe
