package interp

import "reflect"

// convertFn is the signature of a symbol converter.
type convertFn func(from, to reflect.Type) func(src, dest reflect.Value)

// hooks are external symbol bindings.
type hooks struct {
	convert []convertFn
}

func (h *hooks) Parse(m map[string]reflect.Value) {
	if con, ok := getConvertFn(m["convert"]); ok {
		h.convert = append(h.convert, con)
	}
}

func getConvertFn(v reflect.Value) (convertFn, bool) {
	if !v.IsValid() {
		return nil, false
	}
	fn, ok := v.Interface().(func(from, to reflect.Type) func(src, dest reflect.Value))
	if !ok {
		return nil, false
	}
	return fn, true
}
