// A sync.Map key of a comparable type that holds an uncomparable value in an
// interface panics with the type of that value, in every mode. So does the
// old value of CompareAndSwap and CompareAndDelete, when the stored value has
// the same uncomparable type.
package main

import (
	"fmt"
	"go/ast"
	"sync"
)

type K struct{ X any }

type M map[string]int

type W struct{ O ast.Object }

func try(name string, f func()) {
	defer func() { fmt.Println(name, "recovered:", recover()) }()
	f()
}

// with returns a new map that holds k: v. A comparison that panics in
// sync.Map can leave its lock held, so each check uses its own map.
func with(k, v any) *sync.Map {
	m := new(sync.Map)
	m.Store(k, v)
	return m
}

func main() {
	var m sync.Map
	try("k1", func() { m.Store(K{X: []int{1}}, 1) })
	try("k2", func() { m.Store([2]any{1, func() {}}, 1) })
	try("k3", func() { m.Store(K{X: K{X: M{}}}, 1) })
	try("k4", func() { m.Load(K{X: map[int]int{}}) })
	try("k5", func() { m.LoadOrStore(&K{X: []int{1}}, 1) })
	try("k6", func() { m.Delete(K{X: 1}) })
	// an any field of a native struct
	try("n1", func() { m.Store(ast.Object{Data: []int{1}}, 1) })
	try("n2", func() { m.Store(W{ast.Object{Data: map[string]int{}}}, 1) })
	try("n3", func() { m.Store([1]ast.Object{{Data: func() {}}}, 1) })
	try("n4", func() { m.Store(W{ast.Object{Data: 1}}, 1) })
	try("c1", func() { fmt.Println(with("a", K{X: []int{1}}).CompareAndSwap("a", K{X: []int{1}}, 2)) })
	try("c2", func() { fmt.Println(with("a", K{X: []int{1}}).CompareAndSwap("a", K{X: 1}, 2)) })
	try("c3", func() { fmt.Println(with("b", M{}).CompareAndSwap("b", M{}, 2)) })
	try("c4", func() { fmt.Println(with("b", M{}).CompareAndDelete("b", M{})) })
	try("c5", func() { fmt.Println(with("b", M{}).CompareAndDelete("b", map[string]int{})) })
	try("c6", func() { fmt.Println(with("b", M{}).CompareAndSwap("zz", M{}, 2)) })
	f := func() {}
	try("c7", func() { fmt.Println(with("f", f).CompareAndSwap("f", f, 2)) })
	try("c8", func() { fmt.Println(with("s", []int{1}).CompareAndSwap("s", []int{1}, 2)) })
	try("c9", func() { fmt.Println(with("s", 1).CompareAndSwap("s", []int{1}, 2)) })
}
