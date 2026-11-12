// The runtime panics of sync.Map name the types of the program: an
// unhashable key, and an uncomparable old value of CompareAndSwap.
package main

import (
	"fmt"
	"runtime"
	"sync"
)

type Entry struct{ Tags []string }

type Key []int

func try(name string, f func()) {
	defer func() {
		r := recover()
		_, ok := r.(runtime.Error)
		fmt.Printf("%s %T %v %v\n", name, r, ok, r)
	}()
	f()
}

func main() {
	var m sync.Map
	try("a", func() { m.Store([]int{1}, 1) })
	try("b", func() { m.Load(Key{1}) })
	try("c", func() { m.LoadOrStore(map[string]int{}, 1) })
	try("d", func() { m.Delete(Entry{}) })
	try("e", func() { m.Swap(func() {}, 1) })
	// a comparison panics with the lock of the map held: one map per case
	withEntry := func() *sync.Map {
		var m sync.Map
		m.Store(1, Entry{})
		return &m
	}
	try("f", func() { withEntry().CompareAndSwap(1, Entry{}, Entry{}) })
	try("g", func() { withEntry().CompareAndDelete(1, Entry{}) })
	try("h", func() { fmt.Println(withEntry().CompareAndSwap(2, Entry{}, Entry{})) })
	try("i", func() { fmt.Println(withEntry().CompareAndSwap(1, 5, 6)) })
	try("j", func() {
		var m sync.Map
		m.Store("k", []string{"x"})
		m.CompareAndSwap("k", []string{"x"}, nil)
	})
	m.Store("ok", 1)
	v, ok := m.Load("ok")
	fmt.Println("m", v, ok)
	try("k", func() {
		var a, b any = Entry{}, Entry{}
		fmt.Println(a == b)
	})
	try("l", func() {
		mm := map[any]int{}
		mm[Entry{}] = 1
	})
}
