// slices iterators: All, Values, Backward, Collect, AppendSeq, Sorted,
// SortedFunc, SortedStableFunc and Chunk, with early break, and custom
// iterator functions used with slices.Collect.
package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

func main() {
	s := []string{"c", "a", "d", "b"}
	for i, v := range slices.All(s) {
		fmt.Print(i, v, " ")
	}
	fmt.Println()
	for v := range slices.Values(s) {
		fmt.Print(v)
	}
	fmt.Println()
	for i, v := range slices.Backward(s) {
		fmt.Print(i, v, " ")
		if i == 1 {
			break
		}
	}
	fmt.Println()
	fmt.Println(slices.Collect(slices.Values(s)))
	fmt.Println(slices.Sorted(slices.Values(s)), s)
	fmt.Println(slices.AppendSeq([]string{"z"}, slices.Values(s)))
	fmt.Println(slices.SortedFunc(slices.Values([]int{3, 1, 2}), func(a, b int) int { return cmp.Compare(b, a) }))
	words := []string{"bb", "a", "ccc", "dd", "e"}
	fmt.Println(slices.SortedStableFunc(slices.Values(words), func(a, b string) int { return cmp.Compare(len(a), len(b)) }))

	for c := range slices.Chunk([]int{1, 2, 3, 4, 5, 6, 7}, 3) {
		fmt.Print(c, len(c), cap(c), " ")
	}
	fmt.Println()
	for c := range slices.Chunk([]int{}, 2) {
		fmt.Println("never", c)
	}
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		for range slices.Chunk([]int{1}, 0) {
		}
	}()
	// A chunk aliases the input.
	in := []int{1, 2, 3, 4}
	for c := range slices.Chunk(in, 2) {
		c[0] *= 10
	}
	fmt.Println(in)

	// Collect of nothing is nil.
	fmt.Println(slices.Collect(slices.Values([]int{})) == nil)

	// Keys of a map, sorted.
	m := map[string]int{"x": 1, "y": 2, "a": 3}
	fmt.Println(slices.Sorted(maps.Keys(m)), slices.Sorted(maps.Values(m)))

	// A custom iterator used with slices.Collect.
	gen := func(yield func(string) bool) {
		for _, w := range strings.Fields("the quick brown fox") {
			if !yield(strings.ToUpper(w)) {
				return
			}
		}
	}
	fmt.Println(slices.Collect(gen))
	n := 0
	for w := range gen {
		n += len(w)
		if w == "QUICK" {
			break
		}
	}
	fmt.Println(n)

	// Nested ranges over iterators with a labeled continue.
	pairs := 0
outer:
	for i, a := range slices.All([]int{1, 2, 3}) {
		for b := range slices.Values([]int{10, 20, 30}) {
			if b == 20 && i == 1 {
				continue outer
			}
			pairs += a * b
		}
	}
	fmt.Println(pairs)

	total := 0
	for i := range 10 {
		total += i
	}
	fmt.Println(total)
	for i, v := range slices.All([]float64{1.5, 2.5}) {
		fmt.Println(i, v)
	}
}
