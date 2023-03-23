package main

import "fmt"

type vector interface {
	[]int | [3]int
}

func sum[V vector](v V) (out int) {
	for i := 0; i < len(v); i++ {
		out += v[i]
	}
	return
}

func main() {
	va := [3]int{1, 2, 3}
	vs := []int{1, 2, 3}
	fmt.Println(sum[[3]int](va), sum[[]int](vs))
}

// Output:
// 6 6
