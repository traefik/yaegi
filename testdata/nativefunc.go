// After a native call, yaegi v2 writes back the maps and slices that it gave
// to native code. Funcs and NaN map keys must survive a read only call
// (json.Marshal): every func stays the one stored at its key or index, and
// a map with a NaN key keeps all its entries. Changes made by native code
// (maps.Copy, maps.DeleteFunc, reflect Clear, sort.Slice) are kept, as in Go.
package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"sort"
)

type Cfg struct {
	Name string
	H    map[string]func() string `json:"-"`
	L    []func() string          `json:"-"`
	A    []any                    `json:"-"`
	N    map[float64]int          `json:"-"`
}

func mk(s string) func() string { return func() string { return s } }

func show(c *Cfg) {
	keys := slices.Sorted(maps.Keys(c.H))
	for _, k := range keys {
		fmt.Printf("%s=%s ", k, c.H[k]())
	}
	fmt.Println()
	for i, f := range c.L {
		fmt.Printf("%d=%s ", i, f())
	}
	fmt.Println()
	for i, f := range c.A {
		fmt.Printf("%d=%s ", i, f.(func() string)())
	}
	fmt.Println()
	showNaN(c.N)
}

func showNaN(n map[float64]int) {
	nan, sum := 0, 0
	for k, v := range n {
		if k != k {
			nan++
		}
		sum += v
	}
	fmt.Println("len", len(n), "nan", nan, "sum", sum)
}

func main() {
	c := &Cfg{Name: "n", H: map[string]func() string{}, N: map[float64]int{math.NaN(): 100}}
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		c.H[s] = mk(s)
		c.L = append(c.L, mk(s))
		c.A = append(c.A, mk(s))
	}
	for i := range 5 {
		c.N[float64(i)] = i
	}
	// Read only: nothing changes.
	for range 3 {
		if _, err := json.Marshal(c); err != nil {
			panic(err)
		}
	}
	show(c)

	// sort.Slice moves the funcs: each one follows its value.
	sort.Slice(c.L, func(i, j int) bool { return c.L[i]() > c.L[j]() })
	show(c)

	// Native code changes a map with a NaN key.
	maps.Copy(c.N, map[float64]int{math.NaN(): 1000, 7: 7})
	showNaN(c.N)
	maps.DeleteFunc(c.N, func(k float64, v int) bool { return v < 3 || k != k })
	showNaN(c.N)
	reflect.ValueOf(c.N).Clear()
	showNaN(c.N)
	c.N[math.NaN()] = 5
	maps.Copy(c.N, map[float64]int{1: 1})
	showNaN(c.N)
}
