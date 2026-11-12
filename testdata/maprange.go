package main

import (
	"fmt"
	"math"
	"sort"
)

// Range over a map while it changes. The VM walks the map live: an entry
// deleted before the range reaches it is not produced, and the
// "range m { delete(m, k); break }" idiom stays linear (GOROOT test
// maplinear.go: it was quadratic when each range copied all the keys).

func main() {
	// Iterate/delete idiom on a large map: quadratic would take minutes.
	const n = 100000
	m := map[int]int{}
	for i := 0; i < n; i++ {
		m[i] = i
	}
	sum := 0
	for i := 0; i < n; i++ {
		for k, v := range m {
			sum += v
			delete(m, k)
			break
		}
	}
	fmt.Println(len(m), sum)

	// Deleting entries not reached yet: they are never produced.
	m = map[int]int{}
	for i := 0; i < 100; i++ {
		m[i] = i
	}
	seen := map[int]bool{}
	for k := range m {
		if seen[k] {
			panic("entry produced twice")
		}
		seen[k] = true
		// Delete the partner of k: whichever comes second is not produced.
		delete(m, k^1)
	}
	fmt.Println(len(seen), len(m))

	// Values updated during the range are seen with their new value.
	m = map[int]int{1: 1, 2: 2, 3: 3}
	got := []int{}
	for k := range m {
		for j := range m {
			m[j] = j * 10
		}
		got = append(got, m[k])
	}
	sort.Ints(got)
	fmt.Println(got)

	// Keys and values come in pairs.
	s := map[string]int{"a": 1, "b": 2, "c": 3}
	var pairs []string
	for k, v := range s {
		pairs = append(pairs, fmt.Sprint(k, v))
	}
	sort.Strings(pairs)
	fmt.Println(pairs)

	// NaN keys: each one is produced once, with its value.
	f := map[float64]int{math.NaN(): 1, math.NaN(): 2, 1.5: 3}
	nans, total := 0, 0
	for k, v := range f {
		if k != k {
			nans++
		}
		total += v
	}
	fmt.Println(nans, total)

	// Clear during the range: nothing more is produced.
	c := map[int]bool{}
	for i := 0; i < 50; i++ {
		c[i] = true
	}
	count := 0
	for range c {
		count++
		clear(c)
	}
	fmt.Println(count, len(c))

	// Clear during the range of a large map (walked live): nothing more is
	// produced.
	lc := map[int]bool{}
	for i := 0; i < 100; i++ {
		lc[i] = true
	}
	count = 0
	for range lc {
		count++
		clear(lc)
	}
	fmt.Println(count, len(lc))

	// A large map with NaN keys: each entry is produced once, with its key
	// and its value.
	ln := map[float64]int{}
	for i := 0; i < 20; i++ {
		ln[math.NaN()] = 100
		ln[float64(i)] = i
	}
	nans, total = 0, 0
	for k, v := range ln {
		if k != k {
			nans++
		} else if int(k) != v {
			panic("key and value do not match")
		}
		total += v
	}
	fmt.Println(len(ln), nans, total)

	// Clear during the range of a large map with NaN keys.
	count = 0
	for range ln {
		count++
		clear(ln)
	}
	fmt.Println(count, len(ln))

	// Nil map.
	var nm map[string]int
	for range nm {
		panic("nil map has no entry")
	}
	fmt.Println("done")
}
