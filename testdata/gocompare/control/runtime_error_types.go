// The types of panic values: run-time errors of the VM are a
// runtime.errorString as in gc (also when native code such as slog
// recovers or passes them), and sort.Slice of a value that is not a slice
// panics with the *reflectlite.ValueError of gc.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sort"
)

type K struct{ s []int }

type E struct{ s []int }

func (E) Error() string { return "E" }

type Names []string

func try(name string, f func()) {
	defer func() {
		r := recover()
		_, rt := r.(runtime.Error)
		fmt.Printf("%s: %T %v: %v\n", name, r, rt, r)
	}()
	f()
}

func main() {
	try("compare", func() { var a, b any = K{}, K{}; fmt.Println(a == b) })
	try("map key", func() { m := map[any]int{}; m[K{}] = 1 })
	try("slog", func() { slog.AnyValue(K{}).Equal(slog.AnyValue(K{})) })
	try("errors.Is", func() { fmt.Println(errors.Is(E{}, E{})) })
	try("nil deref", func() { var p *int; fmt.Println(*p) })
	try("makeslice", func() { n := -1; _ = make([]int, n) })

	less := func(i, j int) bool { return false }
	try("Slice int", func() { sort.Slice(3, less) })
	try("SliceStable string", func() { sort.SliceStable("s", less) })
	try("SliceIsSorted int", func() { sort.SliceIsSorted(3, less) })
	try("Slice nil", func() { sort.Slice(nil, less) })
	try("Slice ptr", func() { s := []int{2, 1}; sort.Slice(&s, less) })
	try("Slice struct", func() { sort.Slice(K{}, less) })

	ints := []int{3, 1, 2}
	sort.Slice(ints, func(i, j int) bool { return ints[i] < ints[j] })
	names := Names{"b", "c", "a"}
	sort.SliceStable(names, func(i, j int) bool { return names[i] < names[j] })
	ks := []K{{s: []int{2}}, {s: []int{1}}}
	sort.Slice(ks, func(i, j int) bool { return ks[i].s[0] < ks[j].s[0] })
	var none []int
	sort.Slice(none, less)
	b := []byte("cab")
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	fmt.Println(ints, names, ks, none, string(b), sort.SliceIsSorted(ints, func(i, j int) bool { return ints[i] < ints[j] }))
}
