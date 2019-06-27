package syscall

import "reflect"

// Symbols stores the map of syscall package symbols
var Symbols = map[string]map[string]reflect.Value{}

//go:generate ../../cmd/goexports/goexports syscall
