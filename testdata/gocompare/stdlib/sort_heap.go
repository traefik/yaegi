// sort.Interface and container/heap, list and ring implemented with or
// holding program types: sort.Sort, Stable, Reverse, IsSorted, Search,
// slices.SortFunc and SortStableFunc with program comparators, a priority
// queue with update, and container/list of program values.
package main

import (
	"cmp"
	"container/heap"
	"container/list"
	"container/ring"
	"fmt"
	"slices"
	"sort"
	"strings"
)

type Person struct {
	Name string
	Age  int
}

type ByAge []Person

func (a ByAge) Len() int           { return len(a) }
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAge) Less(i, j int) bool { return a[i].Age < a[j].Age }

type ByName struct{ ByAge }

func (b ByName) Less(i, j int) bool { return b.ByAge[i].Name < b.ByAge[j].Name }

type Item struct {
	value    string
	priority int
	index    int
}

type PQ []*Item

func (pq PQ) Len() int { return len(pq) }
func (pq PQ) Less(i, j int) bool {
	if pq[i].priority != pq[j].priority {
		return pq[i].priority > pq[j].priority
	}
	return pq[i].value < pq[j].value
}
func (pq PQ) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *PQ) Push(x any) {
	it := x.(*Item)
	it.index = len(*pq)
	*pq = append(*pq, it)
}
func (pq *PQ) Pop() any {
	old := *pq
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*pq = old[:n-1]
	return it
}

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

func people() []Person {
	return []Person{{"Eve", 30}, {"bob", 25}, {"Ann", 30}, {"Dan", 20}, {"Cid", 25}, {"Ann", 19}, {"Zed", 30}}
}

func main() {
	p := people()
	sort.Stable(ByAge(p))
	fmt.Println("stable age:", p, sort.IsSorted(ByAge(p)))
	sort.Stable(ByName{p})
	fmt.Println("stable name:", p)
	sort.Sort(sort.Reverse(ByAge(p)))
	fmt.Println("reverse sorted:", sort.IsSorted(sort.Reverse(ByAge(p))), sort.IsSorted(ByAge(p)))
	p = people()
	sort.SliceStable(p, func(i, j int) bool { return strings.ToLower(p[i].Name) < strings.ToLower(p[j].Name) })
	fmt.Println("slicestable:", p)
	p = people()
	slices.SortStableFunc(p, func(a, b Person) int { return cmp.Or(cmp.Compare(b.Age, a.Age), strings.Compare(a.Name, b.Name)) })
	fmt.Println("sortfunc:", p)
	i, found := slices.BinarySearchFunc(p, 25, func(e Person, t int) int { return cmp.Compare(t, e.Age) })
	fmt.Println("binsearch:", i, found)
	idx := sort.Search(len(p), func(i int) bool { return p[i].Age <= 20 })
	fmt.Println("search:", idx, p[idx])
	fmt.Println(slices.IndexFunc(p, func(x Person) bool { return x.Name == "Dan" }), slices.ContainsFunc(p, func(x Person) bool { return x.Age > 40 }))
	mx := slices.MaxFunc(p, func(a, b Person) int { return cmp.Compare(a.Name, b.Name) })
	mn := slices.MinFunc(p, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
	fmt.Println(mx, mn)
	ints := []int{5, 2, 8, 1, 9, 3}
	sort.Ints(ints)
	fmt.Println(ints, sort.SearchInts(ints, 8), sort.SearchInts(ints, 4))
	strs := sort.StringSlice{"pear", "apple", "fig"}
	strs.Sort()
	fmt.Println(strs, strs.Search("fig"))
	fl := []float64{2.5, -1, 0}
	sort.Float64s(fl)
	fmt.Println(fl)

	// Priority queue.
	items := map[string]int{"banana": 3, "apple": 2, "pear": 4, "kiwi": 3}
	pq := make(PQ, 0)
	keys := []string{"banana", "apple", "pear", "kiwi"}
	for _, k := range keys {
		heap.Push(&pq, &Item{value: k, priority: items[k]})
	}
	orange := &Item{value: "orange", priority: 1}
	heap.Push(&pq, orange)
	orange.priority = 5
	heap.Fix(&pq, orange.index)
	removed := heap.Remove(&pq, pq[len(pq)-1].index).(*Item)
	fmt.Println("removed:", removed.value, removed.index)
	for pq.Len() > 0 {
		it := heap.Pop(&pq).(*Item)
		fmt.Printf("%s:%d ", it.value, it.priority)
	}
	fmt.Println()
	h := &IntHeap{5, 2, 8}
	heap.Init(h)
	heap.Push(h, 3)
	fmt.Print("min=", (*h)[0], " ")
	for h.Len() > 0 {
		fmt.Print(heap.Pop(h), " ")
	}
	fmt.Println()

	// container/list holding program values.
	l := list.New()
	e4 := l.PushBack(Person{"four", 4})
	e1 := l.PushFront(Person{"one", 1})
	l.InsertBefore(Person{"three", 3}, e4)
	l.InsertAfter(Person{"two", 2}, e1)
	l.MoveToBack(e1)
	for e := l.Front(); e != nil; e = e.Next() {
		fmt.Print(e.Value.(Person).Name, " ")
	}
	fmt.Println(l.Len(), l.Remove(e4).(Person).Age, l.Len())
	r := ring.New(4)
	for i := 0; i < r.Len(); i++ {
		r.Value = Person{fmt.Sprint("r", i), i}
		r = r.Next()
	}
	r.Move(2).Do(func(v any) { fmt.Print(v.(Person).Name, " ") })
	fmt.Println()
}
