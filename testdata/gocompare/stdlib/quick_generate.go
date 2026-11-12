// testing/quick calls the Generate method of a type of the program to make
// the arguments of the function it checks (quick.Generator).
package main

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing/quick"
)

type small int

func (small) Generate(r *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(small(r.Intn(10)))
}

type pair struct{ A, B small }

func (pair) Generate(r *rand.Rand, size int) reflect.Value {
	a := small(r.Intn(5))
	return reflect.ValueOf(pair{a, a + 1})
}

func main() {
	max, calls := 0, 0
	err := quick.Check(func(s small, n uint8) bool {
		calls++
		if int(s) > max {
			max = int(s)
		}
		return true
	}, nil)
	fmt.Println("small:", err, max < 10, calls)

	err = quick.Check(func(p pair) bool { return p.B == p.A+1 && p.A < 5 }, &quick.Config{MaxCount: 20})
	fmt.Println("pair:", err)

	err = quick.CheckEqual(func(s small) int { return int(s) * 2 }, func(s small) int { return int(s) + int(s) }, nil)
	fmt.Println("CheckEqual:", err)

	err = quick.Check(func(s small) bool { return s < 0 }, nil)
	ce, ok := err.(*quick.CheckError)
	fmt.Println("failing:", ok, ok && ce.Count == 1, ok && len(ce.In) == 1)

	// a parameter of an interface type with a Generate method is not a
	// Generator: quick cannot make its values
	err = quick.Check(func(g gen) bool { return true }, nil)
	fmt.Println("interface:", err != nil)
}

type gen interface {
	Generate(r *rand.Rand, size int) reflect.Value
}
