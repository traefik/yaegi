// Package quickplug imports testing/quick, whose table is in
// stdsyms/sideeffect: a host that does not import that package refuses it
// at load time.
package quickplug

import (
	"reflect"
	"testing/quick"
)

func Run() bool {
	_, ok := quick.Value(reflect.TypeOf(0), nil)
	return ok
}
