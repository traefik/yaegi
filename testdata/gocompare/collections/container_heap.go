// container/heap: a min-heap of ints, a priority queue with Fix and Remove
// and an index kept in the items, and heap.Init on unsorted data.
package main

import (
	"container/heap"
	"fmt"
)

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

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

func main() {
	h := &IntHeap{5, 2, 8, 1, 9, 3}
	heap.Init(h)
	fmt.Println((*h)[0], h.Len())
	heap.Push(h, 0)
	heap.Push(h, 7)
	fmt.Println((*h)[0])
	var out []int
	for h.Len() > 0 {
		out = append(out, heap.Pop(h).(int))
	}
	fmt.Println(out)

	// Heap sort of a bigger set.
	h2 := &IntHeap{}
	for i := 0; i < 200; i++ {
		heap.Push(h2, (i*37)%200)
	}
	prev, ok := -1, true
	for h2.Len() > 0 {
		v := heap.Pop(h2).(int)
		if v < prev {
			ok = false
		}
		prev = v
	}
	fmt.Println(ok, prev)

	items := map[string]int{"banana": 3, "apple": 2, "pear": 4, "kiwi": 3}
	pq := PQ{}
	for _, name := range []string{"banana", "apple", "pear", "kiwi"} {
		heap.Push(&pq, &Item{value: name, priority: items[name]})
	}
	orange := &Item{value: "orange", priority: 1}
	heap.Push(&pq, orange)
	orange.priority = 5
	heap.Fix(&pq, orange.index)
	fmt.Println(pq[0].value, pq[0].index)
	removed := heap.Remove(&pq, pq.Len()-1).(*Item)
	fmt.Println(removed.index, pq.Len())
	var names []string
	for pq.Len() > 0 {
		it := heap.Pop(&pq).(*Item)
		names = append(names, fmt.Sprintf("%s:%d", it.value, it.priority))
	}
	names = append(names, removed.value)
	fmt.Println(names)
}
