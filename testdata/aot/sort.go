// AOT test program: sort on interpreted slices and sort.Interface.
//
//aot:lib sort
package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"
)

type Person struct {
	Name string
	Age  int
}

type ByAge []Person

func (a ByAge) Len() int           { return len(a) }
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAge) Less(i, j int) bool { return a[i].Age < a[j].Age }

type Score float64

func main() {
	if benchMain(benchOp) {
		return
	}
	people := []Person{{"carol", 41}, {"alice", 30}, {"bob", 30}, {"dave", 19}}
	sort.Slice(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	fmt.Println(people)
	sort.SliceStable(people, func(i, j int) bool { return people[i].Age < people[j].Age })
	fmt.Println(people, sort.SliceIsSorted(people, func(i, j int) bool { return people[i].Age < people[j].Age }))
	sort.Sort(sort.Reverse(ByAge(people)))
	fmt.Println(people, sort.IsSorted(ByAge(people)))
	sort.Stable(ByAge(people))
	fmt.Println(people)
	ints := []int{5, 2, 9, 1}
	sort.Ints(ints)
	strs := []string{"b", "c", "a"}
	sort.Strings(strs)
	fl := []float64{2.5, -1, 3}
	sort.Float64s(fl)
	fmt.Println(ints, strs, fl, sort.SearchInts(ints, 5), sort.StringsAreSorted(strs))
	scores := []Score{3, 1, 2}
	sort.Slice(scores, func(i, j int) bool { return scores[i] > scores[j] })
	fmt.Println(scores)
	i := sort.Search(len(ints), func(i int) bool { return ints[i] >= 3 })
	j, found := sort.Find(len(strs), func(i int) int {
		switch {
		case "b" < strs[i]:
			return -1
		case "b" > strs[i]:
			return 1
		}
		return 0
	})
	fmt.Println(i, j, found)
	arr := [3]int{3, 1, 2}
	sort.Ints(arr[:])
	fmt.Println(arr)
	sl := sort.IntSlice{4, 3}
	sl.Sort()
	fmt.Println(sl, sl.Search(4))
}

// benchOp: sort.Slice of 64 interpreted structs.
func benchOp() {
	ps := make([]Person, 64)
	for i := range ps {
		ps[i] = Person{Name: strconv.Itoa(i), Age: (i * 7919) % 101}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Age < ps[j].Age })
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
