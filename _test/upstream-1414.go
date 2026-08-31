package main

import "fmt"

func main() {
	arr := make([]interface{}, 0)
	arr = append(arr, []interface{}{1, 2, 3})
	arr = append(arr, []interface{}{5, 6, 7})
	fmt.Println("array:", arr, "len:", len(arr))
	a := []int{1, 2}
	b := []int{3, 4}
	a = append(a, b...)
	grid := [][]int{}
	grid = append(grid, []int{1, 2})
	fmt.Println(a, grid, len(grid))
	bs := []byte("he")
	bs = append(bs, "llo"...)
	fmt.Println(string(bs))
}

// Output:
// array: [[1 2 3] [5 6 7]] len: 2
// [1 2 3 4] [[1 2]] 1
// hello
