// cmp.Ordered, cmp.Compare, cmp.Less, cmp.Or, min and max on type
// parameters, NaN ordering, generic sorting and binary search, and the
// slices and maps packages on program types.
package main

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
)

type Score int

type Person struct {
	Name string
	Age  int
}

func MinMax[T cmp.Ordered](xs ...T) (lo, hi T) {
	if len(xs) == 0 {
		return
	}
	lo, hi = xs[0], xs[0]
	for _, x := range xs[1:] {
		lo = min(lo, x)
		hi = max(hi, x)
	}
	return
}

func InsertSorted[T cmp.Ordered](s []T, v T) []T {
	i, _ := slices.BinarySearch(s, v)
	return slices.Insert(s, i, v)
}

func Clamp[T cmp.Ordered](v, lo, hi T) T { return max(lo, min(v, hi)) }

func main() {
	fmt.Println(cmp.Compare(1, 2), cmp.Compare(2, 2), cmp.Compare(3, 2))
	fmt.Println(cmp.Compare("a", "b"), cmp.Compare("b", "a"), cmp.Compare("", ""))
	nan := math.NaN()
	fmt.Println(cmp.Compare(nan, 1.0), cmp.Compare(1.0, nan), cmp.Compare(nan, nan), cmp.Compare(math.Inf(-1), nan))
	fmt.Println(cmp.Less(nan, 1.0), cmp.Less(1.0, nan), cmp.Less(nan, nan), cmp.Less(-0.0, 0.0))
	fmt.Println(cmp.Or(0, 0, 3, 4), cmp.Or("", "x"), cmp.Or[string](), cmp.Or(Score(0), Score(7)))
	fmt.Println(min(3, 1, 2), max(3, 1, 2), min("b", "a"), max(2.5, -1), min(Score(4), Score(2)))
	fmt.Println(math.IsNaN(min(1, nan)), math.IsNaN(max(nan, 1)))
	fmt.Println(MinMax(5, 3, 9, 1))
	fmt.Println(MinMax("pear", "apple", "zoo"))
	fmt.Println(MinMax[float64]())
	fmt.Println(MinMax(Score(3), Score(-3)))
	fmt.Println(Clamp(15, 0, 10), Clamp(-5, 0, 10), Clamp("m", "a", "k"))

	s := []int{}
	for _, v := range []int{5, 1, 4, 1, 3} {
		s = InsertSorted(s, v)
	}
	fmt.Println(s)
	fmt.Println(slices.BinarySearch(s, 4))
	fmt.Println(slices.BinarySearch(s, 2))

	people := []Person{{"bob", 30}, {"ann", 25}, {"cat", 30}, {"dan", 25}}
	slices.SortStableFunc(people, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
	fmt.Println(people)
	slices.SortFunc(people, func(a, b Person) int {
		return cmp.Or(cmp.Compare(b.Age, a.Age), strings.Compare(a.Name, b.Name))
	})
	fmt.Println(people)
	i, found := slices.BinarySearchFunc(people, 25, func(p Person, age int) int { return cmp.Compare(age, p.Age) })
	fmt.Println(i, found)
	fmt.Println(slices.IndexFunc(people, func(p Person) bool { return p.Name == "cat" }), slices.Contains(people, Person{"ann", 25}))
	fmt.Println(slices.MaxFunc(people, func(a, b Person) int { return strings.Compare(a.Name, b.Name) }))

	scores := []Score{3, 1, 2}
	slices.Sort(scores)
	fmt.Println(scores, slices.Max(scores), slices.Min(scores), slices.Index(scores, 2))
	fmt.Println(slices.Equal(scores, []Score{1, 2, 3}), slices.Compare(scores, []Score{1, 3}))
	slices.Reverse(scores)
	fmt.Println(scores)

	m := map[string]Score{"b": 2, "a": 1, "c": 3}
	keys := slices.Sorted(maps.Keys(m))
	vals := slices.Sorted(maps.Values(m))
	fmt.Println(keys, vals)
	m2 := maps.Clone(m)
	m2["d"] = 4
	fmt.Println(len(m), len(m2), maps.Equal(m, m2))
	maps.DeleteFunc(m2, func(k string, v Score) bool { return v%2 == 0 })
	fmt.Println(m2)

	fs := []float64{3, nan, 1, math.Inf(1), -0.0}
	slices.Sort(fs)
	fmt.Println(fs)
}
