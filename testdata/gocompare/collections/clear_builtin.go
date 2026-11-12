// The clear builtin: on slices it zeroes the elements and keeps len and
// cap, on a sub-slice only that part, on maps it removes every key (NaN keys
// too), and on nil it does nothing.
package main

import (
	"fmt"
	"math"
)

type T struct {
	A int
	B string
	C *int
}

func main() {
	s := []int{1, 2, 3, 4, 5}
	clear(s[1:3])
	fmt.Println(s, len(s), cap(s))
	clear(s)
	fmt.Println(s, len(s))

	x := 5
	ts := []T{{1, "a", &x}, {2, "b", nil}}
	clear(ts)
	fmt.Println(ts[0].A, ts[0].B == "", ts[0].C == nil, len(ts))

	strs := make([]string, 3, 6)
	strs[0], strs[1], strs[2] = "x", "y", "z"
	clear(strs)
	fmt.Printf("%q %d %d\n", strs, len(strs), cap(strs))
	full := strs[:6]
	full[5] = "w"
	clear(strs)
	fmt.Printf("%q\n", full)

	m := map[string]int{"a": 1, "b": 2, "c": 3}
	alias := m
	clear(m)
	fmt.Println(len(m), len(alias), m)
	m["d"] = 4
	fmt.Println(alias)

	f := map[float64]int{}
	f[math.NaN()] = 1
	f[math.NaN()] = 2
	f[1] = 3
	fmt.Println(len(f))
	clear(f)
	fmt.Println(len(f), f)

	var nilS []int
	var nilM map[int]int
	clear(nilS)
	clear(nilM)
	fmt.Println(nilS == nil, nilM == nil)

	// clear on an array slice zeroes the array.
	arr := [4]byte{1, 2, 3, 4}
	clear(arr[2:])
	fmt.Println(arr)

	// clear during a range on a map: no further keys are seen.
	mm := map[int]bool{1: true, 2: true, 3: true, 4: true}
	n := 0
	for range mm {
		n++
		clear(mm)
	}
	fmt.Println(n, len(mm))

	// Slices of slices and maps of maps.
	ss := [][]int{{1}, {2}}
	inner := ss[0]
	clear(ss)
	fmt.Println(ss[0] == nil, inner)
	mmm := map[string]map[string]int{"a": {"x": 1}}
	in := mmm["a"]
	clear(mmm)
	fmt.Println(len(mmm), in)
}
