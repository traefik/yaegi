// AOT test program: decode hooks of github.com/mitchellh/mapstructure
// written by the plugin, of the three forms: func(reflect.Kind,
// reflect.Kind, any), func(reflect.Type, reflect.Type, any) and
// func(reflect.Value, reflect.Value). The form with reflect.Kind was refused
// in AOT mode ("invalid decode hook signature"): with the VM reflect,
// reflect.Kind of the program had no native counterpart, so the twin found
// the func not convertible to mapstructure.DecodeHookFuncKind. (From the
// reflect battery of the modes challenge, case lib_mapstructure in AOT
// mode.)
//
//aot:lib github.com/mitchellh/mapstructure
package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
)

func kindHook(f reflect.Kind, t reflect.Kind, data any) (any, error) {
	if f != reflect.String || t != reflect.Slice {
		return data, nil
	}
	return strings.Split(data.(string), ","), nil
}

func typeHook(f reflect.Type, t reflect.Type, data any) (any, error) {
	if f.Kind() == reflect.String && t.Kind() == reflect.Int {
		return len(data.(string)), nil
	}
	return data, nil
}

func valueHook(f reflect.Value, t reflect.Value) (any, error) {
	return f.Interface(), nil
}

type C struct {
	L []string
	N int
}

func main() {
	for _, h := range []any{kindHook, typeHook, valueHook, mapstructure.ComposeDecodeHookFunc(kindHook, typeHook)} {
		var c C
		dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{DecodeHook: h, Result: &c})
		if err != nil {
			fmt.Println(err)
			continue
		}
		err = dec.Decode(map[string]any{"l": "a,b", "n": "xyz"})
		fmt.Println(err, c)
	}
	// reflect.Kind values given to native code and back
	kinds := map[reflect.Kind]string{reflect.String: "s", reflect.Slice: "l"}
	fmt.Println(kinds[reflect.String], reflect.Slice, fmt.Sprint(reflect.Map), reflect.Kind(3))
}
