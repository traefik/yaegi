package interp

import "reflect"

// convertFn is the signature of a symbol converter.
type convertFn func(from, to reflect.Type) func(src, dest reflect.Value)

type convertHook struct {
	fn                  convertFn
	ownedUnsafeIdentity bool
}

// hooks are external symbol bindings.
type hooks struct {
	convert []convertHook
}

func (h *hooks) Parse(m map[string]reflect.Value) {
	if con, ok := getConvertFn(m["convert"]); ok {
		ownedUnsafeIdentity := false
		if marker := m["convertOwnedUnsafeIdentity"]; marker.IsValid() && marker.Kind() == reflect.Bool {
			ownedUnsafeIdentity = marker.Bool()
		}
		h.convert = append(h.convert, convertHook{fn: con, ownedUnsafeIdentity: ownedUnsafeIdentity})
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
