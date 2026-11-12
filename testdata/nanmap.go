package main

import (
	"fmt"
	"math"
	"sort"
)

// Maps with NaN keys: every NaN is a distinct key, found by range only.

func main() {
	m := map[float64]int{math.NaN(): 1, math.NaN(): 2, 3: 3}
	m[math.NaN()] = 4
	delete(m, math.NaN())
	var vs []int
	nans := 0
	for k, v := range m {
		if k != k {
			nans++
		}
		vs = append(vs, v)
	}
	sort.Ints(vs)
	fmt.Println(len(m), nans, vs)

	var im = map[any]string{math.NaN(): "a", 1: "b"}
	n := 0
	for k := range im {
		if f, ok := k.(float64); ok && math.IsNaN(f) {
			n++
		}
	}
	fmt.Println(len(im), n)

	clear(m)
	fmt.Println(len(m))
}
