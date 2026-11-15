// Keys of named, pointer and struct types through a context wrapper of
// the program (pass) under a native context (WithValue, WithCancel,
// WithTimeout, WithoutCancel): the native context gives the wrapper the
// key in its native form, and the wrapper asks its embedded context with
// it. Before, yaegi v2 printed <nil> for these keys.

package main

import (
	"context"
	"fmt"
)

// pass wraps a context, as middleware libraries do.
type pass struct{ context.Context }

type sk struct{}

type ik int

var pk = new(int)

func main() {
	c := context.WithValue(context.Background(), sk{}, "struct")
	c = context.WithValue(c, ik(1), "int")
	c = context.WithValue(c, pk, "ptr")
	c = context.WithValue(c, "str", "s")
	fmt.Println(pass{c}.Value(sk{}), pass{c}.Value(ik(1)), pass{c}.Value(pk), pass{c}.Value("str"))
	w1 := context.WithValue(pass{c}, "o", 1)
	fmt.Println(w1.Value(sk{}), w1.Value(ik(1)), w1.Value(pk), w1.Value("str"), w1.Value("o"))
	w2, cancel := context.WithCancel(pass{c})
	defer cancel()
	fmt.Println(w2.Value(sk{}), w2.Value(ik(1)), w2.Value(pk), w2.Value("str"))
	w3, cancel3 := context.WithTimeout(pass{w2}, 0)
	defer cancel3()
	fmt.Println(w3.Value(sk{}), w3.Value(ik(1)), w3.Value(pk), w3.Value("str"))
	// native code asks the wrapper
	w4 := context.WithoutCancel(pass{c})
	fmt.Println(w4.Value(sk{}), w4.Value(ik(1)), w4.Value(pk), w4.Value("str"))
	// a different key of the same type is not found
	fmt.Println(w1.Value(ik(2)), w1.Value(new(int)))
}
