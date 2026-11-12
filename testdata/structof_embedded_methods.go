// A program struct that embeds a native type with methods, given to native
// code that asserts it to an interface. reflect.StructOf promotes the
// methods of the embedded field into tables the runtime cannot use (Go
// issue 15924): the assertion ended the whole process with "fatal error:
// runtime: type offset base pointer out of range" (TestAliasing of
// math/big, with a struct{ *big.Int } given to testing/quick).
package main

import (
	"fmt"
	"math/rand"
	"testing/quick"
	"time"
)

type D struct{ time.Duration }

type P struct {
	*time.Location
}

func main() {
	n := 0
	err := quick.Check(func(d D) bool { n++; return d.Duration == d.Duration }, &quick.Config{Rand: rand.New(rand.NewSource(1)), MaxCount: 5})
	fmt.Println(err, n)

	m := 0
	err = quick.Check(func(a, b D, s string) bool { m++; return a.String() != "" && len(s) >= 0 }, &quick.Config{Rand: rand.New(rand.NewSource(2)), MaxCount: 3})
	fmt.Println(err, m)

	// The program still sees the promoted methods.
	p := P{time.UTC}
	fmt.Println(p.String(), D{3 * time.Second}.String())
}
