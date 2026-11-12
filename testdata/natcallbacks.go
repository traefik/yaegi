package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Funcs of the program given to native code with the signatures that have a
// callback trampoline (vm/callbacks.go): func(int, int) bool, func(int)
// bool, func(int) int, func(rune) bool, func(rune) rune. Their results,
// panics and captured variables must be the ones of go run, also with
// named types in the signature (the general path).

type person struct {
	name string
	age  int
}

type score int

func main() {
	ps := []person{{"carol", 41}, {"alice", 30}, {"bob", 30}, {"dave", 19}, {"eve", 55}}
	calls := 0
	sort.Slice(ps, func(i, j int) bool {
		calls++
		return ps[i].name < ps[j].name
	})
	fmt.Println(ps, calls > 0)
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].age < ps[j].age })
	fmt.Println(ps, sort.SliceIsSorted(ps, func(i, j int) bool { return ps[i].age < ps[j].age }))

	// A less func that sorts another slice inside (nested callbacks).
	inner := []int{3, 1, 2}
	sort.Slice(ps, func(i, j int) bool {
		sort.Slice(inner, func(a, b int) bool { return inner[a] > inner[b] })
		return ps[i].age > ps[j].age
	})
	fmt.Println(ps, inner)

	// A panic in the callback goes through native code to the caller.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		sort.Slice(ps, func(i, j int) bool {
			if ps[i].name == "bob" || ps[j].name == "bob" {
				panic(errors.New("no bob"))
			}
			return false
		})
	}()

	xs := []int{1, 3, 5, 7, 9, 11}
	fmt.Println(sort.Search(len(xs), func(i int) bool { return xs[i] >= 7 }))
	i, found := sort.Find(len(xs), func(i int) int { return 5 - xs[i] })
	fmt.Println(i, found)

	fmt.Println(strings.FieldsFunc("a1b22c333;d", unicode.IsDigit))
	fmt.Println(strings.FieldsFunc("  x,y;;z ", func(r rune) bool { return r == ',' || r == ';' || r == ' ' }))
	fmt.Println(strings.IndexFunc("héllo wörld", func(r rune) bool { return r > 127 }))
	fmt.Println(strings.TrimFunc("123abc456", unicode.IsDigit))
	fmt.Println(strings.Map(func(r rune) rune {
		switch {
		case r == 'a':
			return -1 // dropped
		case r >= 'b' && r <= 'z':
			return r - 32
		}
		return r
	}, "banana 42 é"))
	fmt.Println(strings.Map(func(r rune) rune { return r + 0x10000 }, "ab"))

	// Named types in the signature take the general path.
	scores := []score{3, 1, 2}
	sort.Slice(scores, func(i, j int) bool { return scores[i] < scores[j] })
	fmt.Println(scores)
	type lessFn func(i, j int) bool
	var lf lessFn = func(i, j int) bool { return xs[i] > xs[j] }
	sort.Slice(xs, lf)
	fmt.Println(xs)
}
