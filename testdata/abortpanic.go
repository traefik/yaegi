// panic(http.ErrAbortHandler) leaves the VM as the native sentinel (so
// that net/http sees it), also from a callback that native code calls.
// recover must still give a value equal to http.ErrAbortHandler, and
// other panic values must not change.
//
// This file only checks that recover keeps go run parity with the change
// (it also passes without it: recover gives the panic value in both
// paths). The regression test of the change is TestPluginAbortHandler in
// vm/stdio_test.go.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
)

func viaNative(v any) (r any) {
	defer func() { r = recover() }()
	s := []int{2, 1}
	sort.Slice(s, func(i, j int) bool { panic(v) })
	return nil
}

func direct(v any) (r any) {
	defer func() { r = recover() }()
	panic(v)
}

type myErr struct{ s string }

func (e *myErr) Error() string { return e.s }

func main() {
	for _, get := range []func(any) any{viaNative, direct} {
		r := get(http.ErrAbortHandler)
		err, _ := r.(error)
		fmt.Println(r == http.ErrAbortHandler, errors.Is(err, http.ErrAbortHandler), r)
		w := fmt.Errorf("wrapped: %w", http.ErrAbortHandler)
		r = get(w)
		err, _ = r.(error)
		fmt.Println(r == w, r == http.ErrAbortHandler, errors.Is(err, http.ErrAbortHandler), r)
		e := &myErr{"mine"}
		r = get(e)
		fmt.Println(r == e, r)
		fmt.Println(get("text"), get(42))
	}
}
