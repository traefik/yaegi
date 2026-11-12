// slices.SortFunc on an input that makes pdqsort call breakPatterns: the
// generic instance calls the unexported nextPowerOfTwo and xorshift.Next of
// package slices (sigs.k8s.io/json sorts its fields this way).
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type field struct {
	name  string
	index int
}

func main() {
	// A pattern that makes pdqsort pick bad pivots, so that it calls
	// breakPatterns (nextPowerOfTwo, xorshift).
	n := 5000
	fs := make([]field, n)
	for i := range fs {
		k := i
		if i%2 == 1 {
			k = n - i
		}
		fs[i] = field{name: fmt.Sprintf("f%05d", k%97*31+k), index: i}
	}
	slices.SortFunc(fs, func(a, b field) int { return cmp.Compare(a.name, b.name) })
	fmt.Println(fs[0], fs[n/2], fs[n-1], slices.IsSortedFunc(fs, func(a, b field) int { return cmp.Compare(a.name, b.name) }))
	xs := make([]int, n)
	for i := range xs {
		xs[i] = (i * 7919) % 1000
	}
	slices.Sort(xs)
	fmt.Println(xs[0], xs[n/2], xs[n-1])
}
