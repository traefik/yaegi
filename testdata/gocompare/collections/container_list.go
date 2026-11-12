// container/list: PushFront, PushBack, InsertBefore/After, MoveToFront,
// MoveToBack, MoveBefore/After, Remove, PushBackList, iteration in both
// directions, and the zero List.
package main

import (
	"container/list"
	"fmt"
	"strings"
)

func dump(l *list.List) string {
	var b strings.Builder
	for e := l.Front(); e != nil; e = e.Next() {
		fmt.Fprint(&b, e.Value, " ")
	}
	b.WriteString("| ")
	for e := l.Back(); e != nil; e = e.Prev() {
		fmt.Fprint(&b, e.Value, " ")
	}
	return fmt.Sprintf("%d: %s", l.Len(), b.String())
}

func main() {
	var zero list.List
	fmt.Println(zero.Len(), zero.Front() == nil)
	zero.PushBack(1)
	fmt.Println(dump(&zero))

	l := list.New()
	e4 := l.PushBack(4)
	e1 := l.PushFront(1)
	l.InsertBefore(3, e4)
	l.InsertAfter(2, e1)
	fmt.Println(dump(l))
	l.MoveToFront(e4)
	fmt.Println(dump(l))
	l.MoveToBack(e4)
	fmt.Println(dump(l))
	l.MoveBefore(e4, e1)
	fmt.Println(dump(l))
	l.MoveAfter(e4, l.Back())
	fmt.Println(dump(l))
	v := l.Remove(e1)
	fmt.Println(v, dump(l))
	// Removing an element twice is a no-op.
	l.Remove(e1)
	fmt.Println(dump(l))

	other := list.New()
	other.PushBack("a")
	other.PushBack("b")
	l.PushBackList(other)
	l.PushFrontList(other)
	fmt.Println(dump(l), dump(other))

	// Remove while iterating.
	for e := l.Front(); e != nil; {
		next := e.Next()
		if _, ok := e.Value.(string); ok {
			l.Remove(e)
		}
		e = next
	}
	fmt.Println(dump(l))

	// An element of another list is ignored.
	foreign := other.Front()
	l.MoveToFront(foreign)
	l.InsertAfter(99, foreign)
	fmt.Println(dump(l), foreign.Value)

	// Init clears.
	l.Init()
	fmt.Println(dump(l))

	// A queue and a stack of structs.
	type job struct {
		id   int
		name string
	}
	q := list.New()
	for i := range 5 {
		q.PushBack(job{i, fmt.Sprint("j", i)})
	}
	for q.Len() > 0 {
		j := q.Remove(q.Front()).(job)
		fmt.Print(j.id, j.name, " ")
		if j.id == 1 {
			q.PushBack(job{10, "late"})
		}
	}
	fmt.Println()
	// Pointer values can be updated in place.
	pl := list.New()
	pl.PushBack(&job{1, "x"})
	pl.Front().Value.(*job).name = "y"
	fmt.Println(*pl.Front().Value.(*job))
}
