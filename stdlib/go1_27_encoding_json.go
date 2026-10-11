//go:build go1.27

package stdlib

import (
	"encoding/json"
	"reflect"
)

func init() {
	// Go 1.27 defines Marshaler and Unmarshaler in encoding/json/v2 and
	// aliases them from encoding/json. reflect reports the v2 package path,
	// so the interface wrappers have to be registered there too.
	Symbols["encoding/json/v2/v2"] = map[string]reflect.Value{
		"Marshaler":    reflect.ValueOf((*json.Marshaler)(nil)),
		"Unmarshaler":  reflect.ValueOf((*json.Unmarshaler)(nil)),
		"_Marshaler":   reflect.ValueOf((*_encoding_json_Marshaler)(nil)),
		"_Unmarshaler": reflect.ValueOf((*_encoding_json_Unmarshaler)(nil)),
	}
}
