package syscall

import "reflect"

// Value stores the map of stdlib values per package
var Value = map[string]map[string]reflect.Value{}

// Wrapper stores the map of stdlib interface wrapper types per package
var Wrapper = map[string]map[string]reflect.Type{}

//go:generate ../../cmd/goexports/goexports syscall
