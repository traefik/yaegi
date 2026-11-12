// Large maps: growth to many thousand keys of several types, deletes and
// re-inserts, lookups after growth, and checks on len, sums and sorted
// keys, never on the range order.
package main

import (
	"fmt"
	"slices"
	"strconv"
)

type key struct {
	A int32
	B string
}

func main() {
	m := map[int64]int64{}
	for i := int64(0); i < 20000; i++ {
		m[i*7919%20011] = i
	}
	fmt.Println(len(m))
	for i := int64(0); i < 20000; i += 2 {
		delete(m, i*7919%20011)
	}
	fmt.Println(len(m))
	var sum int64
	for _, v := range m {
		sum += v
	}
	fmt.Println(sum)
	miss := 0
	for i := int64(0); i < 20000; i++ {
		v, ok := m[i*7919%20011]
		if ok != (i%2 == 1) || (ok && v != i) {
			miss++
		}
	}
	fmt.Println(miss)
	for i := int64(0); i < 20000; i += 2 {
		m[i*7919%20011] = -i
	}
	sum = 0
	for _, v := range m {
		sum += v
	}
	fmt.Println(len(m), sum)

	sm := map[string]int{}
	for i := range 5000 {
		sm[strconv.Itoa(i)] = i
	}
	for i := range 5000 {
		if i%3 == 0 {
			delete(sm, strconv.Itoa(i))
		}
	}
	keys := make([]string, 0, len(sm))
	for k := range sm {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	fmt.Println(len(sm), keys[:5], keys[len(keys)-3:])

	km := map[key][]int{}
	for i := range 3000 {
		k := key{int32(i % 100), strconv.Itoa(i % 7)}
		km[k] = append(km[k], i)
	}
	fmt.Println(len(km), km[key{5, "5"}][:3], len(km[key{0, "0"}]))

	// Map with big values.
	type big struct {
		Arr [64]int64
		S   string
	}
	bm := map[int]big{}
	for i := range 300 {
		var b big
		b.Arr[i%64] = int64(i)
		b.S = strconv.Itoa(i)
		bm[i] = b
	}
	fmt.Println(len(bm), bm[299].Arr[299%64], bm[299].S, bm[64].Arr[0])

	// Interface keys growth.
	im := map[any]int{}
	for i := range 1000 {
		im[i] = i
		im[strconv.Itoa(i)] = -i
		im[float64(i)+0.5] = i * 2
	}
	fmt.Println(len(im), im[999], im["999"], im[999.5], im[int64(5)])

	// Clear then reuse.
	clear(m)
	for i := int64(0); i < 100; i++ {
		m[i] = i
	}
	fmt.Println(len(m), m[99])
}
