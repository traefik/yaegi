package main

import (
	"container/heap"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Interpreted types passed as native interfaces other than error/Stringer.

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(s string) error { *l = append(*l, s); return nil }

type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *intHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

type upper struct{ w io.Writer }

func (u upper) Write(p []byte) (int, error) {
	return u.w.Write([]byte(strings.ToUpper(string(p))))
}

type byLen []string

func (s byLen) Len() int           { return len(s) }
func (s byLen) Less(i, j int) bool { return len(s[i]) < len(s[j]) }
func (s byLen) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func main() {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var l list
	fs.Var(&l, "x", "values")
	if err := fs.Parse([]string{"-x", "a", "-x", "b", "rest"}); err != nil {
		fmt.Println(err)
	}
	fmt.Println(l, fs.NArg(), fs.Lookup("x").Value.String())

	h := &intHeap{5, 2, 8}
	heap.Init(h)
	heap.Push(h, 3)
	for h.Len() > 0 {
		fmt.Print(heap.Pop(h), " ")
	}
	fmt.Println()

	fmt.Fprintf(upper{os.Stdout}, "hello %s\n", "world")
	var w io.Writer = upper{os.Stdout}
	io.WriteString(w, "again\n")

	s := byLen{"ccc", "a", "bb"}
	sort.Sort(s)
	fmt.Println(s)
}
