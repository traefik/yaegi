// The sort package with functions and custom types: Ints, Strings,
// Float64s, Stable and Sort of a custom sort.Interface, sort.Reverse of a
// custom type, Slice, SliceStable, Search, SearchInts, SearchStrings, Find.
package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type byLen []string

func (b byLen) Len() int           { return len(b) }
func (b byLen) Less(i, j int) bool { return len(b[i]) < len(b[j]) }
func (b byLen) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

type rec struct {
	k string
	v int
}

// recs sorts by v then k, with a pointer receiver.
type recs struct{ r []rec }

func (p *recs) Len() int { return len(p.r) }
func (p *recs) Less(i, j int) bool {
	if p.r[i].v != p.r[j].v {
		return p.r[i].v < p.r[j].v
	}
	return p.r[i].k < p.r[j].k
}
func (p *recs) Swap(i, j int) { p.r[i], p.r[j] = p.r[j], p.r[i] }

func main() {
	ints := []int{9, -3, 5, 0, 5, 12, -100}
	sort.Ints(ints)
	fmt.Println(ints, sort.IntsAreSorted(ints))
	fmt.Println(sort.SearchInts(ints, 5), sort.SearchInts(ints, 6), sort.SearchInts(ints, -1000), sort.SearchInts(ints, 1000))

	strs := []string{"banana", "apple", "Cherry", "apple2", ""}
	sort.Strings(strs)
	fmt.Printf("%q %v\n", strs, sort.StringsAreSorted(strs))
	fmt.Println(sort.SearchStrings(strs, "apple"), sort.SearchStrings(strs, "b"))

	fl := []float64{2.5, math.NaN(), -1, math.Inf(1), 0}
	sort.Float64s(fl)
	fmt.Println(fl, sort.Float64sAreSorted(fl))

	words := byLen{"ccc", "a", "bb", "dd", "e", "fff", "g"}
	sort.Stable(words)
	fmt.Println(words, sort.IsSorted(words))
	sort.Stable(sort.Reverse(words))
	fmt.Println(words, sort.IsSorted(sort.Reverse(words)))

	p := &recs{[]rec{{"c", 2}, {"a", 2}, {"b", 1}, {"z", 0}}}
	sort.Sort(p)
	fmt.Println(p.r, sort.IsSorted(p))

	rs := []rec{{"a", 3}, {"b", 1}, {"c", 3}, {"d", 2}, {"e", 1}, {"f", 3}}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].v < rs[j].v })
	fmt.Println(rs)
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].v != rs[j].v {
			return rs[i].v > rs[j].v
		}
		return rs[i].k < rs[j].k
	})
	fmt.Println(rs, sort.SliceIsSorted(rs, func(i, j int) bool { return rs[i].v > rs[j].v }))

	// Search: the first i where f is true.
	sq := []int{1, 4, 9, 16, 25, 36}
	for _, t := range []int{0, 1, 10, 36, 40} {
		fmt.Print(sort.Search(len(sq), func(i int) bool { return sq[i] >= t }), " ")
	}
	fmt.Println()
	i, found := sort.Find(len(strs), func(i int) int { return strings.Compare("banana", strs[i]) })
	fmt.Println(i, found)
	i, found = sort.Find(len(strs), func(i int) int { return strings.Compare("zzz", strs[i]) })
	fmt.Println(i, found)

	ss := sort.StringSlice{"z", "y", "x"}
	ss.Sort()
	fmt.Println(ss, ss.Search("y"), ss.Len())

	// Many equal keys keep their order with SliceStable.
	type kv struct{ k, i int }
	many := make([]kv, 100)
	for i := range many {
		many[i] = kv{i % 4, i}
	}
	sort.SliceStable(many, func(a, b int) bool { return many[a].k < many[b].k })
	okOrder := true
	for j := 1; j < len(many); j++ {
		if many[j].k == many[j-1].k && many[j].i < many[j-1].i {
			okOrder = false
		}
	}
	fmt.Println(okOrder, many[0], many[24], many[25], many[99])
}
