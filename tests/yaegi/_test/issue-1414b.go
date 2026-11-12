package main

import "fmt"

type T struct{ A int }

func main() {
	a := []interface{}{0}
	a = append(a, []interface{}{1, 2})
	a = append(a, []interface{}{3, 4}...)
	fmt.Println(len(a), a)

	var b []any
	var nilSlice []any
	b = append(b, nilSlice)
	b = append(b, nilSlice...)
	fmt.Println(len(b), b, b[0] == nil)

	c := [][]int{{1}}
	c = append(c, []int{2, 3})
	c = append(c, [][]int{{4}, {5, 6}}...)
	fmt.Println(len(c), c)

	d := []byte("ab")
	d = append(d, "cd"...)
	d = append(d, []byte("ef")...)
	d = append(d, 'g')
	fmt.Println(string(d))

	e := []any{}
	e = append(e, []T{{1}, {2}})
	e = append(e, []int{7})
	e = append(e, "str")
	fmt.Println(len(e), e)

	f := []T{{1}}
	f = append(f, []T{{2}, {3}}...)
	fmt.Println(f)
}

// Output:
// 4 [0 [1 2] 3 4]
// 1 [[]] false
// 4 [[1] [2 3] [4] [5 6]]
// abcdefg
// 3 [[{1} {2}] [7] str]
// [{1} {2} {3}]
