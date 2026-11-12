// The value recovered from a failed type assertion is a
// *runtime.TypeAssertionError.
package main

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"
)

func check(name string, f func()) {
	defer func() {
		e := recover()
		_, isRT := e.(runtime.Error)
		err, _ := e.(error)
		var tae *runtime.TypeAssertionError
		fmt.Println(name, e, isRT, errors.As(err, &tae))
	}()
	f()
}

func main() {
	var a any = 1
	check("a", func() { _ = a.(string) })
	var n any
	check("b", func() { _ = n.(int) })
	var s any = []string{"x"}
	check("c", func() { _ = s.(map[string]int) })
	var d any = time.Second
	check("d", func() { _ = d.(time.Time) })
	var w any = "x"
	check("e", func() { _ = w.(io.Reader) })
	defer func() {
		e := recover()
		tae, ok := e.(*runtime.TypeAssertionError)
		fmt.Printf("f %T %v %v\n", e, ok, tae)
	}()
	_ = a.([]byte)
}
