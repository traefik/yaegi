package main

// Value.SetIterKey and Value.SetIterValue with the iterator of MapRange
// (fxamacker/cbor encodes maps this way).

import (
	"fmt"
	"reflect"
	"sort"
)

type E struct{ M map[string]int }

func main() {
	v := reflect.ValueOf(E{M: map[string]int{"a": 1, "b": 2}}).Field(0)
	k := reflect.New(v.Type().Key()).Elem()
	e := reflect.New(v.Type().Elem()).Elem()
	var out []string
	for it := v.MapRange(); it.Next(); {
		k.SetIterKey(it)
		e.SetIterValue(it)
		out = append(out, fmt.Sprint(k.Interface(), "=", e.Interface()))
	}
	sort.Strings(out)
	fmt.Println(out)
}
