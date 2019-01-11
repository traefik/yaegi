package syscall

import "reflect"

// Value stores the map of stdlib values per package
var Value = map[string]map[string]reflect.Value{}

// Type stores the map of stdlib values per package
var Type = map[string]map[string]reflect.Type{}

//go:generate ../../cmd/goexports/goexports syscall
