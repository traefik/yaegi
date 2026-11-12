// Native boundary of sync: program types given to sync (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"sort"
	"sync"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// key types: comparable program types.
type key struct {
	a string
	b int
}
type nkey int
type gkey[T comparable] struct{ v T }

// box is comparable, but its field may hold an uncomparable value.
type box struct{ v any }

// skey is a slice type: not comparable, not hashable.
type skey []int

// val types.
type val struct{ s string }
type sval []string

// try runs f and prints its result or its panic.
func try(name string, f func() any) {
	defer func() {
		if r := recover(); r != nil {
			out("%s: panic %T: %v", name, r, r)
		}
	}()
	out("%s: %v", name, f())
}

// dump prints the content of m, sorted.
func dump(m *sync.Map) {
	var lines []string
	m.Range(func(k, v any) bool {
		lines = append(lines, fmt.Sprintf("%T %v = %v", k, k, v))
		return true
	})
	sort.Strings(lines)
	for _, l := range lines {
		out("  %s", l)
	}
}

func main() {
	// row: sync (*Map).CompareAndDelete,(*Map).CompareAndSwap,(*Map).Delete,(*Map).Load,(*Map).LoadAndDelete,(*Map).LoadOrStore,(*Map).Store,(*Map).Swap panic == of two interfaces: comparing uncomparable type
	probe("Map/program", func() {
		var m sync.Map
		pk := &key{"p", 0}
		m.Store(key{"a", 1}, val{"v1"})
		m.Store(nkey(2), "v2")
		m.Store(gkey[string]{"g"}, 3)
		m.Store(pk, val{"v4"})
		m.Store(box{"s"}, "v5")
		try("Load key", func() any { v, ok := m.Load(key{"a", 1}); return fmt.Sprint(v, ok) })
		try("Load nkey", func() any { v, ok := m.Load(nkey(2)); return fmt.Sprint(v, ok) })
		try("Load int", func() any { v, ok := m.Load(2); return fmt.Sprint(v, ok) })
		try("Load gkey", func() any { v, ok := m.Load(gkey[string]{"g"}); return fmt.Sprint(v, ok) })
		try("Load ptr", func() any { v, ok := m.Load(pk); return fmt.Sprint(v, ok) })
		try("Load other ptr", func() any { v, ok := m.Load(&key{"p", 0}); return fmt.Sprint(v, ok) })
		try("LoadOrStore", func() any { v, ok := m.LoadOrStore(key{"a", 1}, "x"); return fmt.Sprint(v, ok) })
		try("Swap", func() any { v, ok := m.Swap(nkey(2), val{"sw"}); return fmt.Sprint(v, ok) })
		try("CompareAndSwap ok", func() any { return m.CompareAndSwap(key{"a", 1}, val{"v1"}, val{"cas"}) })
		try("CompareAndSwap bad", func() any { return m.CompareAndSwap(key{"a", 1}, val{"v1"}, val{"no"}) })
		try("CompareAndDelete", func() any { return m.CompareAndDelete(gkey[string]{"g"}, 3) })
		try("LoadAndDelete", func() any { v, ok := m.LoadAndDelete(pk); return fmt.Sprint(v, ok) })
		m.Delete(box{"s"})
		dump(&m)
	})
	// row: sync (*Map).CompareAndDelete,(*Map).CompareAndSwap,(*Map).Delete,(*Map).Load,(*Map).LoadAndDelete,(*Map).LoadOrStore,(*Map).Store,(*Map).Swap panic == of two interfaces: comparing uncomparable type
	probe("Map/unhashable", func() {
		// a panic inside the map may leave it locked: one map per call
		fresh := func() *sync.Map {
			m := new(sync.Map)
			m.Store("x", sval{"a"})
			m.Store("y", box{[]int{1}})
			return m
		}
		try("Store", func() any { fresh().Store(skey{1}, 1); return "stored" })
		try("Load", func() any { v, ok := fresh().Load(skey{1}); return fmt.Sprint(v, ok) })
		try("LoadOrStore", func() any { v, ok := fresh().LoadOrStore(skey{1}, 1); return fmt.Sprint(v, ok) })
		try("Swap", func() any { v, ok := fresh().Swap(skey{1}, 1); return fmt.Sprint(v, ok) })
		try("Delete", func() any { fresh().Delete(skey{1}); return "deleted" })
		try("LoadAndDelete", func() any { v, ok := fresh().LoadAndDelete(skey{1}); return fmt.Sprint(v, ok) })
		try("CompareAndSwap", func() any { return fresh().CompareAndSwap(skey{1}, 1, 2) })
		try("CompareAndDelete", func() any { return fresh().CompareAndDelete(skey{1}, 1) })
		dump(fresh())
	})
	// row: sync (*Map).CompareAndDelete,(*Map).CompareAndSwap,(*Map).Delete,(*Map).Load,(*Map).LoadAndDelete,(*Map).LoadOrStore,(*Map).Store,(*Map).Swap panic == of two interfaces: comparing uncomparable type
	probe("Map/box", func() {
		// a panic inside the map may leave it locked: one map per call
		fresh := func() *sync.Map {
			m := new(sync.Map)
			m.Store("x", sval{"a"})
			m.Store("y", box{[]int{1}})
			return m
		}
		try("Store box", func() any { fresh().Store(box{[]int{1}}, 1); return "stored" })
		try("Load box", func() any { v, ok := fresh().Load(box{[]int{1}}); return fmt.Sprint(v, ok) })
		dump(fresh())
	})
	// row: sync (*Map).CompareAndDelete,(*Map).CompareAndSwap,(*Map).Delete,(*Map).Load,(*Map).LoadAndDelete,(*Map).LoadOrStore,(*Map).Store,(*Map).Swap panic == of two interfaces: comparing uncomparable type
	probe("Map/oldvalue", func() {
		// a panic inside the map may leave it locked: one map per call
		fresh := func() *sync.Map {
			m := new(sync.Map)
			m.Store("x", sval{"a"})
			m.Store("y", box{[]int{1}})
			return m
		}
		// uncomparable old values
		try("CompareAndSwap sval", func() any { return fresh().CompareAndSwap("x", sval{"a"}, 2) })
		try("CompareAndDelete sval", func() any { return fresh().CompareAndDelete("x", sval{"a"}) })
		try("CompareAndSwap val", func() any { return fresh().CompareAndSwap("x", val{"a"}, 2) })
		try("CompareAndSwap box", func() any { return fresh().CompareAndSwap("y", box{[]int{1}}, 2) })
		try("CompareAndDelete box", func() any { return fresh().CompareAndDelete("y", box{[]int{1}}) })
		try("CompareAndSwap other", func() any { return fresh().CompareAndSwap("y", box{2}, 2) })
		dump(fresh())
	})
}
