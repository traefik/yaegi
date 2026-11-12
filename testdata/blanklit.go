package main

import (
	"fmt"
	"math"
	"reflect"
)

// A composite literal does not set blank (_) fields, and clear(m) during a
// range over m stops the loop, also for NaN keys.

var g = struct{ a, _, c int }{1, 2, 3}

type T struct {
	x int
	_ string
	y int
}

func main() {
	l := struct{ a, _, c int }{4, 5, 6}
	t := T{7, "blank", 8}
	fmt.Println(reflect.ValueOf(g).Field(1).Int(), reflect.ValueOf(l).Field(1).Int(),
		reflect.ValueOf(t).Field(1).String() == "")
	fmt.Printf("%v %v %+v\n", g, l, t)

	m := map[float64]int{}
	for i := 0; i < 8; i++ {
		m[math.NaN()] = i
	}
	n := 0
	for range m {
		if n == 0 {
			for i := 0; i < 10; i++ {
				m[float64(i)] = i
			}
			clear(m)
		}
		n++
	}
	fmt.Println(n, len(m))
}
