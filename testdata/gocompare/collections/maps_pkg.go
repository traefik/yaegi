// The maps package: Keys and Values (sorted with slices.Sorted), All,
// Clone, Copy, DeleteFunc, Equal, EqualFunc, Collect, Insert.
package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

func main() {
	m := map[string]int{"one": 1, "two": 2, "three": 3, "four": 4}
	fmt.Println(slices.Sorted(maps.Keys(m)))
	fmt.Println(slices.Sorted(maps.Values(m)))

	c := maps.Clone(m)
	c["five"] = 5
	delete(c, "one")
	fmt.Println(m, c)
	var nm map[string]int
	fmt.Println(maps.Clone(nm) == nil, maps.Clone(map[int]int{}) == nil)

	dst := map[string]int{"one": 100, "ten": 10}
	maps.Copy(dst, m)
	fmt.Println(dst)

	maps.DeleteFunc(dst, func(k string, v int) bool { return v%2 == 0 })
	fmt.Println(dst)
	maps.DeleteFunc(dst, func(k string, v int) bool { return strings.HasPrefix(k, "t") })
	fmt.Println(dst, len(dst))

	fmt.Println(maps.Equal(m, maps.Clone(m)), maps.Equal(m, dst), maps.Equal(map[int]int{}, map[int]int(nil)))
	fmt.Println(maps.EqualFunc(map[int]string{1: "a"}, map[int]string{1: "A"}, strings.EqualFold))

	col := maps.Collect(maps.All(m))
	fmt.Println(col, maps.Equal(col, m))
	ins := map[string]int{"zero": 0}
	maps.Insert(ins, maps.All(map[string]int{"a": 1, "b": 2}))
	fmt.Println(ins)

	// Collect from slices.All: index to value.
	idx := maps.Collect(slices.All([]string{"x", "y", "z"}))
	fmt.Println(idx)

	// All with break (no order printed), and a sum.
	n := 0
	for _, v := range maps.All(m) {
		n += v
		break
	}
	fmt.Println(n > 0)
	sum := 0
	for _, v := range maps.All(m) {
		sum += v
	}
	fmt.Println(sum)

	// Inverting a map, sorted output.
	inv := map[int]string{}
	for k, v := range m {
		inv[v] = k
	}
	for _, k := range slices.Sorted(maps.Keys(inv)) {
		fmt.Print(k, "=", inv[k], " ")
	}
	fmt.Println()

	// Struct keys.
	type K struct {
		A string
		B int
	}
	sk := map[K]bool{{"a", 1}: true, {"a", 2}: false}
	sc := maps.Clone(sk)
	fmt.Println(len(sc), sc[K{"a", 1}], sc[K{"a", 2}], sc[K{"b", 1}])
	fmt.Println(sc)

	// Clone is shallow: slice values are shared.
	ms := map[string][]int{"a": {1, 2}}
	ms2 := maps.Clone(ms)
	ms2["a"][0] = 100
	ms2["b"] = []int{3}
	fmt.Println(ms, ms2)
}
