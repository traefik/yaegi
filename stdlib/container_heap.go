package stdlib

// Code generated by 'goexports container/heap'. DO NOT EDIT.

import (
	"container/heap"
	"reflect"
)

func init() {
	Value["container/heap"] = map[string]reflect.Value{
		"Fix":    reflect.ValueOf(heap.Fix),
		"Init":   reflect.ValueOf(heap.Init),
		"Pop":    reflect.ValueOf(heap.Pop),
		"Push":   reflect.ValueOf(heap.Push),
		"Remove": reflect.ValueOf(heap.Remove),
	}

	Type["container/heap"] = map[string]reflect.Type{
		"Interface": reflect.TypeOf((*heap.Interface)(nil)).Elem(),
	}
}
