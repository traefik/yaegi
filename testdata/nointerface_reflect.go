package main

// With GOEXPERIMENT=fieldtrack, the methods marked //go:nointerface are not
// in the run-time method set, so reflect does not see them either:
// Implements, NumMethod, Method and MethodByName (of reflect.Type and of
// reflect.Value). Run with the VM reflect (-opaque): the default mode gives
// reflect a native copy of the type (the reflect wall).

import (
	"fmt"
	"reflect"
)

type U struct{}

//go:nointerface
func (*U) Bad() {}

//go:nointerface
func (*U) String() string { return "U!" }

func (*U) Good() {}

// All the exported methods of W are marked.
type W struct{}

//go:nointerface
func (*W) A() {}

//go:nointerface
func (*W) B() {}

//go:nointerface
func (*W) C() {}

//go:nointerface
func (*W) D() {}

type T struct{ U } // promoted methods

func show(x any) {
	t := reflect.TypeOf(x)
	bad := reflect.TypeOf((*interface{ Bad() })(nil)).Elem()
	good := reflect.TypeOf((*interface{ Good() })(nil)).Elem()
	str := reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
	fmt.Println(t, "Implements", t.Implements(bad), t.Implements(good), t.Implements(str))
	fmt.Print(t, " NumMethod ", t.NumMethod(), ":")
	for i := 0; i < t.NumMethod(); i++ {
		fmt.Print(" ", t.Method(i).Name)
	}
	fmt.Println()
	_, okBad := t.MethodByName("Bad")
	_, okGood := t.MethodByName("Good")
	v := reflect.ValueOf(x)
	fmt.Println(t, "MethodByName", okBad, okGood, "Value", v.NumMethod(), v.MethodByName("Bad").IsValid(), v.MethodByName("Good").IsValid())
}

func main() {
	show(new(U))
	show(new(W))
	show(&T{})
	fmt.Println(new(U), &T{})
}
