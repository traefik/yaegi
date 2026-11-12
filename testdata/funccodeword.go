// The data word of a func in an interface points to a funcval, whose first
// word is the code pointer: reading it gives the address that
// reflect.ValueOf(f).Pointer() gives, and runtime.FuncForPC names the
// function (the funcPC of GOROOT test fixedbugs/issue17381.go).
package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"unsafe"
)

func funcPC(f interface{}) uintptr {
	var ptr uintptr
	return **(**uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(&f)) + unsafe.Sizeof(ptr)))
}

type eface struct {
	typ  unsafe.Pointer
	data *uintptr
}

func funcPC2(f any) uintptr { return *(*eface)(unsafe.Pointer(&f)).data }

func double(x int) int { return 2 * x }

type counter struct{ n int }

func (c *counter) incr() { c.n++ }

func name(pc uintptr) string {
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "<nil>"
	}
	n := fn.Name()
	return n[strings.LastIndex(n, "/")+1:]
}

func main() {
	k := 3
	closure := func() int { return k }
	c := &counter{}
	for _, f := range []any{double, closure, c.incr, strings.ToUpper, runtime.GC} {
		pc := funcPC(f)
		fmt.Println(name(pc), pc == funcPC2(f), pc == reflect.ValueOf(f).Pointer())
	}
	fmt.Println(funcPC(double) == funcPC(double), funcPC(double) != funcPC(main))

	// the func read through unsafe is still the func
	f := any(double)
	_ = funcPC(f)
	fmt.Println(f.(func(int) int)(21))
}
