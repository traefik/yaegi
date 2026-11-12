package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Route struct {
	Rule     string
	Priority int
}

func main() {
	rs := []Route{{"a", 1}, {"b", 3}, {"c", 2}}
	slices.SortFunc(rs, func(a, b Route) int { return strings.Compare(b.Rule, a.Rule) })
	fmt.Println(rs)
	fmt.Println(slices.Contains([]string{"x", "y"}, "y"), slices.Index([]int{4, 5}, 5))
	m := map[string]int{"b": 2, "a": 1}
	fmt.Println(slices.Sorted(maps.Keys(m)))
}
