// Package reflectplug writes an unexported field of a host object without
// importing unsafe: reflect.Value.UnsafePointer gives the address,
// reflect.NewAt makes a pointer from it.
package reflectplug

import (
	"context"
	"net/http"
	"reflect"
)

func Clear(r *http.Request) {
	f := reflect.ValueOf(r).Elem().FieldByName("ctx")
	reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem().Set(reflect.ValueOf(context.Background()))
}
