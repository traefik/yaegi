package main

import (
	"fmt"
	"math"
	"reflect"
	"sort"
)

// clear during a range stops the range, NaN keys included, and maps with NaN
// keys keep all their entries when native code (reflect) reads them.

func main() {
	m := map[float64]int{}
	for i := 0; i < 8; i++ {
		m[math.NaN()] = i
	}
	m[1] = 100
	n := 0
	for range m {
		if n == 0 {
			for i := 0; i < 10; i++ {
				m[float64(i)+10] = i
			}
			clear(m)
		}
		n++
	}
	fmt.Println(n, len(m))

	for i := 0; i < 5; i++ {
		m[math.NaN()] = i
	}
	m[2] = 200
	v := reflect.ValueOf(m)
	fmt.Println(v.Len())
	var vs []int
	nans := 0
	it := v.MapRange()
	for it.Next() {
		if k := it.Key().Float(); k != k {
			nans++
		}
		vs = append(vs, int(it.Value().Int()))
	}
	sort.Ints(vs)
	fmt.Println(nans, vs)

	n = 0
	it = v.MapRange()
	for it.Next() {
		if n == 2 {
			v.Clear()
		}
		n++
	}
	fmt.Println(n, v.Len())
}
