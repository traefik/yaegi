package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

type Route struct {
	Rule     string
	Priority int
}

type byPrio []Route

func (b byPrio) Len() int           { return len(b) }
func (b byPrio) Less(i, j int) bool { return b[i].Priority > b[j].Priority }
func (b byPrio) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func main() {
	rs := []Route{{"a", 1}, {"b", 3}, {"c", 2}}
	sort.Sort(byPrio(rs))
	fmt.Println(rs)
	sort.Slice(rs, func(i, j int) bool { return rs[i].Rule < rs[j].Rule })
	fmt.Println(rs)
	slices.SortFunc(rs, func(a, b Route) int { return strings.Compare(b.Rule, a.Rule) })
	fmt.Println(rs)
	sort.Stable(sort.Reverse(byPrio(rs)))
	fmt.Println(rs)
}
