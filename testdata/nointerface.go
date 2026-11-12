package main

// With GOEXPERIMENT=fieldtrack, a method marked //go:nointerface does not
// count for type assertions and type switches, also when it is promoted
// or declared on a generic type (GOROOT/test typeparam/mdempsky/15.go,
// fixedbugs/issue47928.go). This is also true when a blank line separates
// the directive from the method (gc applies it to the next declaration), and
// for the interfaces that native code checks (fmt sees no String or Error
// method). The reflect paths are in nointerface_reflect.go.

import "fmt"

type U struct{}

//go:nointerface
func (*U) Bad() {}

func (*U) Good() {}

//go:nointerface

func (*U) Gap() {}

//go:nointerface
func (*U) String() string { return "U!" }

//go:nointerface
func (*U) Error() string { return "U error" }

type T struct{ U }

type X[P any] struct{ T }

//go:nointerface
func (X[P]) XBad() {}

func (X[P]) XGood() {}

type S struct{}

func (*S) String() string { return "S!" }

func check[V any](name string) {
	var i any = new(V)
	_, bad := i.(interface{ Bad() })
	_, good := i.(interface{ Good() })
	_, xbad := i.(interface{ XBad() })
	_, xgood := i.(interface{ XGood() })
	fmt.Println(name, bad, good, xbad, xgood)
	switch i.(type) {
	case interface{ Bad() }:
		fmt.Println(name, "matched Bad")
	case interface{ Good() }:
		fmt.Println(name, "matched Good")
	}
}

func main() {
	check[U]("U")
	check[T]("T")
	check[X[int]]("X")
	(&T{}).Bad() // direct calls still work
	X[int]{}.XBad()

	var u any = new(U)
	_, gap := u.(interface{ Gap() })
	_, str := u.(fmt.Stringer)
	_, isErr := u.(error)
	fmt.Println("Gap", gap, "Stringer", str, "error", isErr)

	// Native fmt must not see String or Error.
	fmt.Println(new(U), &T{}, new(S))
	fmt.Printf("%v %s %+v\n", new(U), new(U), &T{})
	fmt.Println(fmt.Sprint(new(U)) == "&{}", fmt.Sprintf("%v", []any{new(S)}))
	fmt.Println(new(U).String(), new(U).Error(), (&T{}).String()) // direct calls
}
