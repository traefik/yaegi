// Range loops over maps and strings: the VM gives their iterators back to
// a pool at the end of the loop (vm/features.go). Nested loops, loops left
// early, deletes and inserts during a loop, loops on goroutines, closures
// and deferred method calls must not see it.
package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type T struct{ name string }

func (t T) Say(sep string, n int) { fmt.Println("deferred", t.name, sep, n) }

func keys(m map[string]int) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func main() {
	small := map[string]int{"a": 1, "b": 2, "c": 3}
	big := map[int]int{}
	for i := 0; i < 50; i++ {
		big[i] = i * i
	}
	// nested loops over the same maps
	n := 0
	for k1, v1 := range small {
		for k2, v2 := range small {
			if k1 < k2 {
				n += v1 * v2
			}
		}
		for _, v := range big {
			n += v % 7
		}
	}
	fmt.Println("nested", n)
	// loops left early, then new loops
	for i := 0; i < 5; i++ {
		for k := range big {
			if k >= 0 {
				break
			}
		}
		s := 0
		for _, v := range small {
			s += v
		}
		fmt.Print(s, " ")
	}
	fmt.Println()
	// deletes and inserts during a loop
	m := map[string]int{"x": 1, "y": 2, "z": 3}
	for k := range m {
		delete(m, k)
	}
	fmt.Println("emptied", len(m))
	for i := 0; i < 20; i++ {
		m[fmt.Sprint("k", i)] = i
	}
	seen := 0
	for k, v := range m {
		if v%2 == 0 {
			delete(m, k)
		}
		seen++
	}
	fmt.Println("left", len(m), seen >= 10, keys(m)[:3])
	// strings
	var b strings.Builder
	for i, r := range "héllo, wörld" {
		for _, r2 := range "ab" {
			b.WriteRune(r2)
		}
		fmt.Fprint(&b, i, string(r))
	}
	fmt.Println(b.String())
	// goroutines
	var wg sync.WaitGroup
	sums := make([]int, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < 200; r++ {
				for _, v := range big {
					sums[g] += v
				}
				for _, v := range small {
					sums[g] += v
				}
				for _, c := range "abc" {
					sums[g] += int(c)
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("goroutines", sums)
	// closures over loop values, and deferred method calls
	var fs []func() string
	for _, k := range keys(small) {
		v := small[k]
		fs = append(fs, func() string { return fmt.Sprint(k, v) })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()
	func() {
		t := T{"t1"}
		defer t.Say("-", 1)
		t = T{"t2"}
		defer t.Say("+", 2)
	}()
}
