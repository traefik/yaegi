package main

// unique.Handle[T] is an instance of a generic type of a native package: it
// uses the VM layout, and the VM reflect (-aot) knows its name. In default
// mode, native fmt only sees its layout, as for every interpreted named type
// (testdata/wall.go).

import (
	"fmt"
	"unique"
)

func main() {
	fmt.Printf("%T\n", unique.Make("hi"))
	fmt.Printf("%T\n", unique.Make(1))
	var x any = unique.Make("a")
	h, ok := x.(unique.Handle[string])
	fmt.Println(ok, h.Value())
}
