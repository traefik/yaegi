package unsafe

import "reflect"

// Symbols stores the map of unsafe package symbols
var Symbols = map[string]map[string]reflect.Value{}

//go:generate ../../cmd/goexports/goexports unsafe
