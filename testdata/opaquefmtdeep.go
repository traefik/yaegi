package main

// A value nested in interfaces a million times ([]any{[]any{...}}), given
// to native printers. Go does not walk the value for %T. yaegi v2 copied it for
// native code with a recursion on the host stack: a fatal Go stack overflow
// that killed the host, in every mode, and in -restricted -aot (plugins)
// since the printers get a native copy instead of an opaque handle. Deep
// values now get an opaque handle below a bound.

import (
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
)

func nest(n int) any {
	var v any = 0
	for i := 0; i < n; i++ {
		v = []any{v}
	}
	return v
}

func depth(x any) int {
	d := 0
	for {
		l, ok := x.([]any)
		if !ok {
			return d
		}
		x = l[0]
		d++
	}
}

func main() {
	v := nest(1_000_000)
	log.New(io.Discard, "", 0).Printf("%T", v)
	fmt.Println(fmt.Sprintf("%T", v))
	fmt.Println(fmt.Errorf("%T: %w", v, errors.New("inner")))
	fmt.Println(fmt.Sprintf("%T", nest(3_000_000)))

	// Values below the bound are printed and stored as before.
	s := fmt.Sprint(nest(20_000))
	fmt.Println(len(s), s[:5], s[len(s)-5:])
	var m sync.Map
	m.Store("k", v)
	x, _ := m.Load("k")
	fmt.Println("depth", depth(x))
}
