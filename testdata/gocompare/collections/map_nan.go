// NaN map keys: each insert adds a new entry, a lookup never finds it,
// delete does nothing, len counts them, fmt prints them, and a range sees
// them all; also NaN in interface keys and in struct and array keys.
package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
)

func main() {
	m := map[float64]int{}
	nan := math.NaN()
	m[nan] = 1
	m[nan] = 2
	m[math.NaN()] = 3
	m[1] = 4
	fmt.Println(len(m))
	v, ok := m[nan]
	fmt.Println(v, ok)
	delete(m, nan)
	fmt.Println(len(m))
	// fmt sorts the keys, but NaN keys are equal for its sort: their order
	// is the range order. Print the values sorted instead.
	fmt.Println(slices.Sorted(maps.Values(m)))
	sum, nans := 0, 0
	for k, v := range m {
		sum += v
		if k != k {
			nans++
		}
	}
	fmt.Println(sum, nans)
	m[nan]++
	fmt.Println(len(m), slices.Sorted(maps.Values(m)))
	// With equal values for every NaN key, fmt output is fixed.
	same := map[float64]string{nan: "n", 2: "two", -1: "m"}
	same[nan] = "n"
	fmt.Println(same, len(same))

	// Delete all through clear.
	clear(m)
	fmt.Println(len(m))

	f32 := map[float32]string{}
	f32[float32(math.NaN())] = "a"
	f32[float32(math.NaN())] = "b"
	fmt.Println(len(f32))

	im := map[any]int{}
	im[nan] = 1
	im[nan] = 2
	im[float32(nan)] = 3
	fmt.Println(len(im), im[nan])

	type S struct{ F float64 }
	sm := map[S]int{}
	sm[S{nan}] = 1
	sm[S{nan}] = 2
	sm[S{0}] = 3
	fmt.Println(len(sm), sm[S{0}], sm[S{nan}])

	am := map[[2]float64]bool{}
	am[[2]float64{1, nan}] = true
	am[[2]float64{1, nan}] = true
	fmt.Println(len(am), am[[2]float64{1, nan}])

	cm := map[complex128]int{}
	cm[complex(nan, 0)] = 1
	cm[complex(nan, 0)] = 2
	cm[complex(0, nan)] = 3
	fmt.Println(len(cm))

	// Infinities are ordinary keys.
	inf := map[float64]int{math.Inf(1): 1, math.Inf(-1): 2}
	inf[math.Inf(1)]++
	fmt.Println(inf, len(inf))

	// NaN compared.
	fmt.Println(nan == nan, nan != nan, [1]float64{nan} == [1]float64{nan}, S{nan} == S{nan})
	var a, b any = nan, nan
	fmt.Println(a == b)
}
