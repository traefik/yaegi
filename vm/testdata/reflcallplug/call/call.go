// Package call calls functions with reflect.Value.Call, like the rule
// parser of vulcand/predicate. The types of the results are defined by the
// package that imports call, not by call itself.
package call

import "reflect"

// Call calls f with args and returns its first result.
func Call(f any, args ...any) any {
	in := make([]reflect.Value, len(args))
	for i, a := range args {
		in[i] = reflect.ValueOf(a)
	}
	return reflect.ValueOf(f).Call(in)[0].Interface()
}

// Out returns the type of the first result of f.
func Out(f any) string { return reflect.TypeOf(f).Out(0).String() }
