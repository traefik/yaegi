// sort.Slice, sort.SliceStable and sort.SliceIsSorted on values that are
// not slices, in every mode. SliceIsSorted only needs the length: an array,
// a string, a map or a channel does not panic.
package main

import (
	"fmt"
	"sort"
)

type T struct{}

type M map[int]T

func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic %T: %v\n", name, r, r)
		}
	}()
	f()
}

func main() {
	arr := [3]int{3, 2, 1}
	calls := 0
	try("arr", func() {
		fmt.Println("arr", sort.SliceIsSorted(arr, func(i, j int) bool { calls++; return arr[i] < arr[j] }), calls)
	})
	try("str", func() {
		s := "cba"
		fmt.Println("str", sort.SliceIsSorted(s, func(i, j int) bool { return s[i] < s[j] }))
	})
	try("map", func() {
		m := M{0: {}, 1: {}}
		fmt.Println("map", sort.SliceIsSorted(m, func(i, j int) bool { return i < j }))
	})
	try("chan", func() {
		c := make(chan T, 3)
		c <- T{}
		c <- T{}
		fmt.Println("chan", sort.SliceIsSorted(c, func(i, j int) bool { return true }))
	})
	try("int", func() { fmt.Println(sort.SliceIsSorted(1, nil)) })
	try("Slice map", func() { sort.Slice(M{}, nil) })
	try("Slice chan", func() { sort.Slice(make(chan T), nil) })
	try("Slice func", func() { sort.Slice(func() {}, nil) })
	try("Slice arr", func() { sort.Slice([2]T{}, nil) })
	try("Stable *T", func() { sort.SliceStable(&T{}, nil) })
	try("Stable T", func() { sort.SliceStable(T{}, nil) })
	try("nil", func() { sort.Slice(nil, nil) })
	var ns []string
	sort.Slice(ns, nil)
	fmt.Println(sort.SliceIsSorted(ns, nil), ns == nil)
}
