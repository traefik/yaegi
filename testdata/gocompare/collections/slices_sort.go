// slices sorting: Sort, SortFunc, SortStableFunc, IsSorted, IsSortedFunc,
// BinarySearch, BinarySearchFunc, Min, Max, MinFunc, MaxFunc, with ints,
// floats with NaN, strings and structs.
package main

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
)

type Person struct {
	Name string
	Age  int
}

func main() {
	a := []int{5, 2, 8, -1, 0, 2, 9, 3}
	fmt.Println(slices.IsSorted(a))
	slices.Sort(a)
	fmt.Println(a, slices.IsSorted(a))
	for _, x := range []int{-5, -1, 2, 4, 9, 10} {
		i, ok := slices.BinarySearch(a, x)
		fmt.Print(x, ":", i, ok, " ")
	}
	fmt.Println()
	fmt.Println(slices.Min(a), slices.Max(a))

	f := []float64{3, math.NaN(), 1, math.Inf(-1), 2, math.NaN(), math.Inf(1), -0.5}
	slices.Sort(f)
	fmt.Println(f)
	fmt.Println(slices.Min([]float64{1, math.NaN(), 0}), slices.Max([]float64{1, 2}))

	s := []string{"pear", "Apple", "fig", "banana", "apple", "Fig"}
	slices.Sort(s)
	fmt.Println(s)
	slices.SortStableFunc(s, func(x, y string) int {
		return cmp.Compare(strings.ToLower(x), strings.ToLower(y))
	})
	fmt.Println(s)
	slices.SortStableFunc(s, func(x, y string) int { return cmp.Compare(len(x), len(y)) })
	fmt.Println(s)

	people := []Person{{"Alice", 30}, {"Bob", 25}, {"Carol", 30}, {"Dan", 25}, {"Eve", 35}, {"Fay", 30}}
	slices.SortStableFunc(people, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
	fmt.Println(people)
	slices.SortFunc(people, func(a, b Person) int {
		return cmp.Or(cmp.Compare(b.Age, a.Age), strings.Compare(a.Name, b.Name))
	})
	fmt.Println(people)
	fmt.Println(slices.IsSortedFunc(people, func(a, b Person) int { return cmp.Compare(b.Age, a.Age) }))
	i, ok := slices.BinarySearchFunc(people, 30, func(p Person, t int) int { return cmp.Compare(t, p.Age) })
	fmt.Println(i, ok)
	i, ok = slices.BinarySearchFunc(people, 28, func(p Person, t int) int { return cmp.Compare(t, p.Age) })
	fmt.Println(i, ok)
	byAge := func(a, b Person) int { return cmp.Compare(a.Age, b.Age) }
	fmt.Println(slices.MinFunc(people, byAge), slices.MaxFunc(people, byAge))

	// Min of an empty slice panics.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(slices.Min([]int{}))
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(slices.MaxFunc([]int(nil), cmp.Compare[int]))
	}()

	// A large sort with a known result.
	big := make([]int64, 2000)
	for i := range big {
		big[i] = int64((i * 7919) % 2000)
	}
	slices.Sort(big)
	okAll := true
	for i, v := range big {
		if v != int64(i) {
			okAll = false
		}
	}
	fmt.Println(okAll, big[:5], big[1995:])
	u := []uint8{200, 3, 255, 0}
	slices.Sort(u)
	fmt.Println(u, slices.Max(u))

	// The builtin min and max on elements.
	fmt.Println(min(a[0], a[3], a[7]), max(s[0], s[1]), min(2.5, f[3]), max(int8(-3), -4))
}
