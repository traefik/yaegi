package main

// maps.Clone calls maps.clone, a function without body linked to the
// runtime: the VM implements it (GOROOT/test fixedbugs/issue62203.go).

import (
	"fmt"
	"maps"
	"net/http"
)

type pt struct{ X, Y int }

func main() {
	m := map[string]struct{}{}
	for i := 0; i < 48; i++ {
		m[fmt.Sprintf("%d", i)] = struct{}{}
	}
	m["foo"] = struct{}{}
	delete(m, "foo")
	c := maps.Clone(m)
	fmt.Println(len(m), len(c))

	s := map[string][]int{"a": {1}, "b": {2, 3}}
	s2 := maps.Clone(s)
	s2["c"] = nil
	delete(s2, "a")
	fmt.Println(len(s), len(s2), s["a"], s2["b"])

	p := map[pt]pt{{1, 2}: {3, 4}}
	p2 := maps.Clone(p)
	p2[pt{1, 2}] = pt{5, 6}
	fmt.Println(p, p2)

	var nm map[int]int
	fmt.Println(maps.Clone(nm) == nil)

	h := http.Header{"A": {"1"}}
	h2 := maps.Clone(h)
	h2.Set("B", "2")
	fmt.Println(len(h), len(h2), h2.Get("A"))
}
