//go:build goexperiment.jsonv2 && go1.27

package vm

import "encoding/json/jsontext"

// jsonv2Generics are the generic functions of encoding/json/jsontext
// without the AOT copy of jsontext (aotgeneric.go has the hooks of the
// copy). jsontext exists with GOEXPERIMENT=jsonv2 only: the default of Go
// 1.27, an experiment of Go 1.26, where it is not part of the API yet
// (generics_nojsonv2.go). Bytes is
// ~[]byte | ~string: the native instance for string or []byte does the
// work. They are always used: their source calls internal/jsonwire, which
// has no native symbols, so an instance that a loader built from source
// stopped with "jsonwire.AppendQuote not in registry": a call from a package
// of a plugin that is not its main package, or TestStringOption of
// encoding/json.
func jsonv2Generics() map[string]genericImpl {
	return map[string]genericImpl{
		"encoding/json/jsontext.AppendQuote":   {true, bytesGeneric(jsontext.AppendQuote[string], jsontext.AppendQuote[[]byte])},
		"encoding/json/jsontext.AppendUnquote": {true, bytesGeneric(jsontext.AppendUnquote[string], jsontext.AppendUnquote[[]byte])},
		"encoding/json/jsontext.AppendFormat":  {true, bytesGeneric(jsontext.AppendFormat[string], jsontext.AppendFormat[[]byte])},
	}
}
