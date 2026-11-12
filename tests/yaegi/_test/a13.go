package main

type T1 struct {
	num []int
}

func main() {
	a := T1{[]int{1, 3, 5}}
	for i, v := range a.num {
		println(i, v)
	}
}

// Output:
// 0 1
// 1 3
// 2 5
