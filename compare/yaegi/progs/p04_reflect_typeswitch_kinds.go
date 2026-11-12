package main

import (
	"fmt"
	"reflect"
	"sort"
)

type ID int64

type Pair struct {
	K string
	V any
}

func walk(v reflect.Value, depth int) {
	pad := fmt.Sprintf("%*s", depth*2, "")
	switch v.Kind() {
	case reflect.Struct:
		fmt.Println(pad+"struct", v.Type())
		for i := 0; i < v.NumField(); i++ {
			walk(v.Field(i), depth+1)
		}
	case reflect.Slice:
		fmt.Println(pad+"slice", v.Len())
		for i := 0; i < v.Len(); i++ {
			walk(v.Index(i), depth+1)
		}
	case reflect.Map:
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		fmt.Println(pad+"map", len(keys))
		for _, k := range keys {
			walk(v.MapIndex(k), depth+1)
		}
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			fmt.Println(pad + "nil")
			return
		}
		walk(v.Elem(), depth)
	default:
		fmt.Println(pad+v.Kind().String(), v.Type(), v.Interface())
	}
}

func main() {
	walk(reflect.ValueOf([]Pair{{"a", ID(3)}, {"b", map[string]any{"x": 1.5, "y": nil}}, {"c", &Pair{"d", "e"}}}), 0)
}
