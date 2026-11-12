// A failed type assertion in log/slog (an AOT copy) panics like gc: a
// *runtime.TypeAssertionError for a native dynamic type, and a message that
// names the missing method. Go cannot make the native type of main.plain,
// so only its message is checked.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
)

type plain struct{ n int }

func try(name string, native bool, f func()) {
	defer func() {
		r := recover()
		err, _ := r.(error)
		var te *runtime.TypeAssertionError
		fmt.Printf("%s: %v\n", name, r)
		_, isRuntime := r.(runtime.Error)
		fmt.Printf("  runtime.Error=%v\n", isRuntime)
		if native && errors.As(err, &te) {
			fmt.Printf("  %T\n", te)
		}
	}()
	f()
}

func main() {
	try("int", true, func() { slog.IntValue(1).LogValuer() })
	try("string", true, func() { slog.StringValue("s").LogValuer() })
	try("plain", false, func() { slog.AnyValue(plain{1}).LogValuer() })
	// unnamed types of the program have a native type of the same name
	try("struct", true, func() { slog.AnyValue(struct{ A int }{1}).LogValuer() })
	try("slice", true, func() { slog.AnyValue([]struct{ A int }{}).LogValuer() })
}
