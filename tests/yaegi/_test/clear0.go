package main

import "fmt"

type T struct{ a, b int }

func main() {
	m := map[string]int{"a": 1, "b": 2}
	clear(m)
	fmt.Println(len(m), m)

	s := []int{1, 2, 3}
	clear(s[1:])
	fmt.Println(len(s), s)

	ts := []T{{1, 2}, {3, 4}}
	clear(ts)
	fmt.Println(ts)

	es := []error{fmt.Errorf("e")}
	clear(es)
	fmt.Println(es[0] == nil)

	var nm map[int]int
	clear(nm)
	fmt.Println(nm == nil)

	d := map[int]bool{1: true}
	func() {
		defer clear(d)
	}()
	fmt.Println(len(d))
}

// Output:
// 0 map[]
// 3 [1 0 0]
// [{0 0} {0 0}]
// true
// true
// 0
