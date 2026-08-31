package main

import (
	"encoding/json"
	"fmt"
)

type A struct{ B string }

func (a A) String() string { return "A<" + a.B + ">" }

func main() {
	// Marshalling of values held in a non-empty interface variable (#633).
	var s fmt.Stringer = A{B: "v"}
	j, err := json.Marshal(s)
	fmt.Println(string(j), err)

	var p fmt.Stringer = &A{B: "p"}
	jp, errp := json.Marshal(p)
	fmt.Println(string(jp), errp)

	// Slices and maps of interface-held interpreted values are passed to
	// binary code untouched: in-place mutation contracts (sort.Slice) take
	// priority, so marshaling them remains unsupported (loud error, as
	// before the concrete-argument change).
	sl := []fmt.Stringer{A{B: "1"}}
	_, errsl := json.Marshal(sl)
	fmt.Println("slice marshal err:", errsl != nil)

	// Unmarshal into an empty interface variable.
	var x interface{}
	erru := json.Unmarshal([]byte(`{"B":"u"}`), &x)
	fmt.Println(x, erru)

	// Method dispatch through the non-empty interface is unchanged.
	fmt.Println(s.String(), p.String())
}

// Output:
// {"B":"v"} <nil>
// {"B":"p"} <nil>
// slice marshal err: true
// map[B:u] <nil>
// A<v> A<p>
