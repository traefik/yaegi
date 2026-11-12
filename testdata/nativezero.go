// Values equal by == are not the same value: 0 and -0, MyInt(1) and 1 in an
// interface, complex(-0, 0) and 0. After a native call, yaegi v2 writes back
// the maps and slices that it gave to native code. It must keep each entry
// as native code left it: a read only call (json.Marshal) changes nothing,
// and a -0 written by native code (json.Unmarshal, fmt.Sscan) is kept.
package main

import (
	"encoding/json"
	"fmt"
	"math"
)

type MyInt int

type C struct {
	F map[string]float64
	A map[string]any
	S []any
}

type MF struct{ MF map[string]float64 }

func sign(f float64) string {
	if math.Signbit(f) {
		return "-0"
	}
	return "+0"
}

func main() {
	negZero := math.Copysign(0, -1)

	// A read only call keeps the sign of each zero and the dynamic types.
	c := C{
		F: map[string]float64{"a": 0, "b": negZero, "c": 0, "d": negZero},
		A: map[string]any{"a": MyInt(1), "b": 1, "c": MyInt(1), "d": 1},
		S: []any{MyInt(1), 1, 0.0, negZero},
	}
	for range 5 {
		if _, err := json.Marshal(&c); err != nil {
			panic(err)
		}
	}
	fmt.Println(sign(c.F["a"]), sign(c.F["b"]), sign(c.F["c"]), sign(c.F["d"]))
	fmt.Printf("%T %T %T %T\n", c.A["a"], c.A["b"], c.A["c"], c.A["d"])
	fmt.Printf("%T %T %s %s\n", c.S[0], c.S[1], sign(c.S[2].(float64)), sign(c.S[3].(float64)))

	// A -0 written by native code over a 0 is kept.
	mf := map[string]float64{"a": 0}
	if err := json.Unmarshal([]byte(`{"a":-0}`), &mf); err != nil {
		panic(err)
	}
	fmt.Println("map:", sign(mf["a"]))
	st := MF{MF: map[string]float64{"a": 0}}
	if err := json.Unmarshal([]byte(`{"MF":{"a":-0}}`), &st); err != nil {
		panic(err)
	}
	fmt.Println("field:", sign(st.MF["a"]))
	sl := []any{0.0}
	if err := json.Unmarshal([]byte(`[-0]`), &sl); err != nil {
		panic(err)
	}
	fmt.Println("slice:", sign(sl[0].(float64)))

	// A complex -0 written by native code over a 0 is kept.
	cs := []complex128{0}
	if _, err := fmt.Sscan("(-0+0i)", &cs[0]); err != nil {
		panic(err)
	}
	fmt.Println("complex:", cs[0], sign(real(cs[0])))
}
