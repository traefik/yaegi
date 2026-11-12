// sort.Sort, sort.Stable, sort.Slice, sort.Search, slices.SortFunc and
// container/heap with interpreted types.
package main

import (
	"cmp"
	"container/heap"
	"container/list"
	"fmt"
	"slices"
	"sort"
)

type Person struct {
	Name string
	Age  int
}

type ByAge []Person

func (a ByAge) Len() int           { return len(a) }
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAge) Less(i, j int) bool { return a[i].Age < a[j].Age }

type PQ []*Item

type Item struct {
	Val   string
	Prio  int
	index int
}

func (pq PQ) Len() int           { return len(pq) }
func (pq PQ) Less(i, j int) bool { return pq[i].Prio > pq[j].Prio }
func (pq PQ) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i]; pq[i].index = i; pq[j].index = j }
func (pq *PQ) Push(x any)        { it := x.(*Item); it.index = len(*pq); *pq = append(*pq, it) }
func (pq *PQ) Pop() any          { old := *pq; n := len(old); it := old[n-1]; *pq = old[:n-1]; return it }

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any          { o := *h; x := o[len(o)-1]; *h = o[:len(o)-1]; return x }

func main() {
	people := []Person{{"A", 30}, {"B", 20}, {"C", 30}, {"D", 10}, {"E", 20}}
	p1 := slices.Clone(people)
	sort.Sort(ByAge(p1))
	fmt.Println(p1[0], p1[len(p1)-1].Age)
	p2 := slices.Clone(people)
	sort.Stable(ByAge(p2))
	fmt.Println(p2)
	p3 := slices.Clone(people)
	sort.Slice(p3, func(i, j int) bool { return p3[i].Name > p3[j].Name })
	fmt.Println(p3)
	p4 := slices.Clone(people)
	sort.SliceStable(p4, func(i, j int) bool { return p4[i].Age > p4[j].Age })
	fmt.Println(p4)
	sort.Sort(sort.Reverse(ByAge(p2)))
	fmt.Println(sort.IsSorted(sort.Reverse(ByAge(p2))), p2[0].Age)
	fmt.Println(sort.Search(len(p2), func(i int) bool { return p2[i].Age <= 20 }))
	p5 := slices.Clone(people)
	slices.SortStableFunc(p5, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
	fmt.Println(p5, slices.IndexFunc(p5, func(p Person) bool { return p.Name == "C" }))
	i, found := slices.BinarySearchFunc(p5, 20, func(p Person, t int) int { return cmp.Compare(p.Age, t) })
	fmt.Println(i, found)

	pq := PQ{}
	heap.Init(&pq)
	for i, w := range []string{"low", "high", "mid", "top"} {
		heap.Push(&pq, &Item{Val: w, Prio: []int{1, 8, 5, 9}[i]})
	}
	it := pq[len(pq)-1]
	it.Prio = 10
	heap.Fix(&pq, it.index)
	for pq.Len() > 0 {
		x := heap.Pop(&pq).(*Item)
		fmt.Print(x.Val, ":", x.Prio, " ")
	}
	fmt.Println()
	h := &IntHeap{5, 2, 8}
	heap.Init(h)
	heap.Push(h, 3)
	heap.Remove(h, 1)
	for h.Len() > 0 {
		fmt.Print(heap.Pop(h), " ")
	}
	fmt.Println()

	l := list.New()
	l.PushBack(Person{"L", 1})
	l.PushFront(&Item{Val: "f"})
	for e := l.Front(); e != nil; e = e.Next() {
		fmt.Printf("%T ", e.Value)
	}
	fmt.Println(l.Back().Value.(Person).Name)
}
