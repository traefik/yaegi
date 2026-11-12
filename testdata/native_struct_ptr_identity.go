// A pointer of the program to a native struct (var z big.Float) and the
// same pointer given back by native code (z.Copy(&x) returns &z) are equal,
// as in Go, also as map keys (ExampleFloat_Copy of package math/big).
package main

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
)

type holder struct {
	f big.Float
	n int
}

func main() {
	var x, z big.Float
	x.SetFloat64(1.23)
	r := z.Copy(&x)
	fmt.Println(r == &z, &z == &x)
	z.SetInt64(42)
	fmt.Println(r == &z, r.String())
	m := map[*big.Float]int{&z: 1}
	fmt.Println(m[r], len(m))
	m[r] = 2
	fmt.Println(m[&z], len(m))

	var i big.Int
	fmt.Println(i.SetInt64(5) == &i, new(big.Int).SetInt64(5) == &i)

	// a native struct in a field of a program struct
	h := &holder{}
	fmt.Println(h.f.SetInt64(7) == &h.f)
	z = *big.NewFloat(2) // assignment keeps the memory
	fmt.Println(r == &z, r.String())

	var buf bytes.Buffer
	var sb strings.Builder
	fmt.Println(&buf == &buf, any(&sb) == any(&sb))
}
