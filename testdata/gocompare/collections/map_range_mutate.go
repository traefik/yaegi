// Map changes during a range, limited to deterministic facts: deleting the
// current key, deleting every key, deleting not-yet-seen keys (never seen
// after), and the final contents after adds; also a map of slices, a map
// of maps and a map of structs mutated through lookups.
package main

import (
	"fmt"
	"slices"
	"sort"
)

func main() {
	m := map[int]int{}
	for i := range 100 {
		m[i] = i * i
	}
	// Delete the current key: all keys are seen once.
	seen := 0
	for k := range m {
		delete(m, k)
		seen++
	}
	fmt.Println(seen, len(m))

	// Delete every other key from inside: a deleted key not yet reached is
	// never produced.
	for i := range 50 {
		m[i] = i
	}
	got := map[int]bool{}
	for k := range m {
		got[k] = true
		if k%2 == 0 {
			for j := range 50 {
				if j != k && j%2 == 1 {
					delete(m, j)
				}
			}
		}
	}
	odd := 0
	for k := range got {
		if k%2 == 1 {
			odd++
		}
	}
	fmt.Println(len(m), odd <= 25, len(got) >= 25)

	// Adds during range: the final map is known even if the range does not
	// see the new keys.
	a := map[string]int{"a": 1, "b": 2}
	for k, v := range a {
		a[k+k] = v * 10
		if len(a) > 20 {
			break
		}
	}
	fmt.Println(len(a) >= 4, a["a"], a["b"], a["aa"], a["bb"])

	// Map of slices.
	ms := map[string][]int{}
	ms["x"] = append(ms["x"], 1)
	ms["x"] = append(ms["x"], 2)
	ms["y"] = append(ms["y"], 3)
	s := ms["x"]
	s[0] = 100
	fmt.Println(ms)
	ms["x"] = ms["x"][:1]
	fmt.Println(ms, len(ms["missing"]), ms["missing"] == nil)

	// Map of maps, created on demand.
	mm := map[string]map[string]int{}
	add := func(a, b string) {
		inner, ok := mm[a]
		if !ok {
			inner = map[string]int{}
			mm[a] = inner
		}
		inner[b]++
	}
	add("x", "1")
	add("x", "1")
	add("x", "2")
	add("y", "1")
	fmt.Println(mm, len(mm["x"]), mm["z"]["1"])
	delete(mm["x"], "1")
	fmt.Println(mm)

	// Map of structs: copy, change, store back.
	type acc struct {
		N    int
		Tags []string
	}
	sa := map[string]acc{}
	for _, w := range []string{"a", "b", "a", "c", "a"} {
		v := sa[w]
		v.N++
		v.Tags = append(v.Tags, fmt.Sprint(v.N))
		sa[w] = v
	}
	fmt.Println(sa)

	// Map of pointers: change in place.
	sp := map[string]*acc{"a": {}}
	sp["a"].N += 5
	sp["a"].Tags = append(sp["a"].Tags, "t")
	fmt.Println(*sp["a"])

	// Sorted range output.
	keys := make([]string, 0, len(sa))
	for k := range sa {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Print(k, sa[k].N, " ")
	}
	fmt.Println()

	// Grouping into a map of slices then sorting each group.
	words := []string{"apple", "avocado", "banana", "blueberry", "cherry", "apricot"}
	groups := map[byte][]string{}
	for _, w := range words {
		groups[w[0]] = append(groups[w[0]], w)
	}
	letters := make([]byte, 0)
	for k := range groups {
		letters = append(letters, k)
	}
	slices.Sort(letters)
	for _, l := range letters {
		g := groups[l]
		slices.Sort(g)
		fmt.Println(string(l), g)
	}

	// Compound assignment and increments on map entries.
	cnt := map[string]int{}
	cnt["a"] += 3
	cnt["a"] *= 4
	cnt["b"]--
	cnt["c"] |= 6
	cnt["c"] <<= 2
	fmt.Println(cnt)
	fl := map[int]float64{}
	fl[1] += 0.5
	fl[1] /= 4
	fmt.Println(fl)
	str := map[int]string{}
	str[0] += "x"
	str[0] += "y"
	fmt.Println(str)
}
